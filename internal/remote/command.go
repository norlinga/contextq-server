package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/norlinga/contextq-server/internal/target"
)

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Executor interface {
	Run(ctx context.Context, program string, args []string, stdin io.Reader) (Result, error)
}

type OSExecutor struct{}

func (OSExecutor) Run(ctx context.Context, program string, args []string, stdin io.Reader) (Result, error) {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("%s exited with status %d", program, result.ExitCode)
	}
	return result, err
}

func SSHArgs(t target.Target) []string {
	args := []string{}
	if t.SSHPort != 0 {
		args = append(args, "-p", strconv.Itoa(t.SSHPort))
	}
	if t.Identity != "" {
		args = append(args, "-i", t.Identity)
	}
	args = appendMultiplexArgs(args, t.ControlPath)
	return append(args, t.SSHUser+"@"+t.SSHHost)
}

func SCPArgs(t target.Target) []string {
	args := []string{}
	if t.SSHPort != 0 {
		args = append(args, "-P", strconv.Itoa(t.SSHPort))
	}
	if t.Identity != "" {
		args = append(args, "-i", t.Identity)
	}
	args = appendMultiplexArgs(args, t.ControlPath)
	return args
}

func appendMultiplexArgs(args []string, controlPath string) []string {
	if controlPath == "" {
		return args
	}
	return append(args,
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=60",
		"-o", "ControlPath="+controlPath,
	)
}

func RunSSH(ctx context.Context, executor Executor, t target.Target, command string, stdin io.Reader) (Result, error) {
	return executor.Run(ctx, "ssh", append(SSHArgs(t), command), stdin)
}

func Upload(ctx context.Context, executor Executor, t target.Target, localPath, remotePath string) error {
	args := append(SCPArgs(t), localPath, t.SSHUser+"@"+t.SSHHost+":"+remotePath)
	result, err := executor.Run(ctx, "scp", args, nil)
	if err != nil {
		return fmt.Errorf("upload %s: %w: %s", localPath, err, bytes.TrimSpace(result.Stderr))
	}
	return nil
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func ShellCommand(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = ShellQuote(arg)
	}
	return strings.Join(quoted, " ")
}
