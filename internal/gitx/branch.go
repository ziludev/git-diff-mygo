package gitx

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// BranchEntry is one row of the branch switcher. Remote marks a
// remote-tracking branch, whose Name keeps the remote prefix (origin/main) and
// whose RemoteName is the configured remote it came from (origin). Sync counts
// belong to the checked-out branch only and come from Status, which already
// asks git for them; per-branch counts would make for-each-ref walk every
// branch's history.
type BranchEntry struct {
	Name       string `json:"name"`
	Subject    string `json:"subject"`
	Time       int64  `json:"time"`
	Current    bool   `json:"current"`
	Remote     bool   `json:"remote"`
	RemoteName string `json:"remoteName,omitempty"`
}

const (
	refsHeads   = "refs/heads/"
	refsRemotes = "refs/remotes/"
)

// branchFormat asks for one record per branch. for-each-ref expands %xx as a
// hex byte: %00 starts a record and %1f separates its fields. A commit subject
// is arbitrary text and may contain %1f or %1e, so only NUL is safe as the
// record separator, and the subject must be the last field to keep any %1f it
// has. %(symref) is the target of a symbolic ref, which drops origin/HEAD: it
// is a pointer, not a branch.
const branchFormat = "%00%(refname)%1f%(HEAD)%1f%(committerdate:unix)%1f%(symref)%1f%(contents:subject)"

// Branches lists local and remote-tracking branches: the checked-out one
// first, then the locals by name, then the remotes. The full ref name tells
// the two kinds apart, which %(refname:short) cannot do for a remote.
func (r *Runner) Branches(ctx context.Context) ([]BranchEntry, error) {
	remotes := r.remoteNames(ctx)
	out, err := r.Run(ctx, "for-each-ref", "--format="+branchFormat, "refs/heads", "refs/remotes")
	if err != nil {
		return []BranchEntry{}, err
	}
	return parseBranches(out, remotes), nil
}

// remoteNames lists the configured remotes, longest first so the longest
// matching name wins. A remote's name may itself contain "/" (git accepts
// `git remote add foo/bar <url>`), so the remote of a tracking ref cannot be
// guessed from its first segment.
func (r *Runner) remoteNames(ctx context.Context) []string {
	out, err := r.Run(ctx, "remote")
	if err != nil {
		// Without the list, LocalName falls back to the first segment; that
		// only guesses wrong for a remote whose name contains "/".
		return nil
	}
	names := strings.Fields(string(out))
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	return names
}

// remoteOf returns the configured remote a tracking ref name belongs to, or ""
// when none matches (as after a remote was removed).
func remoteOf(name string, remotes []string) string {
	for _, remote := range remotes {
		if strings.HasPrefix(name, remote+"/") {
			return remote
		}
	}
	return ""
}

// parseBranches turns for-each-ref output into entries in display order.
func parseBranches(out []byte, remotes []string) []BranchEntry {
	entries := []BranchEntry{}
	for _, f := range splitRecords(out, 5) {
		if f[3] != "" {
			continue // a symbolic ref, as origin/HEAD
		}
		ref := f[0]
		entry := BranchEntry{Current: f[1] == "*"}
		switch {
		case strings.HasPrefix(ref, refsHeads):
			entry.Name = ref[len(refsHeads):]
		case strings.HasPrefix(ref, refsRemotes):
			entry.Name = ref[len(refsRemotes):]
			entry.Remote = true
			entry.RemoteName = remoteOf(entry.Name, remotes)
		default:
			continue
		}
		if entry.Name == "" {
			continue
		}
		entry.Time, _ = strconv.ParseInt(f[2], 10, 64)
		entry.Subject = f[4]
		entries = append(entries, entry)
	}
	// The checked-out branch stays on top so the list does not move under the
	// pointer when another branch is checked out.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Current != entries[j].Current {
			return entries[i].Current
		}
		if entries[i].Remote != entries[j].Remote {
			return !entries[i].Remote
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// LocalName is the local branch a remote-tracking branch maps to: its Name
// with the remote's name removed, so origin/feature/x is feature/x. A local
// branch returns its own name.
func (b BranchEntry) LocalName() string {
	if !b.Remote {
		return b.Name
	}
	var remotes []string
	if b.RemoteName != "" {
		remotes = []string{b.RemoteName}
	}
	return localName(b.Name, remotes)
}

// localName maps a remote-tracking ref name to its local branch name. remotes
// comes from remoteNames, longest first; a name without a matching remote
// returns itself, which callers treat as "no remote part".
func localName(remoteName string, remotes []string) string {
	if remote := remoteOf(remoteName, remotes); remote != "" {
		return remoteName[len(remote)+1:]
	}
	if i := strings.IndexByte(remoteName, '/'); i >= 0 {
		return remoteName[i+1:]
	}
	return remoteName
}

// CheckoutBranch switches the work tree to a local branch. A dirty work tree
// fails exactly when git would overwrite local changes; nothing is hidden.
func (r *Runner) CheckoutBranch(ctx context.Context, name string) error {
	if err := checkBranchName(name); err != nil {
		return err
	}
	// --no-guess keeps checkout from inventing a tracking branch from a
	// remote ref: the panel only offers local branches.
	_, err := r.Run(ctx, "checkout", "--no-guess", name)
	return err
}

// CheckoutRemoteBranch creates the local branch that corresponds to a
// remote-tracking one and checks it out, with the remote as its upstream:
// origin/feature/x becomes feature/x tracking origin/feature/x.
//
// git refuses when the local branch already exists. The caller checks its list
// first and checks that branch out instead, so the collision is a normal path,
// not the error it would be here.
func (r *Runner) CheckoutRemoteBranch(ctx context.Context, name string) error {
	if err := checkBranchName(name); err != nil {
		return err
	}
	remotes := r.remoteNames(ctx)
	local := localName(name, remotes)
	// Without a remote prefix there is nothing to track; a plain checkout
	// would quietly make a detached HEAD or reuse a local branch.
	if local == name {
		return fmt.Errorf("invalid remote branch %q", name)
	}
	if err := checkBranchName(local); err != nil {
		return err
	}
	// Qualify the start point: a local branch or tag that happens to be named
	// origin/main must not make the short name ambiguous.
	ref := name
	if !strings.HasPrefix(ref, refsRemotes) {
		ref = refsRemotes + name
	}
	// --track refuses a ref whose remote is not configured, which happens in
	// fixtures and after a remote is removed. Creating the branch first always
	// works; the upstream link is then best effort, and a failure only means
	// pull and push ask for an upstream explicitly.
	if _, err := r.Run(ctx, "checkout", "-b", local, ref); err != nil {
		return err
	}
	_, _ = r.Run(ctx, "branch", "--set-upstream-to="+ref, local)
	return nil
}

// CreateBranch creates name at HEAD and checks it out in one step, so the new
// branch is ready to commit on.
func (r *Runner) CreateBranch(ctx context.Context, name string) error {
	if err := checkBranchName(name); err != nil {
		return err
	}
	_, err := r.Run(ctx, "checkout", "-b", name)
	return err
}

// DeleteBranch removes a branch. git refuses one whose commits are not merged
// elsewhere; the panel surfaces that message rather than forcing the delete.
func (r *Runner) DeleteBranch(ctx context.Context, name string) error {
	if err := checkBranchName(name); err != nil {
		return err
	}
	_, err := r.Run(ctx, "branch", "-d", name)
	return err
}

// checkBranchName rejects the names git would read as options. Everything
// else is left to git, which validates the ref itself.
func checkBranchName(name string) error {
	if strings.TrimSpace(name) == "" || strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid branch name %q", name)
	}
	return nil
}
