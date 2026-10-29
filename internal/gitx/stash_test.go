package gitx

import (
	"context"
	"strings"
	"testing"
)

func TestStashMessage(t *testing.T) {
	for in, want := range map[string]string{
		"On main: my note":                "my note",
		"On feature/x: a: b":              "a: b",
		"WIP on main: abc1234 Add readme": "main: abc1234 Add readme",
		"On main":                         "On main",
		"something else entirely":         "something else entirely",
	} {
		if got := stashMessage(in); got != want {
			t.Fatalf("stashMessage(%q) = %q, want %q", in, got, want)
		}
	}
}

// stashRepo has one commit and a dirty tracked file plus an untracked one.
func stashRepo(t *testing.T) *Runner {
	t.Helper()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "base\n")
	commitOID(t, r, "base")
	writeFile(t, r, "a.txt", "edited\n")
	writeFile(t, r, "new.txt", "new\n")
	return r
}

func TestStashPushListApplyPopDrop(t *testing.T) {
	ctx := context.Background()
	r := stashRepo(t)

	if err := r.StashPush(ctx, "my note", true); err != nil {
		t.Fatal(err)
	}
	st, _ := r.Status(ctx)
	if len(st.Unstaged)+len(st.Untracked) != 0 {
		t.Fatalf("work tree not clean after stash: %+v", st)
	}
	list, err := r.Stashes(ctx)
	if err != nil || len(list) != 1 || list[0].Message != "my note" || list[0].Time == 0 {
		t.Fatalf("list = %+v, err %v", list, err)
	}
	oid := list[0].OID

	// Apply keeps the stash.
	if err := r.StashApply(ctx, oid); err != nil {
		t.Fatal(err)
	}
	if readFile(t, r, "a.txt") != "edited\n" || readFile(t, r, "new.txt") != "new\n" {
		t.Fatal("apply did not bring the changes back")
	}
	if list, _ = r.Stashes(ctx); len(list) != 1 {
		t.Fatalf("apply dropped the stash: %+v", list)
	}

	// Pop on a clean tree: applies and drops.
	if _, err := r.Run(ctx, "checkout", "--", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(ctx, "clean", "-fdq"); err != nil {
		t.Fatal(err)
	}
	if err := r.StashPop(ctx, oid); err != nil {
		t.Fatal(err)
	}
	if list, _ = r.Stashes(ctx); len(list) != 0 {
		t.Fatalf("pop kept the stash: %+v", list)
	}
	if readFile(t, r, "a.txt") != "edited\n" {
		t.Fatal("pop did not bring the changes back")
	}

	// Drop a stash that is gone.
	if err := r.StashDrop(ctx, oid); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("drop of a gone stash err = %v", err)
	}
}

// TestStashDropFindsByID drops the right stash after the list shifted.
func TestStashDropFindsByID(t *testing.T) {
	ctx := context.Background()
	r := stashRepo(t)
	if err := r.StashPush(ctx, "first", false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "a.txt", "second edit\n")
	if err := r.StashPush(ctx, "second", false); err != nil {
		t.Fatal(err)
	}
	list, _ := r.Stashes(ctx)
	if len(list) != 2 || list[0].Message != "second" {
		t.Fatalf("list = %+v", list)
	}
	// Drop by the id of "first", which is stash@{1} now.
	if err := r.StashDrop(ctx, list[1].OID); err != nil {
		t.Fatal(err)
	}
	left, _ := r.Stashes(ctx)
	if len(left) != 1 || left[0].Message != "second" {
		t.Fatalf("left = %+v", left)
	}
}

func TestStashNothingToSave(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	writeFile(t, r, "a.txt", "base\n")
	commitOID(t, r, "base")
	if err := r.StashPush(ctx, "", true); err == nil || !strings.Contains(err.Error(), "No local changes") {
		t.Fatalf("empty stash err = %v", err)
	}
	// Untracked files are skipped without the flag: nothing to save either.
	writeFile(t, r, "u.txt", "u\n")
	if err := r.StashPush(ctx, "", false); err == nil {
		t.Fatal("stash without -u saved an untracked file")
	}
	if err := r.StashPush(ctx, "", true); err != nil {
		t.Fatal(err)
	}
}

// TestStashApplyConflictKeepsStash: a conflicting apply leaves the stash.
func TestStashApplyConflictKeepsStash(t *testing.T) {
	ctx := context.Background()
	r := stashRepo(t)
	if err := r.StashPush(ctx, "", false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "a.txt", "other\n")
	commitOID(t, r, "other")
	list, _ := r.Stashes(ctx)
	if err := r.StashPop(ctx, list[0].OID); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicting pop err = %v", err)
	}
	if left, _ := r.Stashes(ctx); len(left) != 1 {
		t.Fatalf("the conflicting pop dropped the stash: %+v", left)
	}
}

func TestStashRefusesBadID(t *testing.T) {
	r := New(t.TempDir())
	ctx := context.Background()
	for name, err := range map[string]error{
		"apply": r.StashApply(ctx, "-x"),
		"pop":   r.StashPop(ctx, "-x"),
		"drop":  r.StashDrop(ctx, "-x"),
	} {
		if err == nil || !strings.Contains(err.Error(), "invalid commit id") {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}
