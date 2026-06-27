package namespace

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"
)

const keyFileVersion = 1

var (
	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrNamespaceMissing = errors.New("namespace not found")
	ErrInvalidLabel     = errors.New("invalid key label")
	ErrDuplicateLabel   = errors.New("key label already exists")
	ErrKeyMissing       = errors.New("key not found")
	ErrInvalidKey       = errors.New("invalid key")
)

var namespacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type Store struct {
	root string
	now  func() time.Time
}

type Namespace struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Key struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Digest    string    `json:"digest,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type IssuedKey struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

type keyFile struct {
	Version int   `json:"version"`
	Keys    []Key `json:"keys"`
}

func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("data root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) NamespaceDir(name string) (string, error) {
	if !namespacePattern.MatchString(name) || name == "." || name == ".." {
		return "", ErrInvalidNamespace
	}
	return filepath.Join(s.root, name), nil
}

func (s *Store) ContextqRoot(name string) (string, error) {
	dir, err := s.NamespaceDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "contextq"), nil
}

func (s *Store) Init(name string) (Namespace, error) {
	dir, err := s.NamespaceDir(name)
	if err != nil {
		return Namespace{}, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "contextq"), 0o750); err != nil {
		return Namespace{}, err
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return Namespace{}, err
	}

	var ns Namespace
	err = withLock(filepath.Join(dir, ".namespace.lock"), func() error {
		metadataPath := filepath.Join(dir, "namespace.json")
		existing, readErr := readNamespace(metadataPath)
		switch {
		case readErr == nil:
			if existing.Name != name {
				return fmt.Errorf("namespace metadata mismatch")
			}
			ns = existing
		case errors.Is(readErr, os.ErrNotExist):
			ns = Namespace{Name: name, CreatedAt: s.now()}
			if err := writeJSONAtomic(metadataPath, ns, 0o640); err != nil {
				return err
			}
		default:
			return readErr
		}

		keysPath := filepath.Join(dir, "keys.json")
		if _, err := os.Stat(keysPath); errors.Is(err, os.ErrNotExist) {
			return writeJSONAtomic(keysPath, keyFile{Version: keyFileVersion, Keys: []Key{}}, 0o600)
		} else if err != nil {
			return err
		}
		return nil
	})
	return ns, err
}

func (s *Store) List() ([]Namespace, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return []Namespace{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Namespace{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ns, err := readNamespace(filepath.Join(s.root, entry.Name(), "namespace.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, ns)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) Exists(name string) bool {
	dir, err := s.NamespaceDir(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "namespace.json"))
	return err == nil
}

func (s *Store) IssueKey(name, label string) (IssuedKey, error) {
	label = strings.TrimSpace(label)
	if err := validateLabel(label); err != nil {
		return IssuedKey{}, err
	}
	dir, err := s.existingDir(name)
	if err != nil {
		return IssuedKey{}, err
	}

	var issued IssuedKey
	err = withLock(filepath.Join(dir, ".keys.lock"), func() error {
		keys, err := readKeyFile(filepath.Join(dir, "keys.json"))
		if err != nil {
			return err
		}
		for _, key := range keys.Keys {
			if strings.EqualFold(key.Label, label) {
				return ErrDuplicateLabel
			}
		}

		idBytes, err := randomBytes(6)
		if err != nil {
			return err
		}
		secret, err := randomBytes(32)
		if err != nil {
			return err
		}
		id := "k_" + hex.EncodeToString(idBytes)
		token := "cqk_" + id + "_" + base64.RawURLEncoding.EncodeToString(secret)
		createdAt := s.now()
		digest := sha256.Sum256([]byte(token))
		keys.Keys = append(keys.Keys, Key{
			ID:        id,
			Label:     label,
			Digest:    hex.EncodeToString(digest[:]),
			CreatedAt: createdAt,
		})
		if err := writeJSONAtomic(filepath.Join(dir, "keys.json"), keys, 0o600); err != nil {
			return err
		}
		issued = IssuedKey{ID: id, Label: label, Token: token, CreatedAt: createdAt}
		return nil
	})
	return issued, err
}

func (s *Store) ListKeys(name string) ([]Key, error) {
	dir, err := s.existingDir(name)
	if err != nil {
		return nil, err
	}
	keys, err := readKeyFile(filepath.Join(dir, "keys.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Key, len(keys.Keys))
	copy(out, keys.Keys)
	for i := range out {
		out[i].Digest = ""
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) RevokeKey(name, id string) error {
	dir, err := s.existingDir(name)
	if err != nil {
		return err
	}
	return withLock(filepath.Join(dir, ".keys.lock"), func() error {
		keys, err := readKeyFile(filepath.Join(dir, "keys.json"))
		if err != nil {
			return err
		}
		filtered := make([]Key, 0, len(keys.Keys))
		found := false
		for _, key := range keys.Keys {
			if key.ID == id {
				found = true
				continue
			}
			filtered = append(filtered, key)
		}
		if !found {
			return ErrKeyMissing
		}
		keys.Keys = filtered
		return writeJSONAtomic(filepath.Join(dir, "keys.json"), keys, 0o600)
	})
}

func (s *Store) Authenticate(name, token string) (Key, error) {
	dir, err := s.existingDir(name)
	if err != nil {
		return Key{}, ErrInvalidKey
	}
	id, ok := keyID(token)
	if !ok {
		return Key{}, ErrInvalidKey
	}
	keys, err := readKeyFile(filepath.Join(dir, "keys.json"))
	if err != nil {
		return Key{}, err
	}
	digest := sha256.Sum256([]byte(token))
	for _, key := range keys.Keys {
		if key.ID != id {
			continue
		}
		stored, err := hex.DecodeString(key.Digest)
		if err != nil || len(stored) != len(digest) {
			return Key{}, ErrInvalidKey
		}
		if subtle.ConstantTimeCompare(stored, digest[:]) == 1 {
			key.Digest = ""
			return key, nil
		}
		return Key{}, ErrInvalidKey
	}
	return Key{}, ErrInvalidKey
}

func (s *Store) existingDir(name string) (string, error) {
	dir, err := s.NamespaceDir(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "namespace.json")); errors.Is(err, os.ErrNotExist) {
		return "", ErrNamespaceMissing
	} else if err != nil {
		return "", err
	}
	return dir, nil
}

func validateLabel(label string) error {
	if label == "" || len(label) > 120 {
		return ErrInvalidLabel
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return ErrInvalidLabel
		}
	}
	return nil
}

func keyID(token string) (string, bool) {
	parts := strings.SplitN(token, "_", 4)
	if len(parts) != 4 || parts[0] != "cqk" || parts[1] != "k" || parts[2] == "" || parts[3] == "" {
		return "", false
	}
	return "k_" + parts[2], true
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

func readNamespace(path string) (Namespace, error) {
	var ns Namespace
	if err := readJSON(path, &ns); err != nil {
		return Namespace{}, err
	}
	return ns, nil
}

func readKeyFile(path string) (keyFile, error) {
	var keys keyFile
	if err := readJSON(path, &keys); err != nil {
		return keyFile{}, err
	}
	if keys.Version != keyFileVersion {
		return keyFile{}, fmt.Errorf("unsupported key file version %d", keys.Version)
	}
	if keys.Keys == nil {
		keys.Keys = []Key{}
	}
	return keys, nil
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("unexpected trailing data in %s", path)
	}
	return nil
}

func writeJSONAtomic(path string, v any, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func withLock(path string, fn func() error) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
