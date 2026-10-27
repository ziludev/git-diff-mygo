package gitx

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// Branch is the checked out branch and its relation to its upstream.
type Branch struct {
	OID      string `json:"oid,omitempty"`
	Head     string `json:"head,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	// Detached is true when HEAD points at a commit instead of a branch.
	Detached bool `json:"detached"`
}

// Change is one file in the working tree, the index, or both.
type Change struct {
	Path       string `json:"path"`
	OrigPath   string `json:"origPath,omitempty"`
	X          string `json:"x"`
	Y          string `json:"y"`
	Staged     bool   `json:"staged"`
	Untracked  bool   `json:"untracked"`
	Conflicted bool   `json:"conflicted"`
}

// Status is the repository state shown by the changes panel.
type Status struct {
	Root       string   `json:"root"`
	Branch     Branch   `json:"branch"`
	Staged     []Change `json:"staged"`
	Unstaged   []Change `json:"unstaged"`
	Untracked  []Change `json:"untracked"`
	Conflicted []Change `json:"conflicted"`
}

// Status runs git status and parses its porcelain v2 output.
func (r *Runner) Status(ctx context.Context) (Status, error) {
	out, err := r.Run(ctx,
		"status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return NewStatus(r.Dir), err
	}
	return ParseStatus(r.Dir, out), nil
}

// NewStatus returns a status with empty (non-nil) change lists. The page
// indexes and filters these slices, so they must never be nil in JSON.
func NewStatus(root string) Status {
	return Status{
		Root:       root,
		Staged:     []Change{},
		Unstaged:   []Change{},
		Untracked:  []Change{},
		Conflicted: []Change{},
	}
}

// ParseStatus parses `git status --porcelain=v2 -z --branch`.
// The format is documented in git-status(1) under "Porcelain Format Version 2".
func ParseStatus(root string, out []byte) Status {
	st := NewStatus(root)
	tokens := strings.Split(string(out), "\x00")
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "" {
			continue
		}
		switch {
		case strings.HasPrefix(tok, "# branch.oid "):
			oid := strings.TrimPrefix(tok, "# branch.oid ")
			if oid != "(initial)" {
				st.Branch.OID = oid
			}
		case strings.HasPrefix(tok, "# branch.head "):
			head := strings.TrimPrefix(tok, "# branch.head ")
			if head == "(detached)" {
				st.Branch.Detached = true
			} else {
				st.Branch.Head = head
			}
		case strings.HasPrefix(tok, "# branch.upstream "):
			st.Branch.Upstream = strings.TrimPrefix(tok, "# branch.upstream ")
		case strings.HasPrefix(tok, "# branch.ab "):
			// "# branch.ab +1 -2"
			fields := strings.Fields(strings.TrimPrefix(tok, "# branch.ab "))
			if len(fields) == 2 {
				st.Branch.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
				st.Branch.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
			}
		case strings.HasPrefix(tok, "1 "):
			// 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			f := strings.SplitN(tok, " ", 9)
			if len(f) == 9 {
				addChange(&st, f[1], f[8], "")
			}
		case strings.HasPrefix(tok, "2 "):
			// 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>
			// followed by a NUL and the original path.
			f := strings.SplitN(tok, " ", 10)
			if len(f) == 10 {
				orig := ""
				if i+1 < len(tokens) {
					orig = tokens[i+1]
					i++
				}
				addChange(&st, f[1], f[9], orig)
			}
		case strings.HasPrefix(tok, "u "):
			// u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
			f := strings.SplitN(tok, " ", 11)
			if len(f) == 11 && len(f[1]) == 2 {
				st.Conflicted = append(st.Conflicted, Change{
					Path:       f[10],
					X:          string(f[1][0]),
					Y:          string(f[1][1]),
					Conflicted: true,
				})
			}
		case strings.HasPrefix(tok, "? "):
			st.Untracked = append(st.Untracked, Change{
				Path:      tok[2:],
				Untracked: true,
			})
		}
	}
	sortChanges(st.Staged)
	sortChanges(st.Unstaged)
	sortChanges(st.Untracked)
	sortChanges(st.Conflicted)
	return st
}

func addChange(st *Status, xy, path, orig string) {
	if len(xy) != 2 {
		return
	}
	x, y := xy[0], xy[1]
	if x == '.' && y == '.' {
		return
	}
	base := Change{Path: path, OrigPath: orig, X: string(x), Y: string(y)}
	if x != '.' {
		c := base
		c.Staged = true
		st.Staged = append(st.Staged, c)
	}
	if y != '.' {
		st.Unstaged = append(st.Unstaged, base)
	}
}

func sortChanges(cs []Change) {
	sort.Slice(cs, func(i, j int) bool {
		return cs[i].Path < cs[j].Path
	})
}
