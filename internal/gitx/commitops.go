package gitx

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// oidPattern is what a commit id looks like from the app's own log. Checking it
// again here keeps a value that starts with "-" from ever being read as an
// option by git.
var oidPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

func checkOID(oid string) error {
	if !oidPattern.MatchString(oid) {
		return fmt.Errorf("invalid commit id %q", oid)
	}
	return nil
}

func shortOID(oid string) string {
	if len(oid) > 7 {
		return oid[:7]
	}
	return oid
}

// CheckoutRevision detaches HEAD at a commit. A dirty work tree fails exactly
// when git would overwrite local changes.
func (r *Runner) CheckoutRevision(ctx context.Context, oid string) error {
	if err := checkOID(oid); err != nil {
		return err
	}
	_, err := r.Run(ctx, "checkout", "--detach", oid)
	return err
}

// CreateBranchAt creates a branch at a commit and checks it out.
func (r *Runner) CreateBranchAt(ctx context.Context, name, oid string) error {
	if err := checkBranchName(name); err != nil {
		return err
	}
	if err := checkOID(oid); err != nil {
		return err
	}
	_, err := r.Run(ctx, "checkout", "-b", name, oid)
	return err
}

// Tag creates a lightweight tag at a commit. An existing tag is git's error,
// never overwritten.
func (r *Runner) Tag(ctx context.Context, name, oid string) error {
	if err := checkBranchName(name); err != nil {
		return err
	}
	if err := checkOID(oid); err != nil {
		return err
	}
	_, err := r.Run(ctx, "tag", name, oid)
	return err
}

// CherryPick applies a commit onto the current branch. See sequence for what
// happens when it stops.
func (r *Runner) CherryPick(ctx context.Context, oid string) error {
	if err := checkOID(oid); err != nil {
		return err
	}
	return r.sequence(ctx, "cherry-pick", "CHERRY_PICK_HEAD", oid)
}

// Revert adds a commit that undoes oid. The message is git's default, so git
// never opens an editor.
func (r *Runner) Revert(ctx context.Context, oid string) error {
	if err := checkOID(oid); err != nil {
		return err
	}
	return r.sequence(ctx, "revert", "REVERT_HEAD", oid, "--no-edit")
}

// sequence runs a cherry-pick or revert. The app has no conflict editor, so a
// stop with the operation in progress (a conflict, or a pick that turned out
// empty) is aborted and reported: the repository is left as it was instead of
// half applied. A refusal before anything started, such as a dirty work tree,
// leaves nothing to abort and keeps git's own message.
func (r *Runner) sequence(ctx context.Context, op, marker, oid string, extra ...string) error {
	// The operation name and the commit id cannot drift: the command is built
	// from op, extra and oid, in that order.
	args := append(append([]string{op}, extra...), oid)
	_, err := r.Run(ctx, args...)
	if err == nil {
		return nil
	}
	if _, markErr := r.Run(ctx, "rev-parse", "--verify", "--quiet", marker); markErr != nil {
		return err
	}
	if _, abortErr := r.Run(ctx, op, "--abort"); abortErr != nil {
		return fmt.Errorf("%s of %s stopped and could not be aborted: %w", op, shortOID(oid), abortErr)
	}
	return fmt.Errorf("%s of %s stopped (conflict or nothing to apply) and was aborted; nothing changed: %s",
		op, shortOID(oid), firstLine(err))
}

// firstLine is the first line of git's complaint.
func firstLine(err error) string {
	var ge *Error
	msg := err.Error()
	if errors.As(err, &ge) && ge.Stderr != "" {
		msg = ge.Stderr
	}
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}

// ResetMode is how far a reset reaches: the branch pointer only (soft), plus
// the index (mixed), or plus the work tree (hard).
type ResetMode string

const (
	ResetSoft  ResetMode = "soft"
	ResetMixed ResetMode = "mixed"
	ResetHard  ResetMode = "hard"
)

// Reset moves the current branch (or the detached HEAD) to a commit. Hard
// discards uncommitted work, so callers confirm first.
func (r *Runner) Reset(ctx context.Context, oid string, mode ResetMode) error {
	if err := checkOID(oid); err != nil {
		return err
	}
	switch mode {
	case ResetSoft, ResetMixed, ResetHard:
	default:
		return fmt.Errorf("invalid reset mode %q", mode)
	}
	_, err := r.Run(ctx, "reset", "--"+string(mode), oid)
	return err
}
