package gitx

import (
	"context"
	"strings"
	"testing"
)

func gitLog(t *testing.T, r *Runner, format string) string {
	t.Helper()
	out, err := r.Run(context.Background(), "log", "-1", "--format="+format)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitAndAmend(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "one\n")
	if _, err := r.Run(ctx, "add", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}

	oid, err := r.Commit(ctx, CommitRequest{Message: "  first  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(oid) != 40 {
		t.Fatalf("oid = %q, want a full hash", oid)
	}
	if got := gitLog(t, r, "%s"); got != "first" {
		t.Fatalf("message = %q, want trimmed %q", got, "first")
	}

	writeFile(t, r, "a.txt", "two\n")
	if _, err := r.Run(ctx, "add", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "second", Amend: true}); err != nil {
		t.Fatal(err)
	}
	if got := gitLog(t, r, "%s"); got != "second" {
		t.Fatalf("amended message = %q", got)
	}
	if out, err := r.Run(ctx, "rev-list", "--count", "HEAD"); err != nil {
		t.Fatal(err)
	} else if strings.TrimSpace(string(out)) != "1" {
		t.Fatalf("commit count = %s, want 1", out)
	}
}

func TestCommitRequiresMessage(t *testing.T) {
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "one\n")
	if _, err := r.Run(context.Background(), "add", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{"", "   ", "\n\t"} {
		if _, err := r.Commit(context.Background(), CommitRequest{Message: message}); err == nil {
			t.Fatalf("message %q should be rejected", message)
		}
	}
}

func TestCommitWithNothingStaged(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "one\n")
	if _, err := r.Run(ctx, "add", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, CommitRequest{Message: "first"}); err != nil {
		t.Fatal(err)
	}

	// An unstaged edit must not be committed.
	writeFile(t, r, "a.txt", "two\n")
	if _, err := r.Commit(ctx, CommitRequest{Message: "second"}); err == nil {
		t.Fatal("a commit with no staged changes must fail")
	}
	if got := gitLog(t, r, "%s"); got != "first" {
		t.Fatalf("HEAD moved to %q", got)
	}
}
