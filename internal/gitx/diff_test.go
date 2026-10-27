package gitx

import (
	"strings"
	"testing"
)

func TestNewFilePatch(t *testing.T) {
	got := newFilePatch("dir/a.txt", "hello\nworld\n", "100644")
	want := "diff --git a/dir/a.txt b/dir/a.txt\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/dir/a.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+hello\n" +
		"+world\n"
	if got != want {
		t.Fatalf("patch =\n%s\nwant\n%s", got, want)
	}
}

func TestNewFilePatchNoTrailingNewline(t *testing.T) {
	got := newFilePatch("a.txt", "hello", "100644")
	if !strings.Contains(got, "+hello\n\\ No newline at end of file\n") {
		t.Fatalf("patch =\n%s", got)
	}
}

func TestSelectHunks(t *testing.T) {
	patch := "diff --git a/a.txt b/a.txt\n" +
		"index 111..222 100644\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		"-one\n" +
		"+ONE\n" +
		" two\n" +
		"@@ -10,2 +10,2 @@\n" +
		"-ten\n" +
		"+TEN\n" +
		" eleven\n"

	got, err := selectHunks(patch, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "-one") || !strings.Contains(got, "-ten") {
		t.Fatalf("selected wrong hunks:\n%s", got)
	}
	if !strings.HasPrefix(got, "diff --git a/a.txt b/a.txt\n") {
		t.Fatalf("missing header:\n%s", got)
	}
}

func TestSelectHunksOutOfRange(t *testing.T) {
	_, err := selectHunks("diff --git a/a b/a\n@@ -1 +1 @@\n-a\n+b\n", []int{2})
	if err == nil {
		t.Fatal("expected an error")
	}
}
