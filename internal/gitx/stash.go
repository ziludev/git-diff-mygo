package gitx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// StashEntry is one stash. It is identified by its commit id: positions in the
// list shift whenever a stash is pushed or dropped.
type StashEntry struct {
	OID     string `json:"oid"`
	Short   string `json:"short"`
	Message string `json:"message"`
	Time    int64  `json:"time"`
}

// Stashes lists the stashes, newest first. A repository without any is an
// empty list.
func (r *Runner) Stashes(ctx context.Context) ([]StashEntry, error) {
	out, err := r.Run(ctx, "stash", "list", "--format=%H%x1f%ct%x1f%gs%x00")
	if err != nil {
		return []StashEntry{}, err
	}
	entries := []StashEntry{}
	for _, f := range splitRecords(out, 3) {
		when, _ := strconv.ParseInt(f[1], 10, 64)
		entries = append(entries, StashEntry{
			OID: f[0], Short: shortOID(f[0]), Time: when, Message: stashMessage(f[2]),
		})
	}
	return entries, nil
}

// stashMessage trims git's framing from a stash's reflog subject: a named
// stash ("On main: my note") shows the user's text, a default one
// ("WIP on main: abc1234 subject") shows "main: abc1234 subject".
func stashMessage(subject string) string {
	if rest, ok := strings.CutPrefix(subject, "On "); ok {
		if _, msg, found := strings.Cut(rest, ": "); found {
			return msg
		}
	}
	return strings.TrimPrefix(subject, "WIP on ")
}

// StashPush puts the uncommitted changes aside. git exits successfully when
// there was nothing to save, which would look like a stash that never
// appears; the top stash before and after tells the two apart.
func (r *Runner) StashPush(ctx context.Context, message string, untracked bool) error {
	before := r.stashTop(ctx)
	args := []string{"stash", "push"}
	if untracked {
		args = append(args, "--include-untracked")
	}
	if m := strings.TrimSpace(message); m != "" {
		args = append(args, "-m", m)
	}
	if _, err := r.Run(ctx, args...); err != nil {
		return err
	}
	if r.stashTop(ctx) == before {
		return errors.New("No local changes to save")
	}
	return nil
}

// stashTop is the id of the newest stash, "" when there is none.
func (r *Runner) stashTop(ctx context.Context) string {
	out, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", "refs/stash")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// StashApply puts a stash's changes back and keeps the stash. A conflict
// stays in the work tree: nothing here can undo it without risking the
// changes that were already there, so the caller says so.
func (r *Runner) StashApply(ctx context.Context, oid string) error {
	if err := checkOID(oid); err != nil {
		return err
	}
	_, err := r.Run(ctx, "stash", "apply", oid)
	if err == nil {
		return nil
	}
	// A refusal (local changes in the way) touched nothing; a conflict did,
	// and the user needs to know the work tree now holds markers.
	if out, listErr := r.Run(ctx, "diff", "--name-only", "--diff-filter=U"); listErr == nil &&
		strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("the stash was applied with conflicts and kept; "+
			"resolve them outside the app: %s", firstLine(err))
	}
	return err
}

// StashPop applies a stash and, only when that went through cleanly, drops it.
func (r *Runner) StashPop(ctx context.Context, oid string) error {
	if err := r.StashApply(ctx, oid); err != nil {
		return err
	}
	if err := r.StashDrop(ctx, oid); err != nil {
		return fmt.Errorf("the changes were applied, but the stash could not be dropped: %w", err)
	}
	return nil
}

// StashDrop removes a stash. drop needs its position, which is looked up from
// the id right now, so a stale click cannot drop another stash.
func (r *Runner) StashDrop(ctx context.Context, oid string) error {
	if err := checkOID(oid); err != nil {
		return err
	}
	entries, err := r.Stashes(ctx)
	if err != nil {
		return err
	}
	for i, entry := range entries {
		if entry.OID == oid {
			_, err := r.Run(ctx, "stash", "drop", "-q", fmt.Sprintf("stash@{%d}", i))
			return err
		}
	}
	return errors.New("stash no longer exists")
}
