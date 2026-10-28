package gitx

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
)

const (
	// defaultLogLimit is one page of history; the page is also what "load
	// more" appends, so it stays small enough to render in one pass.
	defaultLogLimit = 100
	// maxLogLimit bounds what the page may ask for in one call.
	maxLogLimit = 500
)

// Ref is a branch, remote branch or tag pointing at a commit. Kind is one of
// "branch", "remote" or "tag"; Name is the short name the log displays.
type Ref struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Commit is one row of the history list. Parents drives the graph layout: a
// row with two parents is a merge, and its extra parents open new lanes.
type Commit struct {
	OID     string   `json:"oid"`
	Short   string   `json:"short"`
	Subject string   `json:"subject"`
	Author  string   `json:"author"`
	Time    int64    `json:"time"` // author time, unix seconds
	Parents []string `json:"parents"`
	Refs    []Ref    `json:"refs"`
}

// LogRequest selects a page of the repository history.
type LogRequest struct {
	Limit int `json:"limit"`
	Skip  int `json:"skip"`
}

// FileEntry is one file changed by a commit.
type FileEntry struct {
	Path      string `json:"path"`
	OrigPath  string `json:"origPath,omitempty"`
	Status    string `json:"status"` // A, M, D, R, C, T (or U for unmerged)
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
}

// CommitDiffRequest asks for one file's patch inside a commit.
type CommitDiffRequest struct {
	OID      string `json:"oid"`
	Path     string `json:"path"`
	OrigPath string `json:"origPath,omitempty"`
}

// Log returns one page of the history of every ref, newest first. All refs are
// included so a branch that is not merged into HEAD still appears on the
// graph. An unborn branch has no commits and yields an empty list rather than
// an error.
func (r *Runner) Log(ctx context.Context, req LogRequest) ([]Commit, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}
	skip := req.Skip
	if skip < 0 {
		skip = 0
	}

	// --topo-order keeps every parent below its children, which the lane
	// layout assumes: without it, a merge's parents can appear before the
	// merge itself and lanes run backwards.
	out, err := r.Run(ctx, "log", "--all", "--no-color", "--no-show-signature", "--topo-order",
		"--max-count="+strconv.Itoa(limit), "--skip="+strconv.Itoa(skip),
		"--format=%H%x1f%h%x1f%an%x1f%at%x1f%P%x1f%s%x00", "--")
	if err != nil {
		// "HEAD does not have any commits yet" (or no refs at all): an empty
		// history, not a failure.
		if !r.HeadExists(ctx) {
			return []Commit{}, nil
		}
		return []Commit{}, err
	}
	commits := parseLog(out)

	// Labels are decoration: a failed ref listing leaves them off rather than
	// failing the list the reader is about to use.
	if refs, refErr := r.refsByOID(ctx); refErr == nil {
		for i := range commits {
			if list, ok := refs[commits[i].OID]; ok {
				commits[i].Refs = list
			}
		}
	}
	return commits, nil
}

// refKindOrder sorts the labels of one commit: local branches first, then
// remote-tracking branches, then tags.
func refKindOrder(kind string) int {
	switch kind {
	case "branch":
		return 0
	case "remote":
		return 1
	default:
		return 2
	}
}

// refsByOID maps object ids to the branches and tags pointing at them.
func (r *Runner) refsByOID(ctx context.Context) (map[string][]Ref, error) {
	out, err := r.Run(ctx, "for-each-ref", "--format=%(objectname)%09%(refname)")
	if err != nil {
		return nil, err
	}
	refs := make(map[string][]Ref)
	for _, line := range strings.Split(string(out), "\n") {
		oid, ref, ok := strings.Cut(line, "\t")
		if !ok || oid == "" {
			continue
		}
		var kind, name string
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			kind, name = "branch", strings.TrimPrefix(ref, "refs/heads/")
		case strings.HasPrefix(ref, "refs/remotes/"):
			kind, name = "remote", strings.TrimPrefix(ref, "refs/remotes/")
			// origin/HEAD is a symbolic pointer, not a branch of its own.
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
		case strings.HasPrefix(ref, "refs/tags/"):
			kind, name = "tag", strings.TrimPrefix(ref, "refs/tags/")
		default:
			continue
		}
		refs[oid] = append(refs[oid], Ref{Name: name, Kind: kind})
	}
	for _, list := range refs {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Kind != list[j].Kind {
				return refKindOrder(list[i].Kind) < refKindOrder(list[j].Kind)
			}
			return list[i].Name < list[j].Name
		})
	}
	return refs, nil
}

// parseLog parses the record and field separators of the Log format.
func parseLog(out []byte) []Commit {
	commits := []Commit{}
	for _, f := range splitRecords(out, 6) {
		ts, _ := strconv.ParseInt(f[3], 10, 64)
		parents := strings.Fields(f[4])
		if parents == nil {
			// The page appends these to lane state; never send null.
			parents = []string{}
		}
		commits = append(commits, Commit{
			OID:     f[0],
			Short:   f[1],
			Author:  f[2],
			Time:    ts,
			Parents: parents,
			Refs:    []Ref{},
			Subject: f[5],
		})
	}
	return commits
}

// CommitFiles lists the files a commit changed, diffed against its first
// parent. Two git calls run here: name-status carries the status letter and
// rename sides, numstat carries the line counts, and -z keeps paths exact.
func (r *Runner) CommitFiles(ctx context.Context, oid string) ([]FileEntry, error) {
	if oid == "" {
		return []FileEntry{}, errors.New("empty commit")
	}
	base := []string{"show", "--format=", "-M", "--first-parent", oid}
	statusOut, err := r.Run(ctx, append(append([]string{}, base...), "--name-status", "-z")...)
	if err != nil {
		return []FileEntry{}, err
	}
	numstatOut, err := r.Run(ctx, append(append([]string{}, base...), "--numstat", "-z")...)
	if err != nil {
		return []FileEntry{}, err
	}

	files := parseNameStatus(statusOut)
	counts := parseNumstat(numstatOut)
	for i := range files {
		if c, ok := counts[files[i].Path]; ok {
			files[i].Additions = c.Additions
			files[i].Deletions = c.Deletions
			files[i].Binary = c.Binary
		}
	}
	return files, nil
}

// CommitBody returns the commit message below its subject line, which is what
// the history detail panel shows under the subject. An empty commit message
// yields an empty string, not an error.
func (r *Runner) CommitBody(ctx context.Context, oid string) (string, error) {
	if oid == "" {
		return "", errors.New("empty commit")
	}
	out, err := r.Run(ctx, "show", "-s", "--format=%b", oid)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// parseNameStatus parses `--name-status -z`: STATUS NUL path NUL, with a
// second path for renames and copies.
func parseNameStatus(out []byte) []FileEntry {
	tokens := strings.Split(string(out), "\x00")
	files := []FileEntry{}
	for i := 0; i < len(tokens); i++ {
		status := strings.TrimSpace(tokens[i])
		if status == "" {
			continue
		}
		if i+1 >= len(tokens) {
			break
		}
		code := status[:1]
		entry := FileEntry{Status: code, Path: tokens[i+1]}
		i++
		if (code == "R" || code == "C") && i+1 < len(tokens) {
			entry.OrigPath = entry.Path
			entry.Path = tokens[i+1]
			i++
		}
		files = append(files, entry)
	}
	return files
}

type numstat struct {
	Additions int
	Deletions int
	Binary    bool
}

// parseNumstat parses `--numstat -z`. A rename writes an empty path, then the
// old and new paths, as separate NUL-terminated tokens.
func parseNumstat(out []byte) map[string]numstat {
	counts := make(map[string]numstat)
	tokens := strings.Split(string(out), "\x00")
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "" {
			continue
		}
		f := strings.SplitN(tok, "\t", 3)
		if len(f) != 3 {
			continue
		}
		path := f[2]
		if path == "" {
			if i+2 >= len(tokens) {
				break
			}
			path = tokens[i+2]
			i += 2
		}
		c := numstat{Binary: f[0] == "-" && f[1] == "-"}
		c.Additions, _ = strconv.Atoi(f[0])
		c.Deletions, _ = strconv.Atoi(f[1])
		counts[path] = c
	}
	return counts
}

// CommitDiff returns one file's patch inside a commit, against its first
// parent. Both sides of a rename are passed so git emits the rename header.
func (r *Runner) CommitDiff(ctx context.Context, req CommitDiffRequest) (DiffResult, error) {
	if req.OID == "" {
		return DiffResult{Path: req.Path}, errors.New("empty commit")
	}
	if req.Path == "" {
		return DiffResult{}, errors.New("empty path")
	}
	return r.patch(ctx, req.Path, req.OrigPath,
		"show", "--format=", "--no-color", "--no-ext-diff", "--no-textconv",
		"--no-show-signature", "--first-parent", "-M",
		"-U"+strconv.Itoa(defaultContext), req.OID)
}
