package remote

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/norlinga/contextq-server/internal/target"
)

type Session struct {
	target target.Target
	dir    string
}

func NewSession(t target.Target) (*Session, error) {
	dir, err := os.MkdirTemp("", "contextq-ssh-")
	if err != nil {
		return nil, fmt.Errorf("create SSH control directory: %w", err)
	}
	t.ControlPath = filepath.Join(dir, "control")
	return &Session{target: t, dir: dir}, nil
}

func (s *Session) Target() target.Target {
	return s.target
}

func (s *Session) Close(ctx context.Context, executor Executor) error {
	defer os.RemoveAll(s.dir)
	args := []string{}
	if s.target.SSHPort != 0 {
		args = append(args, "-p", strconv.Itoa(s.target.SSHPort))
	}
	if s.target.Identity != "" {
		args = append(args, "-i", s.target.Identity)
	}
	args = append(args, "-S", s.target.ControlPath, "-O", "exit", s.target.SSHUser+"@"+s.target.SSHHost)
	result, err := executor.Run(ctx, "ssh", args, nil)
	if err != nil && len(result.Stderr) > 0 {
		return fmt.Errorf("close SSH multiplex session: %w: %s", err, result.Stderr)
	}
	return err
}
