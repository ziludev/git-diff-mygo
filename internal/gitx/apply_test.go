package gitx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo initializes a repository with one commit-friendly config.
func newTestRepo(t *testing.T) *Runner {
	t.Helper()
	r := New(t.TempDir())
	if _, err := r.Run(context.Background(), "init", "-q"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	configTestUser(t, r)
	return r
}

// configTestUser gives a test repository the identity and signing settings a
// commit needs, with no dependency on the machine's git config.
func configTestUser(t *testing.T, r *Runner) {
	t.Helper()
	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := r.Run(context.Background(), args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
}

func writeFile(t *testing.T, r *Runner, name, content string) {
	t.Helper()
	full := filepath.Join(r.Dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, r *Runner, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.Dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func commitAll(t *testing.T, r *Runner, message string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", message},
	} {
		if _, err := r.Run(context.Background(), args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
}

func indexFile(t *testing.T, r *Runner, name string) string {
	t.Helper()
	out, err := r.Run(context.Background(), "show", ":"+name)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// twoHunkRepo commits a 30-line file, then edits line 1 and line 25 so the
// diff has two separate hunks.
func twoHunkRepo(t *testing.T) *Runner {
	t.Helper()
	r := newTestRepo(t)
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	writeFile(t, r, "a.txt", b.String())
	commitAll(t, r, "init")

	changed := strings.Replace(b.String(), "line 1\n", "LINE 1\n", 1)
	changed = strings.Replace(changed, "line 25\n", "LINE 25\n", 1)
	writeFile(t, r, "a.txt", changed)
	return r
}

func countHunks(t *testing.T, r *Runner, staged bool) int {
	t.Helper()
	res, err := r.Diff(context.Background(), DiffRequest{Path: "a.txt", Staged: staged})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(res.Patch, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			n++
		}
	}
	return n
}

func TestApplyHunksStagesOnlySelectedHunk(t *testing.T) {
	r := twoHunkRepo(t)
	if got := countHunks(t, r, false); got != 2 {
		t.Fatalf("worktree hunks = %d, want 2", got)
	}

	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "a.txt", Op: ApplyStage, Hunks: []int{1},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := countHunks(t, r, true); got != 1 {
		t.Fatalf("staged hunks = %d, want 1", got)
	}
	index := indexFile(t, r, "a.txt")
	if !strings.Contains(index, "line 1\n") || !strings.Contains(index, "LINE 25\n") {
		t.Fatalf("index kept the wrong hunk:\n%s", index)
	}
	worktree := readFile(t, r, "a.txt")
	if !strings.Contains(worktree, "LINE 1\n") || !strings.Contains(worktree, "LINE 25\n") {
		t.Fatalf("worktree changed while staging:\n%s", worktree)
	}
}

func TestApplyHunksUnstagesOnlySelectedHunk(t *testing.T) {
	r := twoHunkRepo(t)
	if _, err := r.Run(context.Background(), "add", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}

	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "a.txt", Op: ApplyUnstage, Hunks: []int{0},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := countHunks(t, r, true); got != 1 {
		t.Fatalf("staged hunks = %d, want 1", got)
	}
	index := indexFile(t, r, "a.txt")
	if strings.Contains(index, "LINE 1\n") || !strings.Contains(index, "LINE 25\n") {
		t.Fatalf("index kept the wrong hunk:\n%s", index)
	}
	worktree := readFile(t, r, "a.txt")
	if !strings.Contains(worktree, "LINE 1\n") || !strings.Contains(worktree, "LINE 25\n") {
		t.Fatalf("unstage touched the worktree:\n%s", worktree)
	}
}

func TestApplyHunksDiscardsOnlySelectedHunk(t *testing.T) {
	r := twoHunkRepo(t)

	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "a.txt", Op: ApplyDiscard, Hunks: []int{1},
	})
	if err != nil {
		t.Fatal(err)
	}

	worktree := readFile(t, r, "a.txt")
	if !strings.Contains(worktree, "LINE 1\n") || strings.Contains(worktree, "LINE 25\n") {
		t.Fatalf("discard reverted the wrong hunk:\n%s", worktree)
	}
	if got := countHunks(t, r, false); got != 1 {
		t.Fatalf("worktree hunks = %d, want 1", got)
	}
}

func TestApplyHunksStagesUntrackedFile(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "docs/new.md", "hello\nworld\n")

	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "docs/new.md", Op: ApplyStage, Untracked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := indexFile(t, r, "docs/new.md"); got != "hello\nworld\n" {
		t.Fatalf("index content = %q", got)
	}
}

func TestUnstageRenameResetsBothSides(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "old.txt", "one\ntwo\n")
	commitAll(t, r, "init")
	if _, err := r.Run(context.Background(), "mv", "old.txt", "new.txt"); err != nil {
		t.Fatal(err)
	}

	change := PathChange{Path: "new.txt", OrigPath: "old.txt"}
	err := r.Unstage(context.Background(), Paths(change.Path, change.OrigPath)...)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := r.Run(context.Background(), "show", ":old.txt"); err != nil {
		t.Fatalf("old path should be back in the index: %v", err)
	}
	if _, err := r.Run(context.Background(), "show", ":new.txt"); err == nil {
		t.Fatal("new path should be out of the index")
	}
}

func TestDiscardRestoresStagedRenameWorktreeEdit(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "old.txt", "one\ntwo\n")
	commitAll(t, r, "init")
	if _, err := r.Run(context.Background(), "mv", "old.txt", "new.txt"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "new.txt", "one\ntwo\nthree\n")

	if err := r.Discard(context.Background(), "new.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, r, "new.txt"); got != "one\ntwo\n" {
		t.Fatalf("worktree content = %q, want the index version", got)
	}
}

func TestApplyHunksRejectsRenamePartial(t *testing.T) {
	r := newTestRepo(t)
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	writeFile(t, r, "old.txt", b.String())
	commitAll(t, r, "init")
	if _, err := r.Run(context.Background(), "mv", "old.txt", "new.txt"); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(b.String(), "line 1\n", "LINE 1\n", 1)
	changed = strings.Replace(changed, "line 25\n", "LINE 25\n", 1)
	writeFile(t, r, "new.txt", changed)
	if _, err := r.Run(context.Background(), "add", "--", "new.txt"); err != nil {
		t.Fatal(err)
	}

	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "new.txt", OrigPath: "old.txt", Op: ApplyUnstage, Hunks: []int{0},
	})
	if err == nil {
		t.Fatal("a partial apply of a rename must fail")
	}
}

func TestApplyHunksStagesEmptyUntrackedFile(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "empty.txt", "")

	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "empty.txt", Op: ApplyStage, Untracked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := indexFile(t, r, "empty.txt"); got != "" {
		t.Fatalf("index content = %q, want empty", got)
	}
}

func TestApplyHunksRejectsUntrackedNonStage(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "new.md", "hello\n")

	for _, op := range []ApplyOp{ApplyUnstage, ApplyDiscard} {
		err := r.ApplyHunks(context.Background(), ApplyRequest{
			Path: "new.md", Op: op, Untracked: true,
		})
		if err == nil {
			t.Fatalf("%s of an untracked file must fail", op)
		}
	}
}

func TestApplyHunksReportsOutOfRange(t *testing.T) {
	r := twoHunkRepo(t)
	err := r.ApplyHunks(context.Background(), ApplyRequest{
		Path: "a.txt", Op: ApplyStage, Hunks: []int{9},
	})
	if err == nil {
		t.Fatal("expected an out-of-range error")
	}
}
