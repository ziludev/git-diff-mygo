package gitx

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

// maxOutgoing caps the commits Outgoing lists; Total still counts them all.
const maxOutgoing = 50

// OutgoingPlan is what a push would send, read before the user commits to it.
type OutgoingPlan struct {
	// Branch is the checked-out branch.
	Branch string `json:"branch"`
	// Upstream is the tracked branch ("origin/main"); empty when the branch
	// has none, in which case Commits are the ones no remote has yet.
	Upstream string `json:"upstream,omitempty"`
	// Remote is where Publish would push a branch without an upstream: the
	// only remote, or origin among several. Empty when there is no safe
	// choice, and Remotes says why. Both are read only for a branch that has
	// no upstream.
	Remote  string   `json:"remote,omitempty"`
	Remotes []string `json:"remotes"`
	// Commits are the first maxOutgoing outgoing commits, newest first, and
	// Total how many there are. Behind is how many upstream commits the branch
	// lacks, which makes git refuse a plain push.
	Commits []Commit `json:"commits"`
	Total   int      `json:"total"`
	Behind  int      `json:"behind"`
}

// Outgoing reads what a push of the current branch would publish. It never
// touches the network or the work tree.
func (r *Runner) Outgoing(ctx context.Context) (OutgoingPlan, error) {
	plan := OutgoingPlan{Commits: []Commit{}, Remotes: []string{}}
	out, err := r.Run(ctx, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return plan, errors.New("HEAD is detached: there is no branch to push")
	}
	plan.Branch = strings.TrimSpace(string(out))
	if !r.HeadExists(ctx) {
		return plan, errors.New("the branch has no commits yet")
	}
	if up, upErr := r.Run(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); upErr == nil {
		plan.Upstream = strings.TrimSpace(string(up))
	}

	var rng []string
	counted := false
	if plan.Upstream != "" {
		rng = []string{plan.Upstream + "..HEAD"}
		// One call gives both directions: left is what HEAD has and the
		// upstream lacks (what a push sends), right the reverse.
		if counts, countErr := r.Run(ctx, "rev-list", "--left-right", "--count", "HEAD..."+plan.Upstream); countErr == nil {
			if ahead, behind, ok := parseCounts(string(counts)); ok {
				plan.Total, plan.Behind, counted = ahead, behind, true
			}
		}
	} else {
		// The remotes are only needed to name where a new branch would
		// publish; a tracked branch does not read them at all.
		plan.Remotes = r.remoteNames(ctx)
		plan.Remote = publishRemote(plan.Remotes)
		rng = []string{"HEAD", "--not", "--remotes"}
	}
	if !counted {
		count, err := r.Run(ctx, append([]string{"rev-list", "--count"}, rng...)...)
		if err != nil {
			return plan, err
		}
		plan.Total, _ = strconv.Atoi(strings.TrimSpace(string(count)))
	}
	log, err := r.Run(ctx, append(append([]string{"log", "--no-color", "--no-show-signature",
		"--max-count=" + strconv.Itoa(maxOutgoing), logFormat}, rng...), "--")...)
	if err != nil {
		return plan, err
	}
	plan.Commits = parseLog(log)
	return plan, nil
}

// parseCounts reads the "ahead<TAB>behind" pair of rev-list --left-right
// --count.
func parseCounts(s string) (ahead, behind int, ok bool) {
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0, 0, false
	}
	ahead, errA := strconv.Atoi(fields[0])
	behind, errB := strconv.Atoi(fields[1])
	if errA != nil || errB != nil {
		return 0, 0, false
	}
	return ahead, behind, true
}

// publishRemote picks the remote a new branch goes to: the only one, or origin
// among several. Anything else is left to the user, since a guess would
// publish the work to the wrong place.
func publishRemote(remotes []string) string {
	if len(remotes) == 1 {
		return remotes[0]
	}
	for _, name := range remotes {
		if name == "origin" {
			return name
		}
	}
	return ""
}

// PushBranch publishes a branch to a remote and sets it as the upstream. The
// refspec is explicit, so push.default cannot widen it.
func (r *Runner) PushBranch(ctx context.Context, remote, branch string) error {
	if err := checkBranchName(remote); err != nil {
		return err
	}
	if err := checkBranchName(branch); err != nil {
		return err
	}
	known := false
	for _, name := range r.remoteNames(ctx) {
		known = known || name == remote
	}
	if !known {
		return fmt.Errorf("unknown remote %q", remote)
	}
	spec := "refs/heads/" + branch + ":refs/heads/" + branch
	_, err := r.Run(ctx, "push", "--set-upstream", remote, spec)
	return err
}
