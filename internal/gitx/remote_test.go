package gitx

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// newRemotePair returns two clones of one bare origin, so one side can push
// what the other fetches and pulls. The first clone already pushed a commit,
// which gives the bare repository a HEAD to clone and the second clone an
// upstream.
func newRemotePair(t *testing.T) (a, b *Runner) {
	t.Helper()
	base := t.TempDir()
	origin := filepath.Join(base, "origin.git")
	seed := New(base)
	if _, err := seed.Run(context.Background(), "init", "-q", "--bare", "-b", "main", origin); err != nil {
		t.Fatalf("init origin: %v", err)
	}
	a = New(filepath.Join(base, "a"))
	if _, err := seed.Run(context.Background(), "clone", "-q", origin, a.Dir); err != nil {
		t.Fatalf("clone a: %v", err)
	}
	configTestUser(t, a)
	writeFile(t, a, "readme.txt", "one\n")
	commitAll(t, a, "first")
	if err := a.Push(context.Background()); err != nil {
		t.Fatalf("push first commit: %v", err)
	}
	// The second clone is made after the first push, so both have a commit and
	// an upstream to work with.
	b = New(filepath.Join(base, "b"))
	if _, err := seed.Run(context.Background(), "clone", "-q", origin, b.Dir); err != nil {
		t.Fatalf("clone b: %v", err)
	}
	configTestUser(t, b)
	return a, b
}

func TestFetchMovesRemoteRefOnly(t *testing.T) {
	a, b := newRemotePair(t)
	ctx := context.Background()

	writeFile(t, a, "readme.txt", "two\n")
	commitAll(t, a, "second")
	if err := a.Push(ctx); err != nil {
		t.Fatalf("push: %v", err)
	}

	headBefore, err := b.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Fetch(ctx); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got := readFile(t, b, "readme.txt"); got != "one\n" {
		t.Fatalf("fetch changed the worktree: %q", got)
	}
	headAfter, err := b.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if string(headBefore) != string(headAfter) {
		t.Fatalf("fetch moved HEAD: %q -> %q", headBefore, headAfter)
	}
	remote, err := b.Run(ctx, "rev-parse", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	aHead, err := a.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(remote)) != strings.TrimSpace(string(aHead)) {
		t.Fatalf("origin/main = %q, want %q", remote, aHead)
	}
}

func TestPullFastForwardsWorktree(t *testing.T) {
	a, b := newRemotePair(t)
	ctx := context.Background()

	writeFile(t, a, "readme.txt", "two\n")
	commitAll(t, a, "second")
	if err := a.Push(ctx); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := b.Pull(ctx); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if got := readFile(t, b, "readme.txt"); got != "two\n" {
		t.Fatalf("pulled worktree = %q, want %q", got, "two\n")
	}
}

func TestPullRefusesDivergedBranches(t *testing.T) {
	a, b := newRemotePair(t)
	ctx := context.Background()

	writeFile(t, a, "readme.txt", "from a\n")
	commitAll(t, a, "a change")
	if err := a.Push(ctx); err != nil {
		t.Fatalf("push: %v", err)
	}
	writeFile(t, b, "readme.txt", "from b\n")
	commitAll(t, b, "b change")

	// --ff-only means a diverged branch fails instead of leaving a merge commit
	// or a conflicted worktree the app cannot continue.
	err := b.Pull(ctx)
	if err == nil {
		t.Fatal("pull of diverged branches did not fail")
	}
	if !strings.Contains(err.Error(), "fast-forward") {
		t.Fatalf("pull error = %v, want git's fast-forward message", err)
	}
	if got := readFile(t, b, "readme.txt"); got != "from b\n" {
		t.Fatalf("failed pull changed the worktree: %q", got)
	}
}

func TestPushIgnoresMatchingDefault(t *testing.T) {
	a, b := newRemotePair(t)
	ctx := context.Background()

	// A repository set to push every same-named branch must still publish only
	// the tracked branch from the app's Push button.
	if _, err := b.Run(ctx, "config", "push.default", "matching"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Run(ctx, "checkout", "-q", "-b", "side"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, b, "side.txt", "side\n")
	commitAll(t, b, "side")
	if _, err := b.Run(ctx, "checkout", "-q", "main"); err != nil {
		t.Fatal(err)
	}
	if err := b.Push(ctx); err != nil {
		t.Fatalf("push: %v", err)
	}
	if err := a.Fetch(ctx); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := a.Run(ctx, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/side"); err == nil {
		t.Fatal("push published the untracked side branch")
	}
}

func TestPushWithoutUpstreamFails(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "readme.txt", "one\n")
	commitAll(t, r, "first")
	// A remote exists, but main does not track a branch on it.
	origin := filepath.Join(t.TempDir(), "origin.git")
	if _, err := r.Run(context.Background(), "remote", "add", "origin", origin); err != nil {
		t.Fatalf("add remote: %v", err)
	}

	err := r.Push(context.Background())
	if err == nil {
		t.Fatal("push without an upstream did not fail")
	}
	if !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("push error = %v, want git's upstream message", err)
	}
}

// TestOutgoingWithUpstream lists exactly the commits the upstream lacks, and
// counts the ones it has that the branch does not.
func TestOutgoingWithUpstream(t *testing.T) {
	ctx := context.Background()
	a, b := newRemotePair(t)

	plan, err := a.Outgoing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Upstream == "" || plan.Total != 0 || len(plan.Commits) != 0 || plan.Behind != 0 {
		t.Fatalf("clean plan = %+v", plan)
	}

	writeFile(t, a, "a.txt", "1\n")
	commitAll(t, a, "second")
	writeFile(t, a, "a.txt", "2\n")
	commitAll(t, a, "third")
	// b pushes first, so a is both ahead and behind.
	writeFile(t, b, "b.txt", "b\n")
	commitAll(t, b, "from b")
	if err := b.Push(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Run(ctx, "fetch", "-q"); err != nil {
		t.Fatal(err)
	}
	plan, err = a.Outgoing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Total != 2 || len(plan.Commits) != 2 || plan.Behind != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Commits[0].Subject != "third" || plan.Commits[1].Subject != "second" {
		t.Fatalf("commits = %+v, want newest first", plan.Commits)
	}
	if plan.Branch != "main" || plan.Upstream != "origin/main" {
		t.Fatalf("branch/upstream = %q/%q", plan.Branch, plan.Upstream)
	}
}

// TestOutgoingWithoutUpstream lists what no remote has, picks the lone remote,
// and PushBranch then publishes the branch with its upstream set.
func TestOutgoingWithoutUpstream(t *testing.T) {
	ctx := context.Background()
	a, _ := newRemotePair(t)
	if _, err := a.Run(ctx, "checkout", "-q", "-b", "topic"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, a, "t.txt", "t\n")
	commitAll(t, a, "topic work")

	plan, err := a.Outgoing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Upstream != "" || plan.Remote != "origin" || plan.Total != 1 ||
		plan.Commits[0].Subject != "topic work" {
		t.Fatalf("plan = %+v", plan)
	}
	if err := a.PushBranch(ctx, plan.Remote, plan.Branch); err != nil {
		t.Fatal(err)
	}
	plan, err = a.Outgoing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Upstream != "origin/topic" || plan.Total != 0 {
		t.Fatalf("after publish: %+v", plan)
	}
}

func TestOutgoingCapAndRefusals(t *testing.T) {
	ctx := context.Background()
	a, _ := newRemotePair(t)
	for i := 0; i < maxOutgoing+3; i++ {
		writeFile(t, a, "n.txt", strings.Repeat("x", i+1))
		commitAll(t, a, "c")
	}
	plan, err := a.Outgoing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Total != maxOutgoing+3 || len(plan.Commits) != maxOutgoing {
		t.Fatalf("total=%d listed=%d", plan.Total, len(plan.Commits))
	}

	// Unknown remote and option-looking names never reach git.
	if err := a.PushBranch(ctx, "nowhere", "main"); err == nil || !strings.Contains(err.Error(), "unknown remote") {
		t.Fatalf("unknown remote err = %v", err)
	}
	if err := a.PushBranch(ctx, "--force", "main"); err == nil {
		t.Fatal("an option-looking remote was accepted")
	}
	if err := a.PushBranch(ctx, "origin", "-x"); err == nil {
		t.Fatal("an option-looking branch was accepted")
	}

	// Detached HEAD has no branch to push.
	if _, err := a.Run(ctx, "checkout", "-q", "--detach"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Outgoing(ctx); err == nil {
		t.Fatal("a detached HEAD produced a plan")
	}
}

func TestPublishRemoteChoice(t *testing.T) {
	for _, tc := range []struct {
		remotes []string
		want    string
	}{
		{nil, ""},
		{[]string{"upstream"}, "upstream"},
		{[]string{"fork", "origin"}, "origin"},
		{[]string{"fork", "upstream"}, ""},
	} {
		if got := publishRemote(tc.remotes); got != tc.want {
			t.Fatalf("publishRemote(%v) = %q, want %q", tc.remotes, got, tc.want)
		}
	}
}
