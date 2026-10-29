package shell

import (
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// The branch tree's sections, and the prefixes of its row keys. Keys are stable
// across refreshes, so an OutlineState keeps a folder open while the list
// reloads: "s:local", "d:local:feature", "b:local:feature/x".
const (
	branchLocalSection  = "s:local"
	branchRemoteSection = "s:remote"
)

// branchTreeSections is also the order the tree shows them in.
var branchTreeSections = []struct {
	key   string
	label string
}{
	{branchLocalSection, "Local"},
	{branchRemoteSection, "Remote"},
}

// branchTree is the display form of a branch list: two sections, folders split
// from each branch name's "/" segments, and a leaf per branch.
type branchTree struct {
	roots    []string
	children map[string][]string
	nodes    map[string]branchNode
	// counts holds the number of branches below a section or folder.
	counts map[string]int
}

type branchNode struct {
	label   string
	section bool
	// leaf is nil for a section or folder, which are grouping rows.
	leaf *gitx.BranchEntry
}

// branchTreeState is one tree's place: its filter, the folders it has open and
// the row it has chosen. The tab and the popover each own one, so closing one
// does not move the other.
type branchTreeState struct {
	filter  string
	row     int
	outline ui.OutlineState[string]
	seeded  bool
	// tree is the tree the outline draws and the keyboard walks. It is rebuilt
	// only when the branch list or the filter changes, not on every frame.
	tree        branchTree
	treeVersion uint64
	treeFilter  string
	treeBuilt   bool
	// appliedFilter is the filter the folders were last opened for: a filter
	// expands its matches once, not on every frame, so the user can still
	// collapse a folder while filtering.
	appliedFilter string
}

// newBranchTreeState returns a tree with no chosen row: a tree the user has not
// touched must not highlight its first branch as if they had picked it.
func newBranchTreeState() branchTreeState { return branchTreeState{row: -1} }

// reset drops the place this tree held in the repository the user left. The
// filter stays: it is the user's own words, not repository data.
func (s *branchTreeState) reset() {
	s.outline.Open.Clear()
	s.outline.List.Selected = nil
	s.row = -1
	s.seeded = false
	s.tree = branchTree{}
	s.treeBuilt = false
	s.appliedFilter = ""
}

// buildBranchTree groups the branches that pass needle by their "/" segments.
// A name that is also a folder keeps its own leaf beside the folder, so "feat"
// and "feat/x" both stay reachable.
func buildBranchTree(branches []gitx.BranchEntry, needle string) branchTree {
	t := branchTree{
		children: map[string][]string{},
		nodes:    map[string]branchNode{},
		counts:   map[string]int{},
	}
	for i := range branches {
		b := &branches[i]
		if !nameMatches(needle, b.Name) {
			continue
		}
		section := branchLocalSection
		if b.Remote {
			section = branchRemoteSection
		}
		t.insert(section, b)
	}
	for _, section := range branchTreeSections {
		if len(t.children[section.key]) == 0 {
			continue
		}
		t.nodes[section.key] = branchNode{label: section.label, section: true}
		t.roots = append(t.roots, section.key)
	}
	t.sortChildren()
	return t
}

// insert adds one branch under its section, creating the folders its name
// needs and counting it into the section and every folder above it. A remote's
// first segment is the remote's name, so origin/main and upstream/main stay
// apart.
func (t *branchTree) insert(section string, b *gitx.BranchEntry) {
	t.counts[section]++
	segments := strings.Split(b.Name, "/")
	parent := section
	for i := 0; i < len(segments)-1; i++ {
		key := branchFolderKey(section, strings.Join(segments[:i+1], "/"))
		if _, ok := t.nodes[key]; !ok {
			t.nodes[key] = branchNode{label: segments[i]}
			t.children[parent] = append(t.children[parent], key)
		}
		t.counts[key]++
		parent = key
	}
	key := branchLeafKey(section, b.Name)
	t.nodes[key] = branchNode{label: segments[len(segments)-1], leaf: b}
	t.children[parent] = append(t.children[parent], key)
}

// sortChildren orders every group folders first, then leaves by label, with the
// checked-out branch above the other leaves.
func (t *branchTree) sortChildren() {
	for _, children := range t.children {
		sort.SliceStable(children, func(i, j int) bool {
			a, b := t.nodes[children[i]], t.nodes[children[j]]
			if (a.leaf == nil) != (b.leaf == nil) {
				return a.leaf == nil
			}
			if a.leaf != nil && a.leaf.Current != b.leaf.Current {
				return a.leaf.Current
			}
			if a.label != b.label {
				return a.label < b.label
			}
			return children[i] < children[j]
		})
	}
}

func branchLeafKey(section, name string) string   { return "b:" + section + ":" + name }
func branchFolderKey(section, path string) string { return "d:" + section + ":" + path }

// revealCurrentBranch opens the folders above the checked-out branch in both
// trees when HEAD moves, so a branch created or checked out into a new folder
// is visible even after the trees' first list. Folders the user closed stay
// open states of their own; only the current branch's ancestors are added.
func (a *App) revealCurrentBranch() {
	b := a.status.Branch
	if b.Head == "" || b.Detached || b.Head == a.revealedBranch {
		return
	}
	a.revealedBranch = b.Head
	segments := strings.Split(b.Head, "/")
	keys := []string{branchLocalSection}
	for i := 0; i < len(segments)-1; i++ {
		keys = append(keys, branchFolderKey(branchLocalSection, strings.Join(segments[:i+1], "/")))
	}
	for _, st := range []*branchTreeState{&a.branchTab, &a.branchPopupTree} {
		for _, key := range keys {
			st.outline.Open.Add(key)
		}
	}
}

// branchTreeView draws st's tree and keeps its row selection in st.row. It
// reports whether any branch passed the filter; otherwise the caller draws its
// empty state. maxHeight caps the tree in a popover; 0 lets it take the room a
// side panel has left.
func (a *App) branchTreeView(c *ui.Context, st *branchTreeState, maxHeight float32) bool {
	needle := filterNeedle(st.filter)
	if !st.treeBuilt || st.treeVersion != a.branchesVersion || st.treeFilter != needle {
		st.tree = buildBranchTree(a.branches, needle)
		st.treeVersion, st.treeFilter, st.treeBuilt = a.branchesVersion, needle, true
	}
	tree := st.tree
	if len(tree.roots) == 0 {
		return false
	}
	// The first list, and each new filter, open all folders so no match hides
	// behind a collapsed ancestor. Opening only when the filter changes lets
	// the user collapse folders while still filtering.
	if !st.seeded || (needle != "" && st.appliedFilter != needle) {
		for key, node := range tree.nodes {
			if node.leaf == nil {
				st.outline.Open.Add(key)
			}
		}
		st.seeded = true
	}
	st.appliedFilter = needle
	st.outline.List.Selected = &st.row
	outline := ui.Outline(c, &st.outline, tree.roots, func(key string) []string {
		return tree.children[key]
	}, func(key string) {
		a.branchTreeRow(c, tree, &st.outline, key)
	}).FillWidth()
	if maxHeight > 0 {
		outline.MaxHeight(maxHeight)
	} else {
		outline.Grow(1)
	}
	// A list row owns its press: a click chooses a row, and activation comes
	// as Submitted — Enter while the tree has the focus, or a double click.
	// A row's content does not need, and does not get, click handlers of its
	// own; only controls drawn inside the row, such as the delete button,
	// take their own clicks.
	if outline.Submitted() {
		a.activateBranchRow(st)
	}
	return true
}

// branchTreeRow draws one row: a section, a folder or a branch.
func (a *App) branchTreeRow(c *ui.Context, tree branchTree, st *ui.OutlineState[string], key string) {
	t := c.Theme()
	node := tree.nodes[key]
	row := ui.Row(c).Grow(1).Height(22).Gap(6).Clip().Cursor(ui.CursorPointer)
	hovered := row.Hovered()

	if node.leaf == nil {
		row.Children(func() {
			if node.section {
				ui.Text(c, node.label).FontSize(9).Bold().TextColor(t.TextMuted)
			} else {
				ui.Text(c, node.label).Font(mono()).FontSize(11).SingleLine()
			}
			if count := tree.counts[key]; node.section || count > 1 {
				ui.Text(c, strconv.Itoa(count)).FontSize(9).TextColor(t.TextMuted)
			}
		})
		return
	}

	b := node.leaf
	selected := st.List.Selected != nil && *st.List.Selected >= 0 &&
		*st.List.Selected < st.Rows() && st.Item(*st.List.Selected) == key
	row.Children(func() {
		icon(c, iconBranch).FontSize(11).
			TextColor(boolColor(b.Current, t.Text, t.TextMuted))
		ui.Text(c, node.label).Font(mono()).FontSize(11).Grow(1).SingleLine()
		if b.Current {
			ui.Row(c).Shrink(0).Gap(2).Children(func() {
				icon(c, iconCheck).FontSize(9).TextColor(t.TextMuted)
				ui.Text(c, "current").FontSize(9).TextColor(t.TextMuted)
			})
			if s := syncLabel(a.status.Branch); s != "" {
				ui.Text(c, s).Font(mono()).FontSize(9).TextColor(t.TextMuted)
			}
		} else {
			ui.Text(c, relativeTime(b.Time)).FontSize(9).TextColor(t.TextMuted)
		}
		// Delete stays local and off the current branch; a remote ref is not
		// ours to remove without a push. The selected row shows it too, so a
		// keyboard user who chose the row can reach the action.
		if !b.Remote && !b.Current && (hovered || selected) {
			iconButton(c, iconTrash, "Delete branch", "Delete branch "+b.Name, a.busy(), func() {
				a.deleteBranch(b.Name)
			})
		}
	})
	row.Tooltip(branchTooltip(*b))
	if !b.Current {
		row.ContextMenu(func(m *ui.Menu) { a.branchLeafMenu(m, *b) })
	}
}

func branchTooltip(b gitx.BranchEntry) string {
	if b.Subject == "" {
		return b.Name
	}
	return b.Name + "\n" + b.Subject
}

// branchLeafMenu is the row's right-click menu. The row itself checks out on a
// click; the menu names the same action so it is discoverable.
func (a *App) branchLeafMenu(m *ui.Menu, b gitx.BranchEntry) {
	checkout := "Checkout"
	if b.Remote {
		checkout = "Checkout as Local Branch"
	}
	if m.Item(checkout).Disabled(a.busy()).Chosen() {
		a.checkoutEntry(b)
	}
	if !b.Remote {
		if m.Item("Delete Branch…").Disabled(a.busy()).Chosen() {
			a.deleteBranch(b.Name)
		}
	}
}

// toggleBranchOpen opens or closes one section or folder.
func toggleBranchOpen(st *ui.OutlineState[string], key string) {
	if st.Open.Has(key) {
		st.Open.Remove(key)
	} else {
		st.Open.Add(key)
	}
}

// moveBranchPopupRow moves the popover's choice to the next or previous branch
// leaf while the search field keeps the focus, so typing and choosing never
// fight over the keyboard.
func (a *App) moveBranchPopupRow(delta int) {
	st := &a.branchPopupTree
	if row := nextBranchLeaf(st, st.row, delta); row >= 0 {
		st.row = row
	}
}

// nextBranchLeaf returns the first leaf row after from in direction delta, or
// -1 at the end of the tree.
func nextBranchLeaf(st *branchTreeState, from, delta int) int {
	rows := st.outline.Rows()
	// With no choice yet, start from the edge so the first step lands on the
	// first (or last) branch.
	if from < 0 || from >= rows {
		if delta > 0 {
			from = -1
		} else {
			from = rows
		}
	}
	for i := from + delta; i >= 0 && i < rows; i += delta {
		if node, ok := st.tree.nodes[st.outline.Item(i)]; ok && node.leaf != nil {
			return i
		}
	}
	return -1
}

// activateBranchRow acts on the row a tree chose: a folder opens or closes,
// a branch is checked out. The popover's search field calls it on Enter; the
// outline reports Submitted for Enter on a focused row and for a double click.
func (a *App) activateBranchRow(st *branchTreeState) {
	row := st.row
	if row < 0 || row >= st.outline.Rows() {
		return
	}
	key := st.outline.Item(row)
	node, ok := st.tree.nodes[key]
	if !ok {
		return
	}
	if node.leaf == nil {
		toggleBranchOpen(&st.outline, key)
		return
	}
	a.checkoutEntry(*node.leaf)
}

// activateBranchPopupRow is Enter in the search field: with no choice yet it
// takes the first matching branch, so "type a name, press Enter" switches.
func (a *App) activateBranchPopupRow() {
	st := &a.branchPopupTree
	if st.row < 0 || st.row >= st.outline.Rows() {
		if row := nextBranchLeaf(st, -1, 1); row >= 0 {
			st.row = row
		}
	}
	a.activateBranchRow(st)
}

// checkoutEntry switches to a local branch, or turns a remote-tracking one into
// its local counterpart.
func (a *App) checkoutEntry(b gitx.BranchEntry) {
	if a.busy() {
		return
	}
	name, remote := a.checkoutTarget(b)
	// The current branch, and a remote row whose local branch is the current
	// one, are no-op switches: a write slot and a refresh are not worth it.
	if b.Current || (!remote && name == a.status.Branch.Head) {
		// Close the popover that asked.
		a.branchPopupOpen = false
		return
	}
	if remote {
		a.checkoutRemoteBranch(name)
		return
	}
	a.checkoutBranch(name)
}

// checkoutTarget decides which operation a branch row needs. A remote branch
// with an existing local branch switches to that one: it is what the user has
// been working on, and tracking it again would collide.
func (a *App) checkoutTarget(b gitx.BranchEntry) (name string, remote bool) {
	if !b.Remote {
		return b.Name, false
	}
	local := b.LocalName()
	if a.hasLocalBranch(local) {
		return local, false
	}
	return b.Name, true
}

func (a *App) hasLocalBranch(name string) bool {
	for _, b := range a.branches {
		if !b.Remote && b.Name == name {
			return true
		}
	}
	return false
}
