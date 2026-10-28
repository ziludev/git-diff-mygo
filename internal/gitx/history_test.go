package gitx

import (
	"context"
	"strings"
	"testing"
)

func TestParseLog(t *testing.T) {
	records := strings.Join([]string{
		strings.Join([]string{"aaa", "a1", "Ana", "1700000000", "bbb ccc", "First subject"}, "\x1f"),
		strings.Join([]string{"bbb", "b2", "Bob", "1690000000", "", "Root commit"}, "\x1f"),
		// A subject is arbitrary text and may contain a control byte.
		strings.Join([]string{"ccc", "c3", "Cy", "1680000000", "", "Subject\x1e\x1fstill one record"}, "\x1f"),
	}, "\x00")
	commits := parseLog([]byte(records + "\x00\n"))
	if len(commits) != 3 {
		t.Fatalf("commits = %+v", commits)
	}
	if commits[0].OID != "aaa" || commits[0].Short != "a1" || commits[0].Author != "Ana" ||
		commits[0].Time != 1700000000 || commits[0].Subject != "First subject" {
		t.Fatalf("first = %+v", commits[0])
	}
	if got := commits[0].Parents; len(got) != 2 || got[0] != "bbb" || got[1] != "ccc" {
		t.Fatalf("parents = %v", got)
	}
	if commits[1].OID != "bbb" || commits[1].Subject != "Root commit" {
		t.Fatalf("second = %+v", commits[1])
	}
	if got := commits[1].Parents; len(got) != 0 {
		t.Fatalf("root parents = %v, want empty", got)
	}
	if commits[2].Subject != "Subject\x1e\x1fstill one record" {
		t.Fatalf("third = %+v", commits[2])
	}
}

func TestParseNameStatus(t *testing.T) {
	out := strings.Join([]string{"M", "a.txt", "A", "b.txt", "R100", "old.txt", "new.txt", "D", "gone.txt"}, "\x00") + "\x00"
	files := parseNameStatus([]byte(out))
	if len(files) != 4 {
		t.Fatalf("files = %+v", files)
	}
	if files[0].Path != "a.txt" || files[0].Status != "M" {
		t.Fatalf("modified = %+v", files[0])
	}
	if files[2].Status != "R" || files[2].Path != "new.txt" || files[2].OrigPath != "old.txt" {
		t.Fatalf("rename = %+v", files[2])
	}
	if files[3].Status != "D" || files[3].Path != "gone.txt" {
		t.Fatalf("delete = %+v", files[3])
	}
}

func TestParseNumstat(t *testing.T) {
	out := strings.Join([]string{
		"3\t1\ta.txt",
		"-\t-\tlogo.png",
		"0\t0\t", "old.txt", "new.txt",
	}, "\x00") + "\x00"
	counts := parseNumstat([]byte(out))
	if c := counts["a.txt"]; c.Additions != 3 || c.Deletions != 1 || c.Binary {
		t.Fatalf("a.txt = %+v", c)
	}
	if c := counts["logo.png"]; !c.Binary {
		t.Fatalf("logo.png = %+v, want binary", c)
	}
	if _, ok := counts["new.txt"]; !ok {
		t.Fatalf("rename counts keyed by new path: %+v", counts)
	}
}

func TestLogAndCommitFiles(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)

	writeFile(t, r, "a.txt", "one\ntwo\n")
	if _, err := r.Run(ctx, "add", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Run(ctx, "mv", "a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "b.txt", "one\nTWO\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	second, err := r.Commit(ctx, CommitRequest{Message: "rename and edit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "tag", "v1"); err != nil {
		t.Fatal(err)
	}

	commits, err := r.Log(ctx, LogRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].OID != second || commits[0].Subject != "rename and edit" {
		t.Fatalf("commits = %+v", commits)
	}
	// The tip carries the tag and the checked-out branch.
	var sawTag, sawBranch bool
	for _, ref := range commits[0].Refs {
		if ref.Kind == "tag" && ref.Name == "v1" {
			sawTag = true
		}
		if ref.Kind == "branch" {
			sawBranch = true
		}
	}
	if !sawTag || !sawBranch {
		t.Fatalf("refs = %+v, want the tag and a branch", commits[0].Refs)
	}

	files, err := r.CommitFiles(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %+v", files)
	}
	if f := files[0]; f.Path != "b.txt" || f.OrigPath != "a.txt" || f.Status != "R" {
		t.Fatalf("rename row = %+v", f)
	}

	patch, err := r.CommitDiff(ctx, CommitDiffRequest{OID: second, Path: "b.txt", OrigPath: "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch.Patch, "rename from a.txt") || !strings.Contains(patch.Patch, "+TWO") {
		t.Fatalf("patch = %s", patch.Patch)
	}
}

func TestLogIncludesUnmergedBranches(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "a\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "base"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "checkout", "-q", "-b", "topic"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "b.txt", "b\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "topic only"}); err != nil {
		t.Fatal(err)
	}

	commits, err := r.Log(ctx, LogRequest{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, commit := range commits {
		if commit.Subject == "topic only" {
			found = true
		}
	}
	if !found {
		t.Fatalf("commits = %+v, want the unmerged branch commit", commits)
	}
}

func TestLogUnbornBranch(t *testing.T) {
	r := newTestRepo(t)
	commits, err := r.Log(context.Background(), LogRequest{})
	if err != nil {
		t.Fatalf("unborn branch should not fail: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("commits = %+v, want empty", commits)
	}
}

func TestCommitFilesMergeUsesFirstParent(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "base.txt", "base\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "base"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "checkout", "-q", "-b", "side"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "side.txt", "side\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "side"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "checkout", "-q", "master"); err != nil {
		// Recent git defaults may name the initial branch main.
		if _, err := r.Run(ctx, "checkout", "-q", "main"); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, r, "main.txt", "main\n")
	if _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "merge", "-q", "--no-ff", "side", "-m", "merge side"); err != nil {
		t.Fatal(err)
	}
	oid, err := r.Run(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	merge := strings.TrimSpace(string(oid))

	files, err := r.CommitFiles(ctx, merge)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	if len(paths) != 1 || paths[0] != "side.txt" {
		t.Fatalf("merge files = %v, want [side.txt]", paths)
	}
}
