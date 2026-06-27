package remote

import (
	"context"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/norlinga/contextq-server/internal/target"
)

func TestSSHAndSCPArgs(t *testing.T) {
	target := target.Target{SSHHost: "example.com", SSHUser: "root", SSHPort: 2222, Identity: "/tmp/key"}
	if got, want := SSHArgs(target), []string{"-p", "2222", "-i", "/tmp/key", "root@example.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SSHArgs=%q want=%q", got, want)
	}
	if got, want := SCPArgs(target), []string{"-P", "2222", "-i", "/tmp/key"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SCPArgs=%q want=%q", got, want)
	}
}

func TestMultiplexArgs(t *testing.T) {
	target := target.Target{SSHHost: "example.com", SSHUser: "root", ControlPath: "/tmp/control"}
	want := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=60",
		"-o", "ControlPath=/tmp/control",
		"root@example.com",
	}
	if got := SSHArgs(target); !reflect.DeepEqual(got, want) {
		t.Fatalf("SSHArgs=%q want=%q", got, want)
	}
}

func TestShellCommandQuotesArguments(t *testing.T) {
	got := ShellCommand("key", "add", "--label", "Aaron's laptop")
	want := `'key' 'add' '--label' 'Aaron'\''s laptop'`
	if got != want {
		t.Fatalf("command=%q want=%q", got, want)
	}
}

type recordingExecutor struct {
	program string
	args    []string
}

func (e *recordingExecutor) Run(_ context.Context, program string, args []string, _ io.Reader) (Result, error) {
	e.program = program
	e.args = append([]string(nil), args...)
	return Result{}, nil
}

func TestSessionCloseRemovesControlDirectory(t *testing.T) {
	session, err := NewSession(target.Target{SSHHost: "example.com", SSHUser: "root"})
	if err != nil {
		t.Fatal(err)
	}
	dir := session.dir
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	executor := &recordingExecutor{}
	if err := session.Close(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	if executor.program != "ssh" || !containsArg(executor.args, "-O") || !containsArg(executor.args, "exit") {
		t.Fatalf("close command=%s %q", executor.program, executor.args)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("control directory still exists: %v", err)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
