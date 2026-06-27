package api

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommandRunnerSetsDirectoryAndRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "contextq")
	script := filepath.Join(t.TempDir(), "contextq")
	contents := `#!/bin/sh
pwd > observed-pwd
printf '%s\n' "$@" > observed-args
printf '{"ok":true}\n'
`
	if err := os.WriteFile(script, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := (CommandRunner{Binary: script}).Run(context.Background(), dir, root, []string{"queue", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || string(result.Stdout) != "{\"ok\":true}\n" {
		t.Fatalf("result=%#v", result)
	}
	pwd, err := os.ReadFile(filepath.Join(dir, "observed-pwd"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(pwd)) != dir {
		t.Fatalf("pwd=%q want=%q", strings.TrimSpace(string(pwd)), dir)
	}
	argBytes, err := os.ReadFile(filepath.Join(dir, "observed-args"))
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(argBytes)), "\n")
	want := []string{"--json", "--root", root, "queue", "list"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q want=%q", args, want)
	}
}

func TestCommandRunnerEnforcesOutputLimit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(t.TempDir(), "contextq")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '123456789'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := (CommandRunner{Binary: script, OutputLimit: 4}).Run(context.Background(), dir, filepath.Join(dir, "contextq"), []string{"queue", "list"})
	if err != ErrOutputLimit {
		t.Fatalf("error=%v", err)
	}
}
