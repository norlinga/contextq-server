package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTripAndResolve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &Config{Version: Version, Targets: map[string]Target{}}
	if err := cfg.Set("longship", Target{
		URL:       "https://q.longship.dev/",
		Namespace: "agents",
		SSHHost:   "longship.dev",
	}, true); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := loaded.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "longship" || resolved.Target.URL != "https://q.longship.dev" || resolved.Target.RemoteBin != "/usr/local/bin/contextq-server" {
		t.Fatalf("resolved=%#v", resolved)
	}
}

func TestTargetValidation(t *testing.T) {
	tests := []Target{
		{URL: "", Namespace: "agents"},
		{URL: "ftp://q.example.com", Namespace: "agents"},
		{URL: "https://q.example.com/path", Namespace: "agents"},
		{URL: "https://q.example.com", Namespace: "../escape"},
	}
	for _, target := range tests {
		if err := target.Validate(); err == nil {
			t.Errorf("Validate(%#v) succeeded", target)
		}
	}
}
