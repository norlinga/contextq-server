package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

var ErrOutputLimit = errors.New("contextq output limit exceeded")

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Runner interface {
	Run(ctx context.Context, namespaceDir, contextqRoot string, args []string) (Result, error)
}

type CommandRunner struct {
	Binary      string
	OutputLimit int
}

func (r CommandRunner) Run(ctx context.Context, namespaceDir, contextqRoot string, args []string) (Result, error) {
	if r.Binary == "" {
		return Result{}, fmt.Errorf("contextq binary is required")
	}
	limit := r.OutputLimit
	if limit <= 0 {
		limit = 4 << 20
	}
	commandArgs := make([]string, 0, len(args)+3)
	commandArgs = append(commandArgs, "--json", "--root", contextqRoot)
	commandArgs = append(commandArgs, args...)

	cmd := exec.CommandContext(ctx, r.Binary, commandArgs...)
	cmd.Dir = namespaceDir
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
	}
	stdout := &boundedBuffer{limit: limit}
	stderr := &boundedBuffer{limit: limit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if stdout.truncated || stderr.truncated {
		return result, ErrOutputLimit
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

type boundedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return originalLen, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buf.Write(p)
	return originalLen, nil
}

func (b *boundedBuffer) Bytes() []byte {
	out := make([]byte, b.buf.Len())
	copy(out, b.buf.Bytes())
	return out
}
