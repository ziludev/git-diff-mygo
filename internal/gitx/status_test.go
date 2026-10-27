package gitx

import (
	"strings"
	"testing"
)

func TestParseStatus(t *testing.T) {
	tokens := []string{
		"# branch.oid abc123",
		"# branch.head main",
		"# branch.upstream origin/main",
		"# branch.ab +1 -2",
		"1 M. N... 100644 100644 100644 aaa bbb a.go",
		"1 .M N... 100644 100644 100644 aaa bbb b.go",
		"1 MM N... 100644 100644 100644 aaa bbb c.go",
		"2 R. N... 100644 100644 100644 aaa bbb R100 new.go",
		"old.go",
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.go",
		"? untracked.txt",
	}
	st := ParseStatus("/repo", []byte(strings.Join(tokens, "\x00")))

	if st.Root != "/repo" {
		t.Fatalf("root = %q", st.Root)
	}
	if st.Branch.OID != "abc123" || st.Branch.Head != "main" || st.Branch.Upstream != "origin/main" {
		t.Fatalf("branch = %+v", st.Branch)
	}
	if st.Branch.Ahead != 1 || st.Branch.Behind != 2 {
		t.Fatalf("ahead/behind = %d/%d", st.Branch.Ahead, st.Branch.Behind)
	}
	if len(st.Staged) != 3 {
		t.Fatalf("staged = %+v", st.Staged)
	}
	if st.Staged[0].Path != "a.go" || st.Staged[1].Path != "c.go" || st.Staged[2].Path != "new.go" {
		t.Fatalf("staged paths = %+v", st.Staged)
	}
	if orig := st.Staged[2].OrigPath; orig != "old.go" {
		t.Fatalf("rename orig = %q", orig)
	}
	if len(st.Unstaged) != 2 || st.Unstaged[0].Path != "b.go" || st.Unstaged[1].Path != "c.go" {
		t.Fatalf("unstaged = %+v", st.Unstaged)
	}
	if len(st.Untracked) != 1 || st.Untracked[0].Path != "untracked.txt" {
		t.Fatalf("untracked = %+v", st.Untracked)
	}
	if len(st.Conflicted) != 1 || st.Conflicted[0].Path != "conflict.go" {
		t.Fatalf("conflicted = %+v", st.Conflicted)
	}
}

func TestParseStatusDetachedAndInitial(t *testing.T) {
	tokens := []string{
		"# branch.oid (initial)",
		"# branch.head (detached)",
		"? a.txt",
	}
	st := ParseStatus("/repo", []byte(strings.Join(tokens, "\x00")))
	if st.Branch.OID != "" || st.Branch.Head != "" {
		t.Fatalf("branch = %+v", st.Branch)
	}
	if len(st.Untracked) != 1 {
		t.Fatalf("untracked = %+v", st.Untracked)
	}
}
