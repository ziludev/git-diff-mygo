package gitx

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestFileLinesWorktreeIndexAndCommit(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "one\ntwo\n")
	commitAll(t, r, "first")
	out, err := r.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	oid := strings.TrimSpace(string(out))

	// The worktree moved on; the index and the commit still have the original
	// text, which is exactly the distinction a staged diff needs.
	writeFile(t, r, "a.txt", "one\nchanged\n")

	worktree, err := r.FileLines(ctx, FileTextRequest{Path: "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one", "changed"}; !reflect.DeepEqual(worktree, want) {
		t.Fatalf("worktree = %q", worktree)
	}
	index, err := r.FileLines(ctx, FileTextRequest{Path: "a.txt", Rev: "index"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one", "two"}; !reflect.DeepEqual(index, want) {
		t.Fatalf("index = %q", index)
	}
	commit, err := r.FileLines(ctx, FileTextRequest{Path: "a.txt", Rev: oid})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one", "two"}; !reflect.DeepEqual(commit, want) {
		t.Fatalf("commit = %q", commit)
	}
}

func TestFileLinesRefusesMissingFileAndColonPath(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	if _, err := r.FileLines(ctx, FileTextRequest{Path: "missing.txt"}); err == nil {
		t.Fatal("missing file did not fail")
	}
	if _, err := r.FileLines(ctx, FileTextRequest{Path: "a:b.txt", Rev: "index"}); err == nil {
		t.Fatal("colon path did not fail")
	}
}
