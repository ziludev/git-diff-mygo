package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// maxContextBytes bounds one context read; a multi-megabyte blob would stall
// the UI thread that slices it. How many lines may be shown is the caller's
// decision: it knows the size of the gap, not just the file.
const maxContextBytes = 8 << 20

// tooLarge is the error a read returns for a file this view refuses to expand,
// whichever side it would have read.
func tooLarge(path string) error {
	return fmt.Errorf("%s is larger than the %d MiB this view expands", path, maxContextBytes>>20)
}

// FileTextRequest asks for the text of one side of a diff.
type FileTextRequest struct {
	Path string `json:"path"`
	// Rev selects the side: "" is the working tree, "index" is the index, and
	// any other value is a revision such as a commit id.
	Rev string `json:"rev"`
}

// FileLines returns path's lines at the requested revision. It backs the diff
// pane's context expansion only: staging still applies the hunk indexes of the
// patch on screen, so a read must never re-run `git diff` with wider context.
func (r *Runner) FileLines(ctx context.Context, req FileTextRequest) ([]string, error) {
	if req.Path == "" {
		return nil, errors.New("empty path")
	}
	var content []byte
	if req.Rev == "" {
		full, err := safeJoin(r.Dir, req.Path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(full)
		if err != nil {
			return nil, err
		}
		if info.Size() > maxContextBytes {
			return nil, tooLarge(req.Path)
		}
		content, err = os.ReadFile(full)
		if err != nil {
			return nil, err
		}
	} else {
		// A revision:path spec splits at its first colon, so a path that
		// contains one cannot be addressed. Refusing beats reading a blob the
		// user did not click.
		if strings.Contains(req.Path, ":") {
			return nil, fmt.Errorf("cannot read %q at %q: path contains ':'", req.Path, req.Rev)
		}
		spec := req.Rev + ":" + req.Path
		if req.Rev == "index" {
			// Stage 0 is the merged index entry; a conflicted path has none,
			// and the read fails, which the caller turns into a toast.
			spec = ":0:" + req.Path
		}
		// Ask for the size first: reading a blob only to reject it would copy
		// the whole thing into memory.
		size, err := r.Run(ctx, "cat-file", "-s", spec)
		if err != nil {
			return nil, err
		}
		if n, err := strconv.Atoi(strings.TrimSpace(string(size))); err == nil && n > maxContextBytes {
			return nil, tooLarge(req.Path)
		}
		content, err = r.Run(ctx, "cat-file", "-p", spec)
		if err != nil {
			return nil, err
		}
	}
	if len(content) > maxContextBytes {
		// The file grew between the size check and the read.
		return nil, tooLarge(req.Path)
	}
	if looksBinary(content) {
		return nil, fmt.Errorf("%s is binary", req.Path)
	}

	lines := strings.Split(string(content), "\n")
	// A trailing newline ends the last line; it does not start an empty one.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines, nil
}
