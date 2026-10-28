package gitx

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// BranchEntry is one row of the branch switcher. Sync counts belong to the
// checked-out branch only and come from Status, which already asks git for
// them; per-branch counts would make for-each-ref walk every branch's history.
type BranchEntry struct {
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Time    int64  `json:"time"`
	Current bool   `json:"current"`
}

// branchFormat asks for one record per branch. for-each-ref expands %xx as a
// hex byte: %00 starts a record and %1f separates its fields. A commit subject
// is arbitrary text and may contain %1f or %1e, so only NUL is safe as the
// record separator; the subject is the last field and keeps any %1f it has.
const branchFormat = "%00%(refname:short)%1f%(HEAD)%1f%(committerdate:unix)%1f%(contents:subject)"

// Branches lists local branches, the checked-out one first and the rest by
// name. Remote-tracking branches are deliberately absent: checking one out
// needs a tracking decision the panel does not ask for yet.
func (r *Runner) Branches(ctx context.Context) ([]BranchEntry, error) {
	out, err := r.Run(ctx, "for-each-ref", "--format="+branchFormat, "refs/heads")
	if err != nil {
		return []BranchEntry{}, err
	}
	return parseBranches(out), nil
}

// parseBranches turns for-each-ref output into entries in display order.
func parseBranches(out []byte) []BranchEntry {
	entries := []BranchEntry{}
	for _, f := range splitRecords(out, 4) {
		time, _ := strconv.ParseInt(f[2], 10, 64)
		entries = append(entries, BranchEntry{
			Name:    f[0],
			Current: f[1] == "*",
			Time:    time,
			Subject: f[3],
		})
	}
	// The checked-out branch stays on top so the list does not move under the
	// pointer when another branch is checked out.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Current != entries[j].Current {
			return entries[i].Current
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
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
