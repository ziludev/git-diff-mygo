package shell

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// changeSection is one bucket of the changes list. key names its fold state in
// closedDirs; folds marks the one section whose header opens and closes; tree
// is the section's directory tree in tree mode.
type changeSection struct {
	key   string
	title string
	items []gitx.Change
	folds bool
	tree  *changeNode
}

// sectionKey is a section's fold key in closedDirs.
func sectionKey(section changeSection) string { return "s:" + section.key }

// changeNode is one directory of a section's tree. The summary and count are
// filled once, while the tree is built, so drawing a frame only reads them.
type changeNode struct {
	name    string
	key     string
	items   []gitx.Change // the files directly in this folder
	dirs    []*changeNode
	summary string
	count   int
}

// changeCache is the Changes tab's derived list: the sections and, in tree
// mode, their directory trees. It is rebuilt when the status, the filter or the
// layout changes, so drawing a frame only walks it.
type changeCache struct {
	sections []changeSection
	version  uint64
	needle   string
	tree     bool
	built    bool
}

// folderAction is the one action a folder offers: every file under it sits in
// the same section, and a section holds a single bucket.
type folderAction int

const (
	folderActionNone folderAction = iota
	folderActionStage
	folderActionUnstage
)

// folderAction is decided by the section, not by sampling a file: the bucket
// already says what every row in it needs.
func (s changeSection) folderAction() folderAction {
	switch s.key {
	case "staged":
		return folderActionUnstage
	case "changes", "untracked":
		return folderActionStage
	}
	return folderActionNone
}

// changeTreeOf groups a section's changes under their directories. Paths
// arrive sorted from git, so folders and files keep the order the flat list
// showed them in.
func changeTreeOf(section string, items []gitx.Change) *changeNode {
	root := &changeNode{}
	byPath := map[string]*changeNode{"": root}
	// letters is build-time state only: finalize turns each set into the
	// summary a folder row shows and drops it.
	letters := map[*changeNode]map[string]bool{}
	mark := func(n *changeNode, ch gitx.Change) {
		set := letters[n]
		if set == nil {
			set = map[string]bool{}
			letters[n] = set
		}
		set[changeLetter(ch)] = true
		n.count++
	}
	for _, ch := range items {
		cur, dir := root, ""
		mark(root, ch)
		parts := strings.Split(ch.Path, "/")
		for _, part := range parts[:len(parts)-1] {
			if dir == "" {
				dir = part
			} else {
				dir += "/" + part
			}
			next, ok := byPath[dir]
			if !ok {
				next = &changeNode{name: part, key: section + ":" + dir}
				byPath[dir] = next
				cur.dirs = append(cur.dirs, next)
			}
			mark(next, ch)
			cur = next
		}
		cur.items = append(cur.items, ch)
	}
	finalize(root, letters)
	return root
}

// files lists every file under the folder, subfolders included.
func (n *changeNode) files() []gitx.Change {
	out := make([]gitx.Change, 0, n.count)
	var walk func(*changeNode)
	walk = func(n *changeNode) {
		for _, d := range n.dirs {
			walk(d)
		}
		out = append(out, n.items...)
	}
	walk(n)
	return out
}

// finalize turns each node's letter set into the sorted summary a folder row
// shows, in one walk.
func finalize(n *changeNode, letters map[*changeNode]map[string]bool) {
	codes := make([]string, 0, len(letters[n]))
	for l := range letters[n] {
		codes = append(codes, l)
	}
	sort.Strings(codes)
	n.summary = strings.Join(codes, " ")
	for _, d := range n.dirs {
		finalize(d, letters)
	}
}

// changesView is the working tree: conflicts, the index and the worktree,
// with the commit box under the list.
func (a *App) changesView(c *ui.Context, p palette) {
	ui.Column(c).FillHeight().FillWidth().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(6).Gap(6).Children(func() {
			a.takeFilterFocus(ViewChanges, a.filterInput(c, &a.changeFilter, "Filter files"))
			iconButton(c, iconArchive, "Stash", "Stash the uncommitted changes", !a.canStash(), a.stashChanges)
		})

		needle := filterNeedle(a.changeFilter)
		// Up and Down walk the rows this frame draws.
		a.changeOrder = a.changeOrder[:0]
		// A new filter opens every folder and the untracked section once, so
		// no match hides behind a fold; closing them while filtering works.
		if needle != a.changeFilterSeen {
			a.changeFilterSeen = needle
			a.closedDirs = map[string]bool{}
		}
		sections := a.changeSectionsFor(needle)
		empty := true
		for _, s := range sections {
			if len(s.items) > 0 {
				empty = false
				break
			}
		}

		list := func() {
			a.scrollColumn(c, func() {
				if empty {
					if needle != "" {
						emptyState(c, "No matching files", "", "", nil)
					} else {
						emptyState(c, "Working tree clean", "", "", nil)
					}
					return
				}
				for _, section := range sections {
					if len(section.items) == 0 {
						continue
					}
					open := !section.folds || !a.closedDirs[sectionKey(section)]
					a.changeSectionHeader(c, section, open)
					if !open {
						continue
					}
					if a.changesTree {
						a.changeTreeRows(c, p, section, section.tree, 0)
						continue
					}
					for _, ch := range section.items {
						a.changeRow(c, p, ch, ch.Path, 0)
					}
				}
			})
		}

		// The list and the commit box share the sidebar through a divider
		// the user drags: a long file list leaves the message room to grow,
		// and a tall message squeezes the list. Percent rather than a fixed
		// height keeps both panes in proportion when the window resizes; the
		// split keeps 40 DIPs for each and moves the divider with the arrows
		// once it has the focus.
		ui.SplitVerticalPercent(c, &a.changesPercent, list, func() { a.commitBox(c, p) }).Grow(1)
	})
}

// changeSectionsFor returns the sections for this status and filter, building
// each section's directory tree in tree mode. The result is cached: it is
// rebuilt only when the status, the filter or the layout changes.
func (a *App) changeSectionsFor(needle string) []changeSection {
	c := &a.changeCache
	if c.built && c.version == a.statusVersion && c.needle == needle && c.tree == a.changesTree {
		return c.sections
	}
	sections := []changeSection{
		{key: "conflicts", title: "Conflicts", items: filterChanges(a.status.Conflicted, needle)},
		{key: "staged", title: "Staged", items: filterChanges(a.status.Staged, needle)},
		{key: "changes", title: "Changes", items: filterChanges(a.status.Unstaged, needle)},
		{key: "untracked", title: "Untracked", items: filterChanges(a.status.Untracked, needle), folds: true},
	}
	if a.changesTree {
		for i := range sections {
			sections[i].tree = changeTreeOf(sections[i].key, sections[i].items)
		}
	}
	*c = changeCache{sections: sections, version: a.statusVersion, needle: needle, tree: a.changesTree, built: true}
	return sections
}

// filterChanges keeps the changes whose path matches a normalized filter. An
// empty filter returns the slice itself, without a copy.
func filterChanges(items []gitx.Change, needle string) []gitx.Change {
	if needle == "" {
		return items
	}
	out := make([]gitx.Change, 0, len(items))
	for _, ch := range items {
		if nameMatches(needle, ch.Path) {
			out = append(out, ch)
		}
	}
	return out
}

// changeSectionHeader is a section's title row. The untracked section folds:
// a fresh checkout can leave thousands of new files, so it starts closed. The
// tracked sections have nothing to fold.
func (a *App) changeSectionHeader(c *ui.Context, section changeSection, open bool) {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Height(24).PaddingX(8).Gap(6).Clip()
	row.Children(func() {
		if section.folds {
			chev := iconChevronRight
			if open {
				chev = iconChevronDown
			}
			icon(c, chev).FontSize(9).TextColor(t.TextMuted)
		}
		ui.Text(c, strings.ToUpper(section.title)).FontSize(9).Bold().TextColor(t.TextMuted)
		ui.Text(c, strconv.Itoa(len(section.items))).FontSize(9).TextColor(t.TextMuted)
	})
	if !section.folds {
		return
	}
	row.Cursor(ui.CursorPointer)
	rowTint(row, t, false)
	row.OnClick(func() { a.closedDirs[sectionKey(section)] = open })
}

// changeTreeRows draws one directory's folders and files. A folder's row opens
// or closes it; the whole collection is the section's tree at that depth.
func (a *App) changeTreeRows(c *ui.Context, p palette, section changeSection, node *changeNode, depth int) {
	for _, dir := range node.dirs {
		open := !a.closedDirs[dir.key]
		a.changeDirRow(c, dir, section.folderAction(), depth, open, func() { a.closedDirs[dir.key] = open })
		if open {
			a.changeTreeRows(c, p, section, dir, depth+1)
		}
	}
	for _, ch := range node.items {
		a.changeRow(c, p, ch, path.Base(ch.Path), depth)
	}
}

// changeDirRow is a folder of the tree: chevron, name, and the status letters
// and file count of everything below it. Like a file row, the clickable part
// is separate from the stage/unstage button, which shows while the row is
// hovered; the context menu keeps the action reachable.
func (a *App) changeDirRow(c *ui.Context, dir *changeNode, action folderAction, depth int, open bool, toggle func()) {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Height(26).PaddingX(8 + float32(depth)*12).Gap(4).Clip()
	hovered := rowTint(row, t, false)
	row.Children(func() {
		left := ui.Row(c).Grow(1).Gap(4).ClipX().Cursor(ui.CursorPointer)
		left.Children(func() {
			chev := iconChevronRight
			if open {
				chev = iconChevronDown
			}
			icon(c, chev).FontSize(9).TextColor(t.TextMuted)
			ui.Text(c, dir.name).Font(mono()).FontSize(11).Grow(1).SingleLine()
			ui.Text(c, dir.summary).Font(mono()).FontSize(9).TextColor(t.TextMuted)
			ui.Text(c, strconv.Itoa(dir.count)).FontSize(9).TextColor(t.TextMuted)
		})
		left.OnClick(toggle)
		if hovered {
			// The section decides the action: every file under the folder is in
			// the same bucket.
			switch action {
			case folderActionStage:
				iconButton(c, iconPlus, "Stage folder", "Stage the files in this folder", a.busy(),
					func() { a.stageFolder(dir.files()) })
			case folderActionUnstage:
				iconButton(c, iconMinus, "Unstage folder", "Unstage the files in this folder", a.busy(),
					func() { a.unstageFolder(dir.files()) })
			}
		}
	})
	row.ContextMenu(func(m *ui.Menu) {
		switch action {
		case folderActionUnstage:
			if m.Item("Unstage Folder").Disabled(a.busy()).Chosen() {
				a.unstageFolder(dir.files())
			}
		case folderActionStage:
			if m.Item("Stage Folder").Disabled(a.busy()).Chosen() {
				a.stageFolder(dir.files())
			}
		}
	})
}

// changeRow is a clickable path plus its stage/unstage/discard buttons. The
// buttons sit outside the clickable part, so pressing one does not also
// select the row. They show while the row is hovered or selected, as IDEA
// does; the context menu keeps them reachable when they are hidden. depth
// indents a file under its folders; name is what the row shows.
func (a *App) changeRow(c *ui.Context, p palette, ch gitx.Change, name string, depth int) {
	t := c.Theme()
	selected := a.selection != nil && sameChange(*a.selection, ch)
	code := changeLetter(ch)
	tip := pathLabel(ch.Path, ch.OrigPath)

	a.rememberChange(ch)
	row := ui.Row(c).FillWidth().Height(28).PaddingX(8 + float32(depth)*12).Gap(6).Clip()
	hovered := rowTint(row, t, selected)
	if selected && a.revealRow {
		row.ScrollIntoView()
		a.revealRow = false
	}
	row.Tooltip(tip)
	row.Children(func() {
		left := ui.Row(c).Grow(1).Gap(6).ClipX().Cursor(ui.CursorPointer)
		left.Children(func() {
			ui.Text(c, code).Font(mono()).FontSize(10).Bold().
				Width(12).TextAlign(ui.Center).TextColor(statusColor(p, code))
			ui.Text(c, name).Font(mono()).FontSize(11).SingleLine()
		})
		left.OnClick(func() { a.loadDiff(ch) })

		if hovered || selected {
			ui.Row(c).Shrink(0).Gap(2).Children(func() {
				// A remote operation blocks writes too, so its buttons disable
				// with the same flag instead of silently doing nothing.
				if canStage(ch) {
					iconButton(c, iconPlus, "Stage", "Stage this file", a.busy(), func() { a.stage(ch) })
				}
				if canUnstage(ch) {
					iconButton(c, iconMinus, "Unstage", "Unstage this file", a.busy(), func() { a.unstage(ch) })
				}
				if canDiscard(ch) {
					iconButton(c, iconUndo, "Discard", "Discard changes to this file", a.busy(), func() { a.discard(ch) })
				}
			})
		}
	})
	row.ContextMenu(func(m *ui.Menu) { a.changeMenu(m, ch) })
}

// canStage, canUnstage and canDiscard are a row's action rules. The hover
// buttons and the context menu read the same predicates, so a new rule
// cannot land in one path only.
func canStage(ch gitx.Change) bool   { return !ch.Staged && !ch.Conflicted }
func canUnstage(ch gitx.Change) bool { return ch.Staged }
func canDiscard(ch gitx.Change) bool {
	return !ch.Staged && !ch.Untracked && !ch.Conflicted
}

// changeMenu builds a working-tree row's context menu. Items that do not
// apply to the row are not added, so the menu never offers a no-op; Stage and
// Unstage stay disabled while busy to say why a click would do nothing.
func (a *App) changeMenu(m *ui.Menu, ch gitx.Change) {
	if m.Item("Show Diff").Chosen() {
		a.loadDiff(ch)
	}
	// A conflict is resolved outside the app; there is nothing safe to offer
	// here yet.
	if ch.Conflicted {
		return
	}
	m.Separator()
	if canUnstage(ch) && m.Item("Unstage").Disabled(a.busy()).Chosen() {
		a.unstage(ch)
	}
	if canStage(ch) && m.Item("Stage").Disabled(a.busy()).Chosen() {
		a.stage(ch)
	}
	if canDiscard(ch) && m.Item("Discard Changes…").Disabled(a.busy()).Chosen() {
		a.discard(ch)
	}
}

// commitBox is the message and the commit button. The message survives a
// failed commit and every refresh; only a successful commit clears it.
func (a *App) commitBox(c *ui.Context, p palette) {
	t := c.Theme()
	staged := len(a.status.Staged)
	target := a.status.Branch.Head
	if target == "" {
		if a.status.Branch.Detached {
			target = "detached HEAD"
		} else {
			target = "branch"
		}
	}
	ui.Column(c).FillWidth().Shrink(0).Padding(8).Gap(6).Children(func() {
		message := ui.TextArea(c, &a.message).FillWidth().Height(64).FontSize(12).
			Placeholder("Commit message").Disabled(a.writing).Padding(6)
		// Cmd+Enter commits from the text area itself: a window shortcut does
		// not see the key while the input has the focus.
		message.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter || k.Mods != ui.Cmd {
				return false
			}
			a.message = k.Text()
			a.commit()
			return true
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Checkbox(c, &a.amend, "Amend").FontSize(10).
				TextColor(t.TextMuted).Disabled(a.writing)
			ui.Spacer(c)
			// Pushing an amended commit needs a force push, which the app does
			// not offer, so the combined action waits for Amend to be off.
			both := ui.Button(c, "Commit and Push…").Height(26).FontSize(11).
				Disabled(!a.canCommit() || a.amend)
			both.Tooltip("Commit, then review what will be pushed")
			both.OnClick(func() { a.commitThen(true) })
			label := "Commit to " + target
			btn := ui.PrimaryButton(c, label).Height(26).FontSize(11).
				Disabled(!a.canCommit())
			btn.Tooltip(label)
			btn.OnClick(a.commit)
		})
		hint := "Nothing staged"
		if staged > 0 {
			if staged == 1 {
				hint = "1 staged file"
			} else {
				hint = strconv.Itoa(staged) + " staged files"
			}
		}
		ui.Text(c, hint+" · Cmd+Enter").FontSize(9).TextColor(t.TextMuted)
	})
}

func changeLetter(ch gitx.Change) string {
	code := ch.Y
	if ch.Staged {
		code = ch.X
	}
	if code == "" || code == "." {
		return "?"
	}
	return code
}

func statusColor(p palette, code string) ui.Color {
	switch code {
	case "A":
		return p.addText
	case "M":
		return p.warnText
	case "D":
		return p.delText
	case "R", "C":
		return p.coolText
	default:
		return p.metaText
	}
}
