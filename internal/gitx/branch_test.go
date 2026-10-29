package gitx

import (
	"context"
	"strings"
	"testing"
)

func TestParseBranches(t *testing.T) {
	// branchFormat's five fields, with full ref names as for-each-ref prints
	// them. The subject comes last so an embedded %1f cannot split the record.
	record := func(ref, head, when, symref, subject string) string {
		return "\x00" + strings.Join([]string{ref, head, when, symref, subject}, "\x1f") + "\n"
	}
	out := record("refs/heads/main", "*", "1700000000", "", "Base work") +
		// A subject is arbitrary text: control bytes must not split the record.
		record("refs/heads/topic", " ", "1690000000", "", "Topic\x1ework\x1fstill one field") +
		record("refs/remotes/origin/main", " ", "1680000000", "", "Remote work") +
		// A remote's name may contain "/", so the longest match wins.
		record("refs/remotes/foo/bar/topic", " ", "1670000000", "", "Slash remote") +
		// A symbolic ref points at another ref and is not a branch.
		record("refs/remotes/origin/HEAD", " ", "0", "refs/remotes/origin/main", "")
	entries := parseBranches([]byte(out), []string{"foo/bar", "origin"})
	if len(entries) != 4 {
		t.Fatalf("entries = %+v", entries)
	}
	if e := entries[0]; !e.Current || e.Name != "main" || e.Time != 1700000000 ||
		e.Subject != "Base work" || e.Remote {
		t.Fatalf("main = %+v", e)
	}
	if e := entries[1]; e.Current || e.Name != "topic" || e.Remote ||
		e.Subject != "Topic\x1ework\x1fstill one field" {
		t.Fatalf("topic = %+v", e)
	}
	// Locals sort before remotes, and remotes by name: foo/bar/topic before
	// origin/main.
	if e := entries[2]; !e.Remote || e.Name != "foo/bar/topic" || e.RemoteName != "foo/bar" {
		t.Fatalf("slash remote = %+v", e)
	}
	if e := entries[3]; !e.Remote || e.Name != "origin/main" || e.Current ||
		e.RemoteName != "origin" {
		t.Fatalf("remote = %+v", e)
	}
	if got := entries[2].LocalName(); got != "topic" {
		t.Fatalf("slash remote local name = %q, want topic", got)
	}
	if got := entries[3].LocalName(); got != "main" {
		t.Fatalf("remote local name = %q, want main", got)
	}
}

func TestBranchLocalName(t *testing.T) {
	for _, tc := range []struct {
		entry BranchEntry
		want  string
	}{
		{BranchEntry{Name: "main"}, "main"},
		{BranchEntry{Name: "origin/main", Remote: true, RemoteName: "origin"}, "main"},
		// Only the remote is stripped; the rest of the branch name stays.
		{BranchEntry{Name: "origin/feature/login", Remote: true, RemoteName: "origin"}, "feature/login"},
		{BranchEntry{Name: "upstream/main", Remote: true, RemoteName: "upstream"}, "main"},
		// A remote whose own name contains "/" is stripped whole.
		{BranchEntry{Name: "foo/bar/topic", Remote: true, RemoteName: "foo/bar"}, "topic"},
		// Without a recorded remote (it was removed), the first segment is the
		// best guess.
		{BranchEntry{Name: "origin/main", Remote: true}, "main"},
		// A remote name without a separator is malformed but must not panic.
		{BranchEntry{Name: "origin", Remote: true}, "origin"},
	} {
		if got := tc.entry.LocalName(); got != tc.want {
			t.Fatalf("LocalName(%+v) = %q, want %q", tc.entry, got, tc.want)
		}
	}
}

// TestCheckoutRemoteBranchTracking checks the refs a remote checkout creates:
// a local branch named after the remote's branch, and an upstream when the
// remote is configured.
func TestCheckoutRemoteBranchTracking(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "a\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}
	head, err := r.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	oid := strings.TrimSpace(string(head))
	base, err := r.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	baseName := strings.TrimSpace(string(base))
	for _, ref := range []string{"refs/remotes/origin/feature/x", "refs/remotes/origin/feature/y"} {
		if _, err := r.Run(ctx, "update-ref", ref, oid); err != nil {
			t.Fatal(err)
		}
	}

	// No remote is configured yet. The branch must still be created; only the
	// upstream link is skipped.
	if err := r.CheckoutRemoteBranch(ctx, "origin/feature/x"); err != nil {
		t.Fatal(err)
	}
	name, err := r.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(name)); got != "feature/x" {
		t.Fatalf("HEAD = %q, want feature/x", got)
	}
	if _, err := r.Run(ctx, "rev-parse", "--abbrev-ref", "feature/x@{upstream}"); err == nil {
		t.Fatal("a remote-tracking ref without a remote must not gain an upstream")
	}

	// With origin configured, the branch tracks the ref it came from.
	if _, err := r.Run(ctx, "remote", "add", "origin", "https://example.invalid/repo.git"); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckoutBranch(ctx, baseName); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckoutRemoteBranch(ctx, "origin/feature/y"); err != nil {
		t.Fatal(err)
	}
	up, err := r.Run(ctx, "rev-parse", "--abbrev-ref", "feature/y@{upstream}")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(up)); got != "origin/feature/y" {
		t.Fatalf("upstream = %q, want origin/feature/y", got)
	}

	// The name is not a local branch yet, so a second checkout would collide;
	// the caller checks its list first and never reaches this path.
	if err := r.CheckoutRemoteBranch(ctx, "origin/feature/y"); err == nil {
		t.Fatal("checking out the same remote branch twice should fail")
	}
	// A name without a remote prefix has nothing to track.
	if err := r.CheckoutRemoteBranch(ctx, "main"); err == nil {
		t.Fatal("a remote checkout without a remote prefix should fail")
	}
}

func TestBranchesCheckoutCreateDelete(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "a\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}

	entries, err := r.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Current || entries[0].Subject != "first" {
		t.Fatalf("branches = %+v", entries)
	}
	base := entries[0].Name
	if entries[0].Time == 0 {
		t.Fatalf("tip time = %+v", entries[0])
	}

	if err := r.CreateBranch(ctx, "feature/login"); err != nil {
		t.Fatal(err)
	}
	entries, err = r.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "feature/login" || !entries[0].Current {
		t.Fatalf("after create = %+v", entries)
	}

	if err := r.CheckoutBranch(ctx, base); err != nil {
		t.Fatal(err)
	}
	entries, err = r.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Name != base || !entries[0].Current {
		t.Fatalf("after checkout = %+v", entries)
	}

	// feature/login points at the same commit and is merged, so -d accepts it.
	if err := r.DeleteBranch(ctx, "feature/login"); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteBranch(ctx, base); err == nil {
		t.Fatal("deleting the current branch should fail")
	}

	if err := r.CreateBranch(ctx, "topic"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "b.txt", "b\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "topic work"}); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckoutBranch(ctx, base); err != nil {
		t.Fatal(err)
	}
	// -d refuses an unmerged branch; the panel surfaces that instead of
	// forcing the delete.
	if err := r.DeleteBranch(ctx, "topic"); err == nil {
		t.Fatal("deleting an unmerged branch should fail")
	}
}

// TestBranchesListsRemotes checks the real for-each-ref invocation, not only
// the parser: remote refs are present, origin/HEAD is not, and the remote's
// name is recorded.
func TestBranchesListsRemotes(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "a\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}
	oid := gitLog(t, r, "%H")
	if _, err := r.Run(ctx, "remote", "add", "origin", "https://example.invalid/repo.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "update-ref", "refs/remotes/origin/main", oid); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}

	entries, err := r.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var local, remote bool
	for _, e := range entries {
		switch e.Name {
		case "origin/HEAD":
			t.Fatalf("origin/HEAD was listed: %+v", entries)
		case "origin/main":
			if !e.Remote || e.RemoteName != "origin" || e.Current {
				t.Fatalf("origin/main = %+v", e)
			}
			remote = true
		default:
			local = local || !e.Remote
		}
	}
	if !local || !remote {
		t.Fatalf("local=%v remote=%v entries=%+v", local, remote, entries)
	}
}

// TestCheckoutRemoteBranchAvoidsAmbiguousShortName covers a local branch named
// like the remote ref: the short name is ambiguous, the qualified ref is not.
func TestCheckoutRemoteBranchAvoidsAmbiguousShortName(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "a\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}
	oid := gitLog(t, r, "%H")
	// Keep the checked-out branch away from main, which the test creates.
	if _, err := r.Run(ctx, "branch", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	base, err := r.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "remote", "add", "origin", "https://example.invalid/repo.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "update-ref", "refs/remotes/origin/main", oid); err != nil {
		t.Fatal(err)
	}
	// A local branch whose name is exactly the remote's short ref.
	if _, err := r.Run(ctx, "branch", "origin/main", strings.TrimSpace(string(base))); err != nil {
		t.Fatal(err)
	}

	if err := r.CheckoutRemoteBranch(ctx, "origin/main"); err != nil {
		t.Fatal(err)
	}
	if got := gitLog(t, r, "%D"); !strings.Contains(got, "HEAD -> main") {
		t.Fatalf("HEAD is not the new main branch: %q", got)
	}
}

// TestCheckoutRemoteBranchSlashRemote covers a remote whose own name contains
// "/": the local branch keeps only the branch part.
func TestCheckoutRemoteBranchSlashRemote(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "a\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}
	oid := gitLog(t, r, "%H")
	if _, err := r.Run(ctx, "remote", "add", "foo/bar", "https://example.invalid/x.git"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "update-ref", "refs/remotes/foo/bar/topic", oid); err != nil {
		t.Fatal(err)
	}

	if err := r.CheckoutRemoteBranch(ctx, "foo/bar/topic"); err != nil {
		t.Fatal(err)
	}
	if got := gitLog(t, r, "%D"); !strings.Contains(got, "HEAD -> topic") {
		t.Fatalf("HEAD is not topic: %q", got)
	}
	up, err := r.Run(ctx, "rev-parse", "--abbrev-ref", "topic@{upstream}")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(up)); got != "foo/bar/topic" {
		t.Fatalf("upstream = %q, want foo/bar/topic", got)
	}
}

func TestBranchesUnbornAndInvalidNames(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	entries, err := r.Branches(ctx)
	if err != nil {
		t.Fatalf("unborn branch should not fail: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("branches = %+v, want empty", entries)
	}

	for _, name := range []string{"", "  ", "-bad", "a..b", "has space"} {
		if err := r.CreateBranch(ctx, name); err == nil {
			t.Fatalf("branch %q should be rejected", name)
		}
	}
}
