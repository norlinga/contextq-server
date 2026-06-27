package namespace

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNamespaceAndKeyLifecycle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC) }

	ns, err := store.Init("project-a")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Name != "project-a" {
		t.Fatalf("name = %q", ns.Name)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), "project-a", "contextq")); err != nil {
		t.Fatalf("contextq root: %v", err)
	}
	if _, err := store.Init("project-a"); err != nil {
		t.Fatalf("idempotent init: %v", err)
	}

	issued, err := store.IssueKey("project-a", "macbook-codex")
	if err != nil {
		t.Fatal(err)
	}
	if issued.ID == "" || issued.Token == "" {
		t.Fatalf("incomplete issued key: %#v", issued)
	}
	authenticated, err := store.Authenticate("project-a", issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.ID != issued.ID || authenticated.Label != issued.Label || authenticated.Digest != "" {
		t.Fatalf("authenticated key = %#v", authenticated)
	}

	keys, err := store.ListKeys("project-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Digest != "" {
		t.Fatalf("listed keys = %#v", keys)
	}
	if _, err := store.IssueKey("project-a", "MACBOOK-CODEX"); !errors.Is(err, ErrDuplicateLabel) {
		t.Fatalf("duplicate label error = %v", err)
	}

	if err := store.RevokeKey("project-a", issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate("project-a", issued.Token); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("authentication after revoke = %v", err)
	}
}

func TestKeyIsScopedToNamespace(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if _, err := store.Init(name); err != nil {
			t.Fatal(err)
		}
	}
	issued, err := store.IssueKey("one", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate("two", issued.Token); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("cross-namespace authentication = %v", err)
	}
}

func TestConcurrentDuplicateLabelsCreateOneKey(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Init("shared"); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	results := make(chan error, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	start := make(chan struct{})
	for range workers {
		go func() {
			ready.Done()
			<-start
			_, err := store.IssueKey("shared", "same-agent")
			results <- err
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	duplicates := 0
	for range workers {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrDuplicateLabel):
			duplicates++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || duplicates != workers-1 {
		t.Fatalf("successes=%d duplicates=%d", successes, duplicates)
	}
}

func TestNamespaceValidation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", ".", "..", "../escape", "has/slash", "has space"} {
		if _, err := store.Init(name); !errors.Is(err, ErrInvalidNamespace) {
			t.Errorf("Init(%q) error = %v", name, err)
		}
	}
}
