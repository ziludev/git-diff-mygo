// Package gitx wraps the git command line for the app's repository views.
// It is the only package that starts git processes.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Error is a failed git command. Stderr is kept for the UI; it never
// contains credentials because the commands are read-only by default.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Stderr)
	}
	return fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

// Runner runs git commands in one work tree.
type Runner struct {
	Dir string
}

// New returns a runner for the repository at dir.
func New(dir string) *Runner { return &Runner{Dir: dir} }

// PathChange identifies one row of the working tree. OrigPath is the old side
// of a rename or copy.
type PathChange struct {
	Path     string `json:"path"`
	OrigPath string `json:"origPath,omitempty"`
}

// Paths returns path and, when it differs, origPath. Rename and copy rows
// carry both sides, and commands that reset or add them need both.
func Paths(path, origPath string) []string {
	if origPath == "" || origPath == path {
		return []string{path}
	}
	return []string{path, origPath}
}

// Discover returns the top level of the work tree containing path.
func Discover(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(abs); statErr == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	out, err := New(abs).Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New("not a git repository")
	}
	return root, nil
}

// Run runs git with args and returns its standard output.
func (r *Runner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return r.RunInput(ctx, nil, args...)
}

// RunInput runs git with stdin and args, and returns its standard output.
func (r *Runner) RunInput(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	full := append([]string{
		"-c", "core.pager=cat",
		"-c", "core.quotepath=false",
		"--no-pager",
	}, args...)

	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.Dir
	cmd.Env = gitEnv()
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &Error{
			Args:   args,
			Stderr: strings.TrimSpace(stderr.String()),
			Err:    err,
		}
	}
	return stdout.Bytes(), nil
}

// gitEnv isolates the child from user settings that could execute commands
// (pager, external diff, textconv) or change output encoding, while keeping
// credential helpers and SSH agents available for later network operations.
func gitEnv() []string {
	drop := map[string]bool{
		"LC_ALL":              true,
		"LANG":                true,
		"GIT_OPTIONAL_LOCKS":  true,
		"GIT_TERMINAL_PROMPT": true,
	}
	env := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && drop[kv[:i]] {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"LC_ALL=C",
		"LANG=C",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
}
