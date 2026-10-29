package gitx

import (
	"context"
	"strings"
	"testing"
)

// commitOID stages everything and commits, returning the new commit's id.
func commitOID(t *testing.T, r *Runner, msg string) string {
	t.Helper()
	ctx := context.Background()
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	oid, err := r.Commit(ctx, CommitRequest{Message: msg})
	if err != nil {
		t.Fatal(err)
	}
	return oid
}

func revParse(t *testing.T, r *Runner, rev string) string {
	t.Helper()
	out, err := r.Run(context.Background(), "rev-parse", rev)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

// opsRepo builds main: c1 (a=1) -> c2 (a=2), and a side branch from c1 with
// c3 (adds b) and c4 (a=x, which conflicts with main's a=2). HEAD ends on main.
func opsRepo(t *testing.T) (r *Runner, c1, c2, c3, c4 string) {
	t.Helper()
	ctx := context.Background()
	r = newTestRepo(t)
	writeFile(t, r, "a.txt", "1\n")
	c1 = commitOID(t, r, "c1")
	base := strings.TrimSpace(func() string {
		out, _ := r.Run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
		return string(out)
	}())
	writeFile(t, r, "a.txt", "2\n")
	c2 = commitOID(t, r, "c2")
	if _, err := r.Run(ctx, "checkout", "-q", "-b", "side", c1); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "b.txt", "b\n")
	c3 = commitOID(t, r, "c3")
	writeFile(t, r, "a.txt", "x\n")
	c4 = commitOID(t, r, "c4")
	if _, err := r.Run(ctx, "checkout", "-q", base); err != nil {
		t.Fatal(err)
	}
	return
}

func TestCheckOID(t *testing.T) {
	for _, bad := range []string{"", "-x", "--detach", "zzzz", "abc", "ab cd", "abcd\n"} {
		if err := checkOID(bad); err == nil {
			t.Fatalf("checkOID(%q) accepted", bad)
		}
	}
	if err := checkOID("0123abcdEF"); err != nil {
		t.Fatal(err)
	}
	// Every operation refuses before running git.
	r := New(t.TempDir())
	ctx := context.Background()
	for name, err := range map[string]error{
		"checkout": r.CheckoutRevision(ctx, "-x"),
		"branch":   r.CreateBranchAt(ctx, "n", "-x"),
		"tag":      r.Tag(ctx, "n", "-x"),
		"pick":     r.CherryPick(ctx, "-x"),
		"revert":   r.Revert(ctx, "-x"),
		"reset":    r.Reset(ctx, "-x", ResetHard),
	} {
		if err == nil || !strings.Contains(err.Error(), "invalid commit id") {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if err := r.Reset(ctx, "abcd", "weird"); err == nil {
		t.Fatal("unknown reset mode accepted")
	}
}

func TestCheckoutRevisionDetaches(t *testing.T) {
	r, c1, _, _, _ := opsRepo(t)
	if err := r.CheckoutRevision(context.Background(), c1); err != nil {
		t.Fatal(err)
	}
	st, err := r.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Branch.Detached || revParse(t, r, "HEAD") != c1 {
		t.Fatalf("branch = %+v", st.Branch)
	}
}

func TestCreateBranchAtAndTag(t *testing.T) {
	ctx := context.Background()
	r, c1, _, _, _ := opsRepo(t)
	if err := r.CreateBranchAt(ctx, "from-c1", c1); err != nil {
		t.Fatal(err)
	}
	if revParse(t, r, "HEAD") != c1 {
		t.Fatal("the new branch is not at c1")
	}
	if err := r.Tag(ctx, "v1", c1); err != nil {
		t.Fatal(err)
	}
	if revParse(t, r, "v1^{commit}") != c1 {
		t.Fatal("tag does not point at c1")
	}
	// An existing tag is git's error, not overwritten.
	if err := r.Tag(ctx, "v1", revParse(t, r, "main^{commit}")); err == nil {
		t.Fatal("a second tag with the same name succeeded")
	}
}

func TestCherryPick(t *testing.T) {
	ctx := context.Background()
	r, _, c2, c3, c4 := opsRepo(t)
	if err := r.CherryPick(ctx, c3); err != nil {
		t.Fatal(err)
	}
	if readFile(t, r, "b.txt") != "b\n" || revParse(t, r, "HEAD") == c2 {
		t.Fatal("c3 was not applied as a new commit")
	}
	head := revParse(t, r, "HEAD")

	// c4 conflicts with a=2: the pick is aborted and the repository is as it was.
	err := r.CherryPick(ctx, c4)
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("conflicting pick err = %v", err)
	}
	assertClean(t, r, head, "CHERRY_PICK_HEAD")
}

func TestCherryPickRefusedByDirtyTree(t *testing.T) {
	ctx := context.Background()
	r, _, c2, c3, _ := opsRepo(t)
	// An untracked b.txt would be overwritten by c3: git refuses before it
	// starts, so there is nothing to abort and the message is git's own.
	writeFile(t, r, "b.txt", "mine\n")
	err := r.CherryPick(ctx, c3)
	if err == nil || strings.Contains(err.Error(), "aborted") {
		t.Fatalf("refused pick err = %v", err)
	}
	if revParse(t, r, "HEAD") != c2 || readFile(t, r, "b.txt") != "mine\n" {
		t.Fatal("a refused pick changed the repository")
	}
}

func TestCherryPickOfMergeIsRefused(t *testing.T) {
	ctx := context.Background()
	r, _, _, _, _ := opsRepo(t)
	if _, err := r.Run(ctx, "merge", "--no-ff", "-m", "merge", "side~1"); err != nil {
		t.Fatal(err)
	}
	merge := revParse(t, r, "HEAD")
	if _, err := r.Run(ctx, "reset", "-q", "--hard", "HEAD~1"); err != nil {
		t.Fatal(err)
	}
	head := revParse(t, r, "HEAD")
	if err := r.CherryPick(ctx, merge); err == nil {
		t.Fatal("cherry-picking a merge succeeded")
	}
	assertClean(t, r, head, "CHERRY_PICK_HEAD")
}

func TestRevert(t *testing.T) {
	ctx := context.Background()
	r, _, c2, _, _ := opsRepo(t)
	if err := r.Revert(ctx, c2); err != nil {
		t.Fatal(err)
	}
	if readFile(t, r, "a.txt") != "1\n" {
		t.Fatalf("a.txt = %q after reverting c2", readFile(t, r, "a.txt"))
	}
}

func TestRevertConflictIsAborted(t *testing.T) {
	ctx := context.Background()
	r, c1, _, _, _ := opsRepo(t)
	head := revParse(t, r, "HEAD")
	// Reverting c1 deletes a.txt, which c2 changed: a conflict, aborted.
	err := r.Revert(ctx, c1)
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("conflicting revert err = %v", err)
	}
	assertClean(t, r, head, "REVERT_HEAD")
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	r, c1, c2, _, _ := opsRepo(t)

	if err := r.Reset(ctx, c1, ResetSoft); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status(ctx)
	if revParse(t, r, "HEAD") != c1 || len(st.Staged) != 1 || readFile(t, r, "a.txt") != "2\n" {
		t.Fatalf("soft: staged=%+v", st.Staged)
	}

	if err := r.Reset(ctx, c2, ResetHard); err != nil {
		t.Fatal(err)
	}
	if err := r.Reset(ctx, c1, ResetMixed); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if len(st.Staged) != 0 || len(st.Unstaged) != 1 || readFile(t, r, "a.txt") != "2\n" {
		t.Fatalf("mixed: staged=%+v unstaged=%+v", st.Staged, st.Unstaged)
	}

	if err := r.Reset(ctx, c1, ResetHard); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if len(st.Unstaged) != 0 || readFile(t, r, "a.txt") != "1\n" {
		t.Fatalf("hard: unstaged=%+v", st.Unstaged)
	}
}

// assertClean checks a failed operation left HEAD where it was, no operation
// in progress and a clean work tree.
func assertClean(t *testing.T, r *Runner, head, marker string) {
	t.Helper()
	ctx := context.Background()
	if got := revParse(t, r, "HEAD"); got != head {
		t.Fatalf("HEAD moved from %s to %s", head, got)
	}
	if _, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", marker); err == nil {
		t.Fatalf("%s is still set", marker)
	}
	st, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Conflicted)+len(st.Staged)+len(st.Unstaged)+len(st.Untracked) != 0 {
		t.Fatalf("work tree is not clean: %+v", st)
	}
}

// TestStagePathsLiteralAndBulk stages a list in one call: names that look
// like patterns are taken literally, and an untouched sibling stays out.
func TestStagePathsLiteralAndBulk(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	for _, name := range []string{"dir/*.go", "dir/a b.txt", "dir/other.go", "keep.txt"} {
		writeFile(t, r, name, "x\n")
	}
	if err := r.StagePaths(ctx, []string{"dir/*.go", "dir/a b.txt"}); err != nil {
		t.Fatal(err)
	}
	st, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var staged []string
	for _, ch := range st.Staged {
		staged = append(staged, ch.Path)
	}
	// "dir/other.go" matches the glob "dir/*.go" but is not named in the list.
	if strings.Join(staged, ",") != "dir/*.go,dir/a b.txt" {
		t.Fatalf("staged = %v", staged)
	}
	if len(st.Untracked) != 2 {
		t.Fatalf("untracked = %+v", st.Untracked)
	}
	if err := r.StagePaths(ctx, nil); err == nil {
		t.Fatal("an empty list was accepted")
	}
}

// TestUnstagePathsBothSides unstages a staged rename through both of its
// paths, and works on an unborn branch too.
func TestUnstagePathsBothSides(t *testing.T) {
	ctx := context.Background()

	// Unborn: nothing to reset to, so the index entries are removed.
	r := newTestRepo(t)
	writeFile(t, r, "d/a.txt", "a\n")
	writeFile(t, r, "d/b.txt", "b\n")
	if err := r.StagePaths(ctx, []string{"d/a.txt", "d/b.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := r.UnstagePaths(ctx, []string{"d/a.txt", "d/b.txt"}); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status(ctx)
	if len(st.Staged) != 0 || len(st.Untracked) != 2 {
		t.Fatalf("unborn unstage: staged=%+v untracked=%+v", st.Staged, st.Untracked)
	}

	// With history: a staged rename comes back through both paths.
	r = newTestRepo(t)
	writeFile(t, r, "d/old.txt", "same content that git can match as a rename\n")
	commitOID(t, r, "base")
	if _, err := r.Run(ctx, "mv", "d/old.txt", "d/new.txt"); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if len(st.Staged) != 1 || st.Staged[0].OrigPath != "d/old.txt" {
		t.Fatalf("setup: staged = %+v", st.Staged)
	}
	if err := r.UnstagePaths(ctx, Paths(st.Staged[0].Path, st.Staged[0].OrigPath)); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(ctx)
	if len(st.Staged) != 0 {
		t.Fatalf("rename still staged: %+v", st.Staged)
	}
}
