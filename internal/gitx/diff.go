package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultContext  = 3
	maxNewFileBytes = 4 << 20 // 4 MiB: above this the UI offers a summary only.
)

// DiffRequest asks for one file's patch.
type DiffRequest struct {
	Path      string `json:"path"`
	OrigPath  string `json:"origPath,omitempty"`
	Staged    bool   `json:"staged"`
	Untracked bool   `json:"untracked"`
}

// DiffResult is a unified diff for one file.
type DiffResult struct {
	Path      string `json:"path"`
	Patch     string `json:"patch"`
	Binary    bool   `json:"binary"`
	Untracked bool   `json:"untracked"`
	TooLarge  bool   `json:"tooLarge"`
}

// Diff returns a unified diff of path against HEAD (staged) or the index
// (unstaged). Untracked files get a synthetic "new file" patch.
func (r *Runner) Diff(ctx context.Context, req DiffRequest) (DiffResult, error) {
	res := DiffResult{Path: req.Path, Untracked: req.Untracked}
	if req.Path == "" {
		return res, errors.New("empty path")
	}
	if req.Untracked {
		patch, binary, tooLarge, err := r.untrackedPatch(req.Path)
		res.Patch, res.Binary, res.TooLarge = patch, binary, tooLarge
		return res, err
	}

	args := []string{
		"diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"-U" + strconv.Itoa(defaultContext),
	}
	if req.Staged {
		args = append(args, "--cached")
	}
	args = append(args, "--")
	if req.OrigPath != "" && req.OrigPath != req.Path {
		args = append(args, req.OrigPath)
	}
	args = append(args, req.Path)

	out, err := r.Run(ctx, args...)
	if err != nil {
		return res, err
	}
	res.Patch = string(out)
	res.Binary = isBinaryPatch(res.Patch)
	return res, nil
}

// untrackedPatch builds the patch git would print for a new file, without
// running `git add` first. Large files are summarized instead.
func (r *Runner) untrackedPatch(path string) (patch string, binary, tooLarge bool, err error) {
	full, err := safeJoin(r.Dir, path)
	if err != nil {
		return "", false, false, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return "", false, false, err
	}
	if info.Size() > maxNewFileBytes {
		return "", false, true, nil
	}

	mode := "100644"
	content := []byte(nil)
	if info.Mode()&os.ModeSymlink != 0 {
		mode = "120000"
		target, err := os.Readlink(full)
		if err != nil {
			return "", false, false, err
		}
		content = []byte(target)
	} else {
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		content, err = os.ReadFile(full)
		if err != nil {
			return "", false, false, err
		}
	}
	if looksBinary(content) {
		return "", true, false, nil
	}
	return newFilePatch(path, string(content), mode), false, false, nil
}

// newFilePatch renders a git-style patch for a file added at path.
func newFilePatch(path, content, mode string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", path, path)
	fmt.Fprintf(&b, "new file mode %s\n", mode)
	b.WriteString("--- /dev/null\n")
	fmt.Fprintf(&b, "+++ b/%s\n", path)
	if content == "" {
		return b.String()
	}

	trailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}
	fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, line := range lines {
		b.WriteString("+")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if !trailingNewline {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.String()
}

// ApplyOp says what to do with a patch.
type ApplyOp string

const (
	// ApplyStage moves the selected worktree changes into the index.
	ApplyStage ApplyOp = "stage"
	// ApplyUnstage moves the selected index changes back to the worktree.
	ApplyUnstage ApplyOp = "unstage"
	// ApplyDiscard drops the selected worktree changes.
	ApplyDiscard ApplyOp = "discard"
)

// ApplyRequest applies some or all hunks of one file's diff.
type ApplyRequest struct {
	Path      string  `json:"path"`
	OrigPath  string  `json:"origPath,omitempty"`
	Op        ApplyOp `json:"op"`
	Hunks     []int   `json:"hunks"` // empty means all hunks
	Untracked bool    `json:"untracked,omitempty"`
}

// ApplyHunks applies the selected hunks of one file. The patch is rebuilt
// from git's own output, so the frontend never supplies patch text.
func (r *Runner) ApplyHunks(ctx context.Context, req ApplyRequest) error {
	if req.Path == "" {
		return errors.New("empty path")
	}
	// An untracked file has nothing to unstage or discard: it only exists in
	// the worktree, so adding it to the index is the only valid operation.
	if req.Untracked && req.Op != ApplyStage {
		return fmt.Errorf("untracked path %q can only be staged", req.Path)
	}
	sourceStaged := req.Op == ApplyUnstage
	res, err := r.Diff(ctx, DiffRequest{
		Path:      req.Path,
		OrigPath:  req.OrigPath,
		Staged:    sourceStaged,
		Untracked: req.Untracked,
	})
	if err != nil {
		return err
	}
	if res.Binary {
		return errors.New("binary changes cannot be applied by hunk")
	}
	// A rename header would make any partial patch rename the file as well,
	// so renames and copies only apply as a whole.
	if len(req.Hunks) > 0 &&
		(strings.Contains(res.Patch, "rename from ") || strings.Contains(res.Patch, "copy from ")) {
		return errors.New("renamed files must be applied as a whole")
	}
	// A zero-byte new file has no hunks. Adding it to the index is the whole
	// operation, so it needs no patch.
	if req.Untracked && req.Op == ApplyStage && !strings.Contains(res.Patch, "@@ ") {
		return r.Stage(ctx, req.Path)
	}
	patch, err := selectHunks(res.Patch, req.Hunks)
	if err != nil {
		return err
	}
	if strings.TrimSpace(patch) == "" {
		return errors.New("no changes to apply")
	}

	args := []string{"apply", "--recount", "--whitespace=nowarn"}
	switch req.Op {
	case ApplyStage:
		args = append(args, "--cached")
	case ApplyUnstage:
		args = append(args, "--cached", "-R")
	case ApplyDiscard:
		args = append(args, "-R")
	default:
		return fmt.Errorf("unknown apply operation %q", req.Op)
	}
	_, err = r.RunInput(ctx, []byte(patch), args...)
	return err
}

// Stage adds paths to the index.
func (r *Runner) Stage(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("no paths")
	}
	args := append([]string{"add", "--"}, paths...)
	_, err := r.Run(ctx, args...)
	return err
}

// Unstage removes paths from the index, keeping their worktree content.
func (r *Runner) Unstage(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("no paths")
	}
	if _, err := r.Run(ctx, "rev-parse", "--verify", "HEAD"); err != nil {
		// Unborn branch: there is no HEAD to reset to.
		args := append([]string{"rm", "--cached", "-r", "--"}, paths...)
		_, err := r.Run(ctx, args...)
		return err
	}
	args := append([]string{"reset", "-q", "HEAD", "--"}, paths...)
	_, err := r.Run(ctx, args...)
	return err
}

// Discard restores tracked paths from the index to the worktree. Untracked
// files are left alone: removing them needs an explicit confirmation.
func (r *Runner) Discard(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("no paths")
	}
	args := append([]string{"restore", "--worktree", "--"}, paths...)
	_, err := r.Run(ctx, args...)
	return err
}

// selectHunks keeps the patch header and the hunks whose indexes are in
// want. An empty want keeps every hunk. New and deleted files are applied
// whole: a partial patch cannot create or remove a file.
//
// The patch is scanned once for hunk header offsets instead of being split
// into lines: a click on one hunk must not materialize a whole 4 MiB patch.
func selectHunks(patch string, want []int) (string, error) {
	var starts []int
	headerEnd := -1
	for i := 0; i < len(patch); {
		end := strings.IndexByte(patch[i:], '\n')
		if end < 0 {
			end = len(patch) - i
		} else {
			end++ // include the newline
		}
		if strings.HasPrefix(patch[i:i+end], "@@ ") {
			if headerEnd < 0 {
				headerEnd = i
			}
			starts = append(starts, i)
		}
		i += end
	}
	if len(starts) == 0 {
		return "", errors.New("patch has no hunks")
	}

	header := patch[:headerEnd]
	if strings.Contains(header, "new file mode") || strings.Contains(header, "deleted file mode") {
		// A partial patch cannot create or remove a file, so these apply whole.
		return patch, nil
	}
	if len(want) == 0 {
		return patch, nil
	}

	seen := make(map[int]bool, len(want))
	indexes := make([]int, 0, len(want))
	for _, i := range want {
		if i < 0 || i >= len(starts) {
			return "", fmt.Errorf("hunk %d out of range", i)
		}
		if !seen[i] {
			seen[i] = true
			indexes = append(indexes, i)
		}
	}
	sort.Ints(indexes)

	var b strings.Builder
	b.WriteString(header)
	for _, i := range indexes {
		end := len(patch)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		b.WriteString(patch[starts[i]:end])
	}
	return b.String(), nil
}

// safeJoin joins a repository-relative path to root and refuses escapes.
func safeJoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute path")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes repository")
	}
	return filepath.Join(root, clean), nil
}

func looksBinary(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.IndexByte(data, 0) >= 0
}

func isBinaryPatch(patch string) bool {
	return strings.Contains(patch, "Binary files ") ||
		strings.Contains(patch, "GIT binary patch")
}
