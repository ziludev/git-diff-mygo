package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteLastRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.json")

	if got := readLastRepo(path); got != "" {
		t.Fatalf("missing file = %q, want empty", got)
	}

	if err := writeLastRepo(path, "/tmp/example"); err != nil {
		t.Fatal(err)
	}
	if got := readLastRepo(path); got != "/tmp/example" {
		t.Fatalf("read = %q", got)
	}

	// A later open replaces the saved root.
	if err := writeLastRepo(path, "/tmp/other"); err != nil {
		t.Fatal(err)
	}
	if got := readLastRepo(path); got != "/tmp/other" {
		t.Fatalf("overwrite = %q", got)
	}

	// A damaged state file means "nothing to restore", not an error.
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readLastRepo(path); got != "" {
		t.Fatalf("invalid JSON = %q, want empty", got)
	}

	// The atomic write must not leave its temporary file behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}
