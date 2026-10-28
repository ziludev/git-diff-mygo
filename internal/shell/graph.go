package shell

import (
	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

const (
	// graphLaneWidth is the distance between two lanes, in DIPs.
	graphLaneWidth float32 = 14
	// graphRowHeight is the height of one commit row; the graph fills it. The
	// row is a single line of columns, so this is a text line plus padding.
	graphRowHeight float32 = 30
)

// graphColors are lane colors, cycled by lane index.
var graphColors = []ui.Color{
	ui.Hex("#4f8cff"),
	ui.Hex("#a78bfa"),
	ui.Hex("#34d399"),
	ui.Hex("#fbbf24"),
	ui.Hex("#fb7185"),
	ui.Hex("#22d3ee"),
	ui.Hex("#fb923c"),
	ui.Hex("#2dd4bf"),
}

// graphSegment is a vertical line in one lane, from From to To in row heights
// (0 is the row's top, 1 its bottom).
type graphSegment struct {
	Lane  int
	Color int
	From  float32
	To    float32
}

// graphEdge is a line from the commit's node to one of its parents' lanes.
type graphEdge struct {
	From  int
	To    int
	Color int
}

// graphRow is how one commit row draws: its lane, the lines continuing above
// and below it, and the edges to its parents.
type graphRow struct {
	Lane     int
	Color    int
	Segments []graphSegment
	Edges    []graphEdge
}

// layoutGraph lays out commit lanes the way IDEA's log does: a lane is a
// promise to draw a particular object next, so the lane that expects this
// commit receives it, its first parent continues there, and every extra parent
// opens or joins another lane. Rows are in commit order, newest first.
//
// It is a port of the web view's src/lib/graph.ts and only depends on the
// commits given, so appending a page recomputes the whole list.
func layoutGraph(commits []gitx.Commit) ([]graphRow, int) {
	lanes := []string{} // "" is a free lane
	colors := []int{}
	rows := make([]graphRow, 0, len(commits))
	nextColor := 0
	width := 0

	freeLane := func(state *[]string, skip int) int {
		for i, v := range *state {
			if i != skip && v == "" {
				return i
			}
		}
		*state = append(*state, "")
		return len(*state) - 1
	}
	colorOf := func(i int) int {
		if i >= 0 && i < len(colors) {
			return colors[i]
		}
		return 0
	}
	setColor := func(i int) {
		for len(colors) <= i {
			colors = append(colors, 0)
		}
		colors[i] = nextColor % len(graphColors)
		nextColor++
	}
	indexOf := func(state []string, oid string) int {
		for i, v := range state {
			if v == oid {
				return i
			}
		}
		return -1
	}

	for _, commit := range commits {
		before := append([]string(nil), lanes...)

		// The lane waiting for this commit, or a free one when the commit is
		// a branch tip no lane has reached yet.
		lane := indexOf(lanes, commit.OID)
		if lane == -1 {
			lane = freeLane(&lanes, -1)
			setColor(lane)
		}
		lanes[lane] = commit.OID

		after := append([]string(nil), lanes...)
		after[lane] = ""
		parents := commit.Parents
		targets := []int{}

		if len(parents) > 0 {
			first := parents[0]
			// An older lane may already wait for this parent: join it rather
			// than opening a duplicate lane for the same commit.
			existing := indexOf(before, first)
			if existing != -1 && existing != lane {
				targets = append(targets, existing)
			} else {
				after[lane] = first
				targets = append(targets, lane)
			}
		}
		for i := 1; i < len(parents); i++ {
			parent := parents[i]
			if existing := indexOf(after, parent); existing != -1 {
				targets = append(targets, existing)
				continue
			}
			extra := freeLane(&after, lane)
			after[extra] = parent
			setColor(extra)
			targets = append(targets, extra)
		}

		count := max(len(before), len(after))
		segments := []graphSegment{}
		for i := 0; i < count; i++ {
			existedBefore := i < len(before) && before[i] != ""
			existsAfter := i < len(after) && after[i] != ""
			if i == lane {
				if existedBefore {
					segments = append(segments, graphSegment{Lane: i, Color: colorOf(i), From: 0, To: 0.5})
				}
				continue
			}
			if existedBefore && existsAfter {
				segments = append(segments, graphSegment{Lane: i, Color: colorOf(i), From: 0, To: 1})
			} else if existedBefore {
				// Two lanes were waiting for the same commit; this one ends
				// here.
				segments = append(segments, graphSegment{Lane: i, Color: colorOf(i), From: 0, To: 0.5})
			}
		}
		edges := make([]graphEdge, 0, len(targets))
		for _, to := range targets {
			edges = append(edges, graphEdge{From: lane, To: to, Color: colorOf(to)})
		}

		rows = append(rows, graphRow{
			Lane:     lane,
			Color:    colorOf(lane),
			Segments: segments,
			Edges:    edges,
		})
		width = max(width, count)

		lanes = after
		// Trailing free lanes are not drawn; dropping them keeps the column
		// from growing with every finished branch.
		for len(lanes) > 0 && lanes[len(lanes)-1] == "" {
			lanes = lanes[:len(lanes)-1]
		}
	}
	return rows, width
}
