package target

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const Version = 1

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type Config struct {
	Version int               `json:"version"`
	Default string            `json:"default,omitempty"`
	Targets map[string]Target `json:"targets"`
}

type Target struct {
	URL         string `json:"url"`
	Namespace   string `json:"namespace"`
	Key         string `json:"key,omitempty"`
	SSHHost     string `json:"ssh_host"`
	SSHUser     string `json:"ssh_user,omitempty"`
	SSHPort     int    `json:"ssh_port,omitempty"`
	Identity    string `json:"identity,omitempty"`
	RemoteBin   string `json:"remote_bin,omitempty"`
	ContextqBin string `json:"contextq_bin,omitempty"`
	DataRoot    string `json:"data_root,omitempty"`
	Caddyfile   string `json:"caddyfile,omitempty"`
	SnippetDir  string `json:"snippet_dir,omitempty"`
	ServiceUser string `json:"service_user,omitempty"`
	ControlPath string `json:"-"`
}

type Named struct {
	Name   string `json:"name"`
	Target Target `json:"target"`
}

func DefaultPath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CONTEXTQ_SERVER_CONFIG")); path != "" {
		return filepath.Abs(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".contextq-server"), nil
}

func Load(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Version: Version, Targets: map[string]Target{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open target config: %w", err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse target config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("parse target config: unexpected trailing data")
	}
	if cfg.Version != Version {
		return nil, fmt.Errorf("unsupported target config version %d", cfg.Version)
	}
	if cfg.Targets == nil {
		cfg.Targets = map[string]Target{}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return err
		}
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".contextq-server-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c); err != nil {
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
	return os.Rename(tmpName, path)
}

func (c *Config) Validate() error {
	if c.Version == 0 {
		c.Version = Version
	}
	if c.Version != Version {
		return fmt.Errorf("unsupported target config version %d", c.Version)
	}
	if c.Targets == nil {
		c.Targets = map[string]Target{}
	}
	for name, target := range c.Targets {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("invalid target name %q", name)
		}
		if err := target.Validate(); err != nil {
			return fmt.Errorf("target %q: %w", name, err)
		}
	}
	if c.Default != "" {
		if _, ok := c.Targets[c.Default]; !ok {
			return fmt.Errorf("default target %q does not exist", c.Default)
		}
	}
	return nil
}

func (t *Target) Validate() error {
	t.URL = strings.TrimRight(strings.TrimSpace(t.URL), "/")
	u, err := url.Parse(t.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("url must be an http(s) origin")
	}
	if !namePattern.MatchString(t.Namespace) {
		return fmt.Errorf("invalid namespace %q", t.Namespace)
	}
	t.SSHHost = strings.TrimSpace(t.SSHHost)
	if t.SSHHost == "" {
		t.SSHHost = u.Hostname()
	}
	if t.SSHUser == "" {
		t.SSHUser = "root"
	}
	if t.SSHPort < 0 || t.SSHPort > 65535 {
		return fmt.Errorf("invalid SSH port %d", t.SSHPort)
	}
	if t.RemoteBin == "" {
		t.RemoteBin = "/usr/local/bin/contextq-server"
	}
	if t.ContextqBin == "" {
		t.ContextqBin = "/usr/local/bin/contextq"
	}
	if t.DataRoot == "" {
		t.DataRoot = "/var/contextq"
	}
	if t.Caddyfile == "" {
		t.Caddyfile = "/etc/caddy/Caddyfile"
	}
	if t.SnippetDir == "" {
		t.SnippetDir = "/etc/caddy/contextq.d"
	}
	if t.ServiceUser == "" {
		t.ServiceUser = "contextq"
	}
	for field, value := range map[string]string{
		"remote_bin":   t.RemoteBin,
		"contextq_bin": t.ContextqBin,
		"data_root":    t.DataRoot,
		"caddyfile":    t.Caddyfile,
		"snippet_dir":  t.SnippetDir,
	} {
		if !filepath.IsAbs(value) || strings.ContainsAny(value, " \t\r\n\x00") {
			return fmt.Errorf("%s must be an absolute path without whitespace", field)
		}
	}
	if !namePattern.MatchString(t.ServiceUser) {
		return fmt.Errorf("invalid service user %q", t.ServiceUser)
	}
	return nil
}

func (c *Config) Resolve(name string) (Named, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = c.Default
	}
	if name == "" {
		return Named{}, fmt.Errorf("no default target configured")
	}
	target, ok := c.Targets[name]
	if !ok {
		return Named{}, fmt.Errorf("target %q does not exist", name)
	}
	if err := target.Validate(); err != nil {
		return Named{}, err
	}
	return Named{Name: name, Target: target}, nil
}

func (c *Config) Set(name string, target Target, makeDefault bool) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid target name %q", name)
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if c.Targets == nil {
		c.Targets = map[string]Target{}
	}
	c.Targets[name] = target
	if makeDefault || c.Default == "" {
		c.Default = name
	}
	return nil
}

func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Targets))
	for name := range c.Targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
