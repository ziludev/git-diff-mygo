package shell

import (
	"regexp"
	"strconv"
	"strings"
)

// lineKind is how the diff pane draws one line of a unified patch.
type lineKind int

const (
	lineMeta lineKind = iota
	lineHunk
	lineAdd
	lineDel
	lineContext
)

// patchLine is one row of the diff pane.
type patchLine struct {
	Kind  lineKind
	Text  string
	OldNo int
	NewNo int
	// Hunk is the index of the hunk this line belongs to, -1 before the first
	// one. Hunk header rows carry the index their actions apply to.
	Hunk int
}

// patchHunk describes one @@ header. OldCount is the old range the header
// declares; the diff pane needs it to find the unchanged lines between two
// hunks.
type patchHunk struct {
	Index     int
	OldStart  int
	NewStart  int
	OldCount  int
	Additions int
	Deletions int
}

// patchFile is a parsed unified diff.
type patchFile struct {
	Lines     []patchLine
	Hunks     []patchHunk
	Additions int
	Deletions int
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parsePatch splits a unified diff into drawable lines and its hunk headers.
// It never fails: an unrecognized line is metadata, which keeps a rename or a
// mode change visible instead of hiding the whole patch.
func parsePatch(patch string) patchFile {
	var out patchFile
	if patch == "" {
		return out
	}
	lines := strings.Split(patch, "\n")
	// A trailing newline ends the last line; it is not an empty line.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	hunk := -1
	oldNo, newNo := 0, 0
	for _, text := range lines {
		switch {
		case strings.HasPrefix(text, "@@ "):
			m := hunkHeader.FindStringSubmatch(text)
			hunk++
			oldStart, newStart, oldCount := 0, 0, 0
			if m != nil {
				oldStart, _ = strconv.Atoi(m[1])
				newStart, _ = strconv.Atoi(m[3])
				// A header without a count means one line.
				oldCount = 1
				if m[2] != "" {
					oldCount, _ = strconv.Atoi(m[2])
				}
			}
			oldNo, newNo = oldStart, newStart
			out.Hunks = append(out.Hunks, patchHunk{
				Index:    hunk,
				OldStart: oldStart,
				NewStart: newStart,
				OldCount: oldCount,
			})
			out.Lines = append(out.Lines, patchLine{Kind: lineHunk, Text: text, Hunk: hunk})
		case hunk >= 0 && strings.HasPrefix(text, "+"):
			out.Lines = append(out.Lines, patchLine{Kind: lineAdd, Text: text, NewNo: newNo, Hunk: hunk})
			out.Additions++
			out.Hunks[hunk].Additions++
			newNo++
		case hunk >= 0 && strings.HasPrefix(text, "-"):
			out.Lines = append(out.Lines, patchLine{Kind: lineDel, Text: text, OldNo: oldNo, Hunk: hunk})
			out.Deletions++
			out.Hunks[hunk].Deletions++
			oldNo++
		case hunk >= 0 && strings.HasPrefix(text, " "):
			out.Lines = append(out.Lines, patchLine{
				Kind: lineContext, Text: text, OldNo: oldNo, NewNo: newNo, Hunk: hunk,
			})
			oldNo++
			newNo++
		case hunk >= 0 && strings.HasPrefix(text, `\`):
			// "\ No newline at end of file" belongs to the hunk but has no
			// line number of its own.
			out.Lines = append(out.Lines, patchLine{Kind: lineMeta, Text: text, Hunk: hunk})
		default:
			out.Lines = append(out.Lines, patchLine{Kind: lineMeta, Text: text, Hunk: -1})
		}
	}
	return out
}
