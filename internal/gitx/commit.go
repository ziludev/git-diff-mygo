package gitx

import (
	"context"
	"errors"
	"strings"
)

// CommitRequest is one commit built from the index.
type CommitRequest struct {
	Message string `json:"message"`
	Amend   bool   `json:"amend"`
}

// Commit creates a commit from the index and returns its object id. Hooks and
// signing run as configured in the repository; the message is passed with -m,
// so git never opens an editor.
func (r *Runner) Commit(ctx context.Context, req CommitRequest) (string, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return "", errors.New("a commit message is required")
	}
	args := []string{"commit", "-m", message}
	if req.Amend {
		args = append(args, "--amend")
	}
	if _, err := r.Run(ctx, args...); err != nil {
		return "", err
	}
	out, err := r.Run(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
