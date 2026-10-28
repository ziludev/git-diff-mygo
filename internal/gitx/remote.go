package gitx

import (
	"context"
)

// Fetch updates the remote-tracking refs of the current branch's remote and
// prunes the ones the remote deleted. It changes no local branch and no
// worktree file.
func (r *Runner) Fetch(ctx context.Context) error {
	_, err := r.Run(ctx, "fetch", "--prune")
	return err
}

// Pull fast-forwards the current branch to its upstream. --ff-only is the one
// strategy the app can honour without an editor or a strategy prompt: it never
// creates a merge commit or a conflicted state, and a diverged branch fails
// with git's "Not possible to fast-forward" message.
func (r *Runner) Pull(ctx context.Context) error {
	_, err := r.Run(ctx, "pull", "--ff-only")
	return err
}

// Push publishes the current branch to its upstream. It never forces and never
// guesses a remote: a branch without an upstream fails with git's message.
func (r *Runner) Push(ctx context.Context) error {
	// Explicit defaults: one click publishes the tracked branch and nothing
	// else. push.default=matching would otherwise push every same-named local
	// branch, and push.autoSetupRemote would silently create the upstream this
	// app requires the user to set.
	_, err := r.Run(ctx,
		"-c", "push.default=upstream",
		"-c", "push.autoSetupRemote=false",
		"push")
	return err
}
