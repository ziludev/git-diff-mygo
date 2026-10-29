package shell

import (
	"context"
	"strconv"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// pushDialog is the preview shown before anything leaves the machine: what the
// push would send, read when the dialog opened.
type pushDialog struct {
	loading bool
	err     string
	plan    gitx.OutgoingPlan
}

// openPush shows the push dialog and reads what a push would publish. The read
// is async like every other git call, and its answer is dropped when the
// dialog was closed or the repository changed meanwhile.
func (a *App) openPush() {
	if a.busy() || a.status.Root == "" {
		return
	}
	root := a.status.Root
	epoch := a.epochs.push.Add(1)
	dlg := &pushDialog{loading: true}
	a.push = dlg
	a.pushOpen = true
	go func() {
		plan, err := a.svc.Outgoing(context.Background())
		a.apply(func() {
			if epoch != a.epochs.push.Load() || a.status.Root != root || a.push != dlg {
				return
			}
			dlg.loading = false
			if err != nil {
				dlg.err = err.Error()
				return
			}
			dlg.plan = plan
		})
	}()
}

// closePush hides the dialog and drops a read still in flight.
func (a *App) closePush() {
	a.pushOpen = false
	a.push = nil
	a.epochs.push.Add(1)
}

// canPush mirrors the dialog's button: something to send, somewhere to send
// it, and no write or network operation running.
func (a *App) canPush() bool {
	d := a.push
	if d == nil || d.loading || d.err != "" || a.busy() {
		return false
	}
	return d.plan.Total > 0 && (d.plan.Upstream != "" || d.plan.Remote != "")
}

// runPush starts the push the dialog described. A branch with an upstream is a
// plain push; one without is published to the remote the plan chose, which
// sets the upstream.
func (a *App) runPush() {
	if !a.canPush() {
		return
	}
	plan := a.push.plan
	a.closePush()
	if plan.Upstream != "" {
		a.pushRemote()
		return
	}
	a.syncRemote("push", func(ctx context.Context) error {
		return a.svc.Publish(ctx, plan.Remote, plan.Branch)
	})
}

// pushTitle is the dialog's heading: the branch and where it goes.
func pushTitle(d *pushDialog) string {
	branch := d.plan.Branch
	switch {
	case branch == "":
		return "Push"
	case d.plan.Upstream != "":
		return "Push " + branch + " → " + d.plan.Upstream
	case d.plan.Remote != "":
		return "Publish " + branch + " → " + d.plan.Remote + "/" + branch
	}
	return "Publish " + branch
}

// pushNote explains what the plan cannot show in the list: no remote to
// publish to, a branch behind its upstream, or nothing to send.
func pushNote(d *pushDialog) string {
	p := d.plan
	switch {
	case p.Upstream == "" && p.Remote == "" && len(p.Remotes) == 0:
		return "This branch has no upstream and the repository has no remote."
	case p.Upstream == "" && p.Remote == "":
		return "This branch has no upstream and there are several remotes, none called origin. " +
			"Set the upstream in a terminal."
	case p.Total == 0:
		return "Nothing to push."
	case p.Behind > 0:
		return "The upstream has " + strconv.Itoa(p.Behind) +
			" commit(s) this branch lacks; git will refuse the push until you pull."
	case p.Upstream == "":
		return "No upstream yet: publishing pushes to " + p.Remote + " and sets it."
	}
	return ""
}

// pushModal draws the dialog.
func (a *App) pushModal(c *ui.Context) {
	t := c.Theme()
	d := a.push
	if d == nil {
		a.pushOpen = false
		return
	}
	ui.Modal(c, &a.pushOpen, func() {
		ui.Column(c).Width(460).Gap(10).Padding(16).Background(t.Background).
			Radius(t.Radius+2).Border(1, t.Border).Children(func() {
			ui.Text(c, pushTitle(d)).Bold().FontSize(13)
			switch {
			case d.loading:
				ui.Text(c, "Reading the commits to push…").FontSize(11).TextColor(t.TextMuted)
			case d.err != "":
				ui.Text(c, d.err).FontSize(11).TextColor(t.Danger).Wrap()
			default:
				if note := pushNote(d); note != "" {
					ui.Text(c, note).FontSize(11).TextColor(t.TextMuted).Wrap()
				}
				a.pushCommits(c, d)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				cancel := ui.Button(c, "Cancel")
				cancel.OnClick(a.closePush)
				label := "Push"
				if d.plan.Upstream == "" {
					label = "Publish"
				}
				run := ui.PrimaryButton(c, label).Disabled(!a.canPush())
				run.OnClick(a.runPush)
			})
		})
	})
	if a.push != nil && !a.pushOpen {
		// Escape or a press outside dismissed the modal; drop the dialog with
		// the flag so the two fields never disagree.
		a.closePush()
	}
}

// pushCommits lists the outgoing commits, newest first, inside a scroll area
// so a long list cannot outgrow the window.
func (a *App) pushCommits(c *ui.Context, d *pushDialog) {
	t := c.Theme()
	if len(d.plan.Commits) == 0 {
		return
	}
	ui.Scroll(c).FillWidth().MaxHeight(240).Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			for _, commit := range d.plan.Commits {
				ui.Row(c).FillWidth().Height(24).Gap(8).Children(func() {
					ui.Text(c, commit.Short).Font(mono()).FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, commit.Subject).FontSize(12).Grow(1).SingleLine()
				})
			}
			if more := d.plan.Total - len(d.plan.Commits); more > 0 {
				ui.Text(c, "and "+strconv.Itoa(more)+" more").FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
}
