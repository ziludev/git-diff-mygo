package shell

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// commitMenu builds a history row's context menu. Merge commits get no
// cherry-pick or revert (they need a mainline choice the app cannot ask for),
// and the current tip gets no cherry-pick (it would be empty). Items that
// write stay disabled while a write or a remote operation runs, the same guard
// as every other write.
func (a *App) commitMenu(c *ui.Context, m *ui.Menu, commit gitx.Commit) {
	busy := a.busy()
	if m.Item("Copy Hash").Chosen() {
		c.WriteClipboard(commit.OID)
	}
	m.Separator()
	if m.Item("Checkout Revision").Disabled(busy).Chosen() {
		a.checkoutRevision(commit)
	}
	if m.Item("New Branch from Here…").Disabled(busy).Chosen() {
		a.branchAtCommit(commit)
	}
	if m.Item("Tag Here…").Disabled(busy).Chosen() {
		a.tagCommit(commit)
	}
	merge := len(commit.Parents) > 1
	if !merge {
		m.Separator()
		if commit.OID != a.status.Branch.OID && m.Item("Cherry-pick").Disabled(busy).Chosen() {
			a.cherryPick(commit)
		}
		if m.Item("Revert").Disabled(busy).Chosen() {
			a.revertCommit(commit)
		}
	}
	m.Separator()
	m.Submenu("Reset Current Branch to Here", func(sub *ui.Menu) {
		if sub.Item("Soft").Disabled(busy).Chosen() {
			a.resetTo(commit, gitx.ResetSoft)
		}
		if sub.Item("Mixed").Disabled(busy).Chosen() {
			a.resetTo(commit, gitx.ResetMixed)
		}
		if sub.Item("Hard…").Disabled(busy).Chosen() {
			a.resetTo(commit, gitx.ResetHard)
		}
	})
}

// commitLabel names a commit in a dialog: short hash and subject.
func commitLabel(commit gitx.Commit) string {
	subject := strings.TrimSpace(commit.Subject)
	if subject == "" {
		return commit.Short
	}
	return commit.Short + " " + subject
}

// checkoutRevision detaches HEAD at a commit. It goes through checkout so the
// selection that belonged to the old HEAD is dropped like on a branch switch.
func (a *App) checkoutRevision(commit gitx.Commit) {
	a.checkout(func() error { return a.svc.CheckoutRevision(context.Background(), commit.OID) })
}

// branchAtCommit asks for a name, then creates the branch at the commit and
// checks it out.
func (a *App) branchAtCommit(commit gitx.Commit) {
	a.promptAction(promptSpec{
		title:       "New branch at " + commitLabel(commit),
		detail:      "The branch is created at this commit and checked out.",
		label:       "Create",
		placeholder: "new-branch-name",
	}, func(name string, _ bool) {
		a.checkout(func() error {
			return a.svc.CreateBranchAt(context.Background(), name, commit.OID)
		})
	})
}

// tagCommit asks for a name, then adds a lightweight tag at the commit.
func (a *App) tagCommit(commit gitx.Commit) {
	a.promptAction(promptSpec{
		title:       "Tag " + commitLabel(commit),
		detail:      "A lightweight tag; git refuses a name that already exists.",
		label:       "Tag",
		placeholder: "tag-name",
	}, func(name string, _ bool) {
		a.write(func() error { return a.svc.Tag(context.Background(), name, commit.OID) })
	})
}

func (a *App) cherryPick(commit gitx.Commit) {
	a.write(func() error { return a.svc.CherryPick(context.Background(), commit.OID) })
}

func (a *App) revertCommit(commit gitx.Commit) {
	a.write(func() error { return a.svc.Revert(context.Background(), commit.OID) })
}

// resetTo moves the current branch to a commit. Only hard asks first: it
// discards uncommitted changes to tracked files, which nothing can bring back.
func (a *App) resetTo(commit gitx.Commit, mode gitx.ResetMode) {
	run := func() {
		a.write(func() error { return a.svc.Reset(context.Background(), commit.OID, mode) })
	}
	if mode != gitx.ResetHard {
		run()
		return
	}
	a.confirmAction("Hard reset to "+commitLabel(commit)+"?",
		"The branch moves to this commit and uncommitted changes to tracked files are discarded. "+
			"Untracked files stay. The old tip can be recovered from the reflog.",
		"Reset", run)
}
