package gitx

import (
	"context"
	"strings"
	"testing"
)

func TestParseBranches(t *testing.T) {
	main := strings.Join([]string{"main", "*", "1700000000", "Base work"}, "\x1f")
	// A subject is arbitrary text: control bytes must not split the record.
	topic := strings.Join([]string{"topic", " ", "1690000000", "Topic\x1ework\x1fstill one field"}, "\x1f")
	entries := parseBranches([]byte("\x00" + main + "\n\x00" + topic + "\n"))
	if len(entries) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if e := entries[0]; !e.Current || e.Name != "main" || e.Time != 1700000000 ||
		e.Subject != "Base work" {
		t.Fatalf("main = %+v", e)
	}
	if e := entries[1]; e.Current || e.Name != "topic" || e.Subject != "Topic\x1ework\x1fstill one field" {
		t.Fatalf("topic = %+v", e)
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
