// Package shell is Twill's native user interface: it holds the window state,
// runs repository calls away from the main thread, and builds the view MyGo
// draws. It never starts git itself; internal/service owns that.
package shell

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
	"twill/internal/service"
)

// historyPage is one page of the commit list; "load older" appends the next.
const historyPage = 100

// syncTimeout bounds one network operation. A credential helper or a network
// black hole must not disable writes forever: git is killed when it expires,
// and the user can retry.
const syncTimeout = 5 * time.Minute

// View is the left sidebar's tab.
type View int

const (
	ViewChanges View = iota
	ViewHistory
	ViewBranches
	ViewStashes
)

// App holds the window state. Every field is read and written on the main
// thread; background results arrive through Window.Update.
type App struct {
	svc     *service.RepoService
	actions *service.MenuActions
	win     *mygo.Window

	// sidebar is the file list's width when the panes sit side by side;
	// sidebarHeight is its height when they are stacked.
	sidebar       float32
	sidebarHeight float32
	panes         paneLayout

	// changesPercent is the changes list's share of the sidebar above the
	// commit box, 0 to 100; dragging their divider sets it.
	changesPercent float32

	// commitPanelHeight is the height of the history panel's commit details
	// and file list above the diff; the divider sets it.
	commitPanelHeight float32

	status        gitx.Status
	loadingStatus bool
	selection     *gitx.Change
	diff          gitx.DiffResult
	worktreePatch patchFile
	hasDiff       bool
	loadingDiff   bool
	writing       bool
	errorText     string
	// statusVersion changes whenever a new status is adopted, so the derived
	// lists (the changes tree) know when to rebuild.
	statusVersion uint64

	// syncing is the remote operation in flight ("fetch", "pull" or "push"),
	// "" when none. One at a time: they touch the same refs, and pull and push
	// also rewrite the worktree and the remote. A sync also blocks worktree
	// writes, so a commit cannot capture a half-pulled worktree.
	syncing bool
	syncOp  string
	// syncCancel stops the git process behind the slot, so opening another
	// repository does not leave it racing the next operation.
	syncCancel context.CancelFunc

	view           View
	commits        []gitx.Commit
	graph          []graphRow
	graphLanes     int
	commitsLoaded  bool
	hasMoreCommits bool
	loadingCommits bool
	historyErr     string
	// commitsVersion changes whenever the commit page is replaced, so the
	// filtered history rows are rebuilt only then, or when the filter changes.
	commitsVersion uint64

	// selectedCommit is the commit the right panel shows: its details, its
	// files and the diff of the file picked among them.
	selectedCommit    *gitx.Commit
	commitBody        string
	loadingBody       bool
	commitFiles       []gitx.FileEntry
	loadingFiles      bool
	commitFile        *gitx.FileEntry
	commitDiff        gitx.DiffResult
	commitPatch       patchFile
	hasCommitDiff     bool
	loadingCommitDiff bool
	commitErr         string

	// stashes is the stash list; it is read on every refresh.
	stashes        []gitx.StashEntry
	stashesLoaded  bool
	loadingStashes bool
	stashErr       string

	branches        []gitx.BranchEntry
	branchesLoaded  bool
	loadingBranches bool
	branchErr       string
	// branchesVersion changes whenever a branch list is adopted, so both trees
	// rebuild only then, or when their filter changes.
	branchesVersion uint64

	// The branch popover is the quick switcher on the toolbar chip; the
	// Branches tab draws the same tree from its own state, so each keeps its
	// filter, open folders and chosen row.
	branchPopupOpen   bool
	focusBranchFilter bool
	branchPopupTree   branchTreeState
	branchTab         branchTreeState
	// revealedBranch is the checked-out branch whose folders the trees last
	// opened; a change reveals the new branch's path without reopening folders
	// the user closed.
	revealedBranch string

	message      string
	amend        bool
	changeFilter string
	// changesTree groups the changes rows under their directories; the more
	// menu switches to the flat list. closedDirs holds the folders and the
	// untracked section the user closed, by key; the untracked section starts
	// closed in a new app.
	changesTree bool
	closedDirs  map[string]bool
	// changeFilterSeen is the filter the folders were last opened for: a new
	// filter opens every folder once, so a match is never hidden.
	changeFilterSeen string
	// changeCache holds the sections and directory trees the Changes tab draws.
	// They are rebuilt only when the status, the filter or the layout changes,
	// so drawing a frame walks them instead of rebuilding them.
	changeCache changeCache
	// commitFilter narrows the loaded history page by message, author, hash
	// or ref name.
	commitFilter string
	// commitAuthor keeps one author's commits; empty means everyone.
	commitAuthor string
	// historyCache is the filtered history rows, rebuilt only when the commit
	// page, the author or the needle changes; Up and Down reuse it.
	historyCache historyRowsCache
	creating     branchCreateArea
	newName      string
	focusNew     bool

	// changeOrder is the file rows the Changes tab drew, top to bottom; Up and
	// Down walk it. revealRow asks the chosen row to scroll into view when it
	// is next drawn; focusFilter asks for the filter field of focusFilterOf.
	changeOrder   []gitx.Change
	revealRow     bool
	focusFilter   bool
	focusFilterOf View
	confirm       *confirmation
	promptValue   string
	promptOption  bool
	focusPrompt   bool
	confirmOpen   bool
	// push is the push dialog; nil while closed.
	push         *pushDialog
	pushOpen     bool
	toast        string
	toastIsError bool
	toastDur     time.Duration
	toastTimer   *time.Timer

	historyList ui.ListState
	// commitFilesList scrolls the selected commit's file list.
	commitFilesList ui.ListState
	diffList        ui.ListState

	// The diff pane's rows are derived, not read directly: a layout choice,
	// an expanded gap and the shown patch all change them. diffDirty makes
	// the next build recompute; diffKey names the patch they belong to.
	diffLayout diffLayout
	diffRows   []diffRow
	diffKey    diffSourceID
	diffGaps   map[int]*diffGap
	diffDirty  bool
	gapText    gapRead

	epochs epochs
}

// epochs drop responses for state the user already moved past, as the web
// store's request counters did.
type epochs struct {
	status     atomic.Uint64
	diff       atomic.Uint64
	commits    atomic.Uint64
	files      atomic.Uint64
	body       atomic.Uint64
	commitDiff atomic.Uint64
	branches   atomic.Uint64
	sync       atomic.Uint64
	push       atomic.Uint64
	stashes    atomic.Uint64
}

type confirmation struct {
	title  string
	detail string
	label  string
	run    func()
	// input is the placeholder of a text field; set, the dialog is a prompt
	// and runInput gets the trimmed text (and the checkbox) instead of run
	// being called. option labels an optional checkbox, and optional lets the
	// text stay empty.
	input    string
	option   string
	optional bool
	runInput func(text string, option bool)
}

// promptSpec describes a prompt dialog.
type promptSpec struct {
	title, detail, label, placeholder string
	// option is the label of a checkbox, on by default; "" for none.
	option string
	// optional lets the text stay empty.
	optional bool
}

// New returns the shell for one window.
func New(svc *service.RepoService, actions *service.MenuActions) *App {
	a := &App{
		svc:               svc,
		actions:           actions,
		sidebar:           380,
		sidebarHeight:     380,
		changesPercent:    78,
		commitPanelHeight: 200,
		changesTree:       true,
		closedDirs:        map[string]bool{"s:untracked": true},
		toastDur:          4 * time.Second,
		branchTab:         newBranchTreeState(),
		branchPopupTree:   newBranchTreeState(),
	}
	// The watcher and menu actions run outside the view; marshal their
	// notifications back to the main thread before touching state.
	svc.OnChange(func(root, reason string) {
		// apply marshals to the main thread once the window exists; before
		// that there is only one thread, so running here is safe.
		a.apply(func() {
			if reason == "open" {
				// A different repository invalidates everything derived.
				a.resetRepoState(root)
			}
			a.refresh(reason)
		})
	})
	return a
}

// Attach records the window the shell updates and draws. It is called after
// NewWindow, before the window runs.
func (a *App) Attach(w *mygo.Window) { a.win = w }

// apply runs fn on the main thread, then redraws. Before the window exists
// there is only one thread, so it runs at once.
func (a *App) apply(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// refresh reloads the status and the lists derived from it. Watch events
// skip the commit page (an edit fires one per batch) but keep the branch
// list, which is one for-each-ref.
func (a *App) refresh(reason string) {
	a.reloadStatus()
	if reason != "watch" && a.commitsLoaded {
		a.loadCommits(false)
	}
	if a.branchesLoaded {
		a.loadBranches()
	}
	a.loadStashes()
}

// reloadStatus reads the working tree and adopts it.
func (a *App) reloadStatus() {
	epoch := a.epochs.status.Add(1)
	a.loadingStatus = true
	root := a.status.Root
	go func() {
		st, err := a.svc.Status(context.Background())
		a.apply(func() {
			if epoch != a.epochs.status.Load() {
				return // a newer load won
			}
			a.loadingStatus = false
			if err != nil {
				a.showError(err)
				return
			}
			if root != "" && st.Root != root {
				// Another repository: its lists are not ours.
				a.resetRepoState(st.Root)
			}
			a.adoptStatus(st)
		})
	}()
}

// adoptStatus installs a status and keeps the selected row when it still
// exists, following the same path into its new bucket.
func (a *App) adoptStatus(st gitx.Status) {
	a.status = st
	a.statusVersion++
	// A checkout or a branch created outside Twill moves HEAD; the trees may
	// otherwise keep the new branch closed in a folder.
	a.revealCurrentBranch()
	if st.Root == "" {
		a.resetDiff()
		return
	}
	var next *gitx.Change
	if cur := a.selection; cur != nil {
		if same := findChange(st, *cur); same != nil {
			next = same
		} else {
			next = findChangeByPath(st, cur.Path)
		}
	}
	if next == nil {
		next = firstSelection(st)
	}
	if next == nil {
		a.resetDiff()
		return
	}
	a.loadDiff(*next)
}

// loadDiff fetches the patch of one row. The row is kept even when the load
// fails, so the panel can offer a retry through the list.
func (a *App) loadDiff(ch gitx.Change) {
	epoch := a.epochs.diff.Add(1)
	root := a.status.Root
	a.selection = &ch
	a.loadingDiff = true
	a.errorText = ""
	// The patch is about to be replaced; its gap state and reads no longer
	// describe what the pane shows.
	a.resetDiffGaps()
	go func() {
		res, err := a.svc.Diff(context.Background(), gitx.DiffRequest{
			Path:      ch.Path,
			OrigPath:  ch.OrigPath,
			Staged:    ch.Staged,
			Untracked: ch.Untracked,
		})
		// Parse away from the main thread: a large patch would drop frames.
		patch := parsePatch(res.Patch)
		a.apply(func() {
			if epoch != a.epochs.diff.Load() || a.status.Root != root {
				return
			}
			a.loadingDiff = false
			if err != nil {
				a.errorText = err.Error()
				a.hasDiff = false
				return
			}
			a.diff = res
			a.worktreePatch = patch
			a.hasDiff = true
		})
	}()
}

// resetRepoState drops everything derived from the repository the user left:
// loaded pages, the selected commit, branch lists and both diff selections.
// Epochs are bumped so an in-flight response from the old repository cannot
// land after the reset, and the loading flags are cleared so the new
// repository's first load is not blocked by a request that will never finish.
// The status is emptied too: rows from the old repository must not stay
// clickable while the new status is loading.
func (a *App) resetRepoState(root string) {
	a.epochs.commits.Add(1)
	a.epochs.branches.Add(1)
	a.epochs.sync.Add(1)
	a.loadingCommits = false
	a.loadingBranches = false
	if a.syncCancel != nil {
		a.syncCancel()
		a.syncCancel = nil
	}
	a.syncing = false
	a.syncOp = ""
	a.status = gitx.NewStatus(root)
	// A new repository invalidates everything derived from the old status.
	a.statusVersion++
	a.changeCache = changeCache{}
	a.resetDiff()
	a.resetCommitSelection()
	a.commits = nil
	a.commitsVersion++
	a.historyCache = historyRowsCache{}
	// Authors belong to the repository that is leaving.
	a.commitAuthor = ""
	a.graph = nil
	a.commitsLoaded = false
	a.hasMoreCommits = false
	a.historyErr = ""
	a.epochs.stashes.Add(1)
	a.loadingStashes = false
	a.stashes = nil
	a.stashesLoaded = false
	a.stashErr = ""
	a.branches = nil
	a.branchesVersion++
	a.branchesLoaded = false
	a.branchErr = ""
	// The keyboard's place in the old list, a pending focus request and a row
	// waiting to be revealed all describe rows that no longer exist.
	a.changeOrder = a.changeOrder[:0]
	a.revealRow = false
	a.focusFilter = false
	a.focusFilterOf = ViewChanges
	a.branchPopupOpen = false
	a.branchPopupTree.filter = ""
	a.branchPopupTree.reset()
	a.branchTab.reset()
	a.revealedBranch = ""
	a.cancelNewBranch()
	a.closePush()
}

func (a *App) resetDiff() {
	a.epochs.diff.Add(1)
	a.selection = nil
	a.diff = gitx.DiffResult{}
	a.worktreePatch = patchFile{}
	a.hasDiff = false
	a.loadingDiff = false
	a.errorText = ""
	// The patch is gone, so neither its expansions nor a read still in flight
	// may survive.
	a.resetDiffGaps()
}

// write serializes one index or worktree write, then refreshes. A failure is
// a toast: it should not replace the diff with an error page. A network
// operation in flight blocks writes: a pull would otherwise race a commit on
// the same index and worktree.
func (a *App) write(run func() error) {
	if a.busy() {
		return
	}
	a.writing = true
	go func() {
		err := run()
		a.apply(func() {
			a.writing = false
			if err != nil {
				a.showError(err)
				return
			}
			a.refresh("")
		})
	}()
}

func (a *App) stage(ch gitx.Change) {
	a.write(func() error { return a.svc.Stage(context.Background(), ch.Path) })
}

func (a *App) unstage(ch gitx.Change) {
	a.write(func() error {
		return a.svc.Unstage(context.Background(), ch.Path, ch.OrigPath)
	})
}

// stageFolder and unstageFolder act on exactly the files a folder row lists,
// in one git call, so a folder of new files is staged without also staging
// files the section does not show.
func (a *App) stageFolder(files []gitx.Change) {
	a.write(func() error {
		return a.svc.StagePaths(context.Background(), changePaths(files))
	})
}

func (a *App) unstageFolder(files []gitx.Change) {
	a.write(func() error {
		return a.svc.UnstagePaths(context.Background(), changePaths(files))
	})
}

// changePaths lists the paths git needs for rows, both sides of a rename.
func changePaths(files []gitx.Change) []string {
	out := make([]string, 0, len(files))
	for _, ch := range files {
		out = append(out, gitx.Paths(ch.Path, ch.OrigPath)...)
	}
	return out
}

func (a *App) discard(ch gitx.Change) {
	a.confirmAction("Discard changes to "+ch.Path+"?",
		"This cannot be undone. The file returns to the index version.",
		"Discard", func() {
			a.write(func() error { return a.svc.Discard(context.Background(), ch.Path) })
		})
}

// applyHunk stages, unstages or discards one hunk of the selected file.
func (a *App) applyHunk(op gitx.ApplyOp, hunk int) {
	ch := a.selection
	if ch == nil {
		return
	}
	// A status or diff load in flight means the hunk indices may belong to
	// the previous patch; applying one now could touch the wrong lines.
	if a.writing || a.loadingStatus || a.loadingDiff {
		return
	}
	a.write(func() error {
		return a.svc.ApplyHunks(context.Background(), gitx.ApplyRequest{
			Path:      ch.Path,
			OrigPath:  ch.OrigPath,
			Op:        op,
			Hunks:     []int{hunk},
			Untracked: ch.Untracked,
		})
	})
}

func (a *App) commit() { a.commitThen(false) }

// commitThen commits, and with push opens the push dialog once the commit
// exists, so the push is previewed rather than fired blindly.
func (a *App) commitThen(push bool) {
	if !a.canCommit() || (push && a.amend) {
		return
	}
	msg, amend := a.message, a.amend
	a.writing = true
	go func() {
		_, err := a.svc.Commit(context.Background(), gitx.CommitRequest{
			Message: msg,
			Amend:   amend,
		})
		a.apply(func() {
			a.writing = false
			if err != nil {
				a.showError(err)
				return
			}
			a.message = ""
			a.amend = false
			a.refresh("")
			if push {
				a.openPush()
			}
		})
	}()
}

// fetchRemote, pullRemote and pushRemote share one slot: a second operation
// could race the first on the same refs. The view is refreshed even when the
// operation fails: a pull fetches before it fast-forwards, so its fetch half
// can have moved remote-tracking refs before the merge half refused.
func (a *App) fetchRemote() { a.syncRemote("fetch", a.svc.Fetch) }
func (a *App) pullRemote()  { a.syncRemote("pull", a.svc.Pull) }
func (a *App) pushRemote()  { a.syncRemote("push", a.svc.Push) }

func (a *App) syncRemote(op string, run func(context.Context) error) {
	if a.busy() || a.status.Root == "" {
		return
	}
	// The Git menu bypasses the toolbar's disabled buttons; pull and push
	// still need a branch with a commit to work on.
	if op != "fetch" && !a.hasSyncBranch() {
		return
	}
	root := a.status.Root
	epoch := a.epochs.sync.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	a.syncing = true
	a.syncOp = op
	a.syncCancel = cancel
	go func() {
		err := run(ctx)
		a.apply(func() {
			if epoch != a.epochs.sync.Load() || a.status.Root != root {
				// A repository switch cancelled the operation and cleared the
				// slot; the new repository's own loads show its state.
				return
			}
			if err != nil && ctx.Err() != nil {
				// git was killed by the timeout; its "signal: killed" says
				// nothing to the user.
				err = fmt.Errorf("%s stopped after %s; check the network or your credentials",
					op, syncTimeout)
			}
			cancel()
			a.syncing = false
			a.syncOp = ""
			a.syncCancel = nil
			a.refresh("")
			if err != nil {
				log.Printf("shell: %s failed: %v", op, err)
				a.showError(err)
			}
		})
	}()
}

// busy reports whether a worktree write or a network operation runs. Both
// change the index, the worktree or the refs, so they share one slot.
func (a *App) busy() bool { return a.writing || a.syncing }

// hasSyncBranch reports whether pull and push have a branch to work on: a
// detached HEAD and an unborn branch do not.
func (a *App) hasSyncBranch() bool {
	b := a.status.Branch
	return b.Head != "" && !b.Detached && b.OID != ""
}

// The Git menu calls the toolbar's operations. apply posts to the main thread,
// where the sync slot lives.
func (a *App) FetchFromMenu() { a.apply(a.fetchRemote) }
func (a *App) PullFromMenu()  { a.apply(a.pullRemote) }
func (a *App) PushFromMenu() {
	a.apply(func() {
		if a.hasSyncBranch() {
			a.openPush()
		}
	})
}

// loadCommits reads one page; more appends the next page. A page always
// replaces the list from the top so a newer commit cannot shift an older
// page the user already scrolled through.
func (a *App) loadCommits(more bool) {
	root := a.status.Root
	if root == "" {
		return
	}
	if more && a.loadingCommits {
		return
	}
	epoch := a.epochs.commits.Add(1)
	skip := 0
	if more {
		skip = len(a.commits)
	}
	a.loadingCommits = true
	a.historyErr = ""
	go func() {
		page, err := a.svc.History(context.Background(), gitx.LogRequest{
			Limit: historyPage,
			Skip:  skip,
		})
		a.apply(func() {
			if epoch != a.epochs.commits.Load() || a.status.Root != root {
				return
			}
			a.loadingCommits = false
			if err != nil {
				a.commitsLoaded = true
				// A failed append keeps the page the user already has.
				if more {
					a.showError(err)
				} else {
					a.historyErr = err.Error()
				}
				return
			}
			if more {
				a.commits = append(a.commits, page...)
			} else {
				a.commits = page
			}
			a.commitsLoaded = true
			a.hasMoreCommits = len(page) == historyPage
			a.commitsVersion++
			a.graph, a.graphLanes = layoutGraph(a.commits)
		})
	}()
}

func (a *App) ensureCommits() {
	if a.status.Root != "" && !a.commitsLoaded && !a.loadingCommits {
		a.loadCommits(false)
	}
}

// selectCommit shows a commit in the right panel: its message details, its
// files and the diff of the first one. Selecting the commit already shown
// changes nothing.
func (a *App) selectCommit(c gitx.Commit) {
	if a.selectedCommit != nil && a.selectedCommit.OID == c.OID {
		return
	}
	a.resetCommitSelection()
	oid := c.OID
	a.selectedCommit = &c
	a.loadCommitBody(oid)

	epoch := a.epochs.files.Add(1)
	a.loadingFiles = true
	go func() {
		files, err := a.svc.CommitFiles(context.Background(), oid)
		a.apply(func() {
			if epoch != a.epochs.files.Load() || !a.showsCommit(oid) {
				return
			}
			a.loadingFiles = false
			if err != nil {
				a.commitErr = err.Error()
				return
			}
			a.commitFiles = files
			// Show the first file right away: a selected commit with an empty
			// diff area looks like it is still loading.
			if len(files) > 0 {
				a.selectCommitFile(files[0])
			}
		})
	}()
}

// loadCommitBody reads the message below the subject for the detail panel.
// A failure only drops the body: the subject and the files still show.
func (a *App) loadCommitBody(oid string) {
	epoch := a.epochs.body.Add(1)
	a.loadingBody = true
	go func() {
		body, err := a.svc.CommitBody(context.Background(), oid)
		a.apply(func() {
			if epoch != a.epochs.body.Load() || !a.showsCommit(oid) {
				return
			}
			a.loadingBody = false
			if err != nil {
				log.Printf("shell: read commit %s body: %v", oid, err)
				return
			}
			a.commitBody = body
		})
	}()
}

// showsCommit reports whether oid is still the selected commit, which is what
// makes a late file or body response worth installing.
func (a *App) showsCommit(oid string) bool {
	return a.selectedCommit != nil && a.selectedCommit.OID == oid
}

func (a *App) selectCommitFile(f gitx.FileEntry) {
	commit := a.selectedCommit
	if commit == nil {
		return
	}
	epoch := a.epochs.commitDiff.Add(1)
	oid := commit.OID
	a.commitFile = &f
	a.loadingCommitDiff = true
	a.resetDiffGaps()
	a.commitDiff = gitx.DiffResult{}
	a.commitPatch = patchFile{}
	a.hasCommitDiff = false
	a.commitErr = ""
	go func() {
		res, err := a.svc.CommitDiff(context.Background(), gitx.CommitDiffRequest{
			OID:      oid,
			Path:     f.Path,
			OrigPath: f.OrigPath,
		})
		patch := parsePatch(res.Patch)
		a.apply(func() {
			if epoch != a.epochs.commitDiff.Load() || !a.showsCommit(oid) {
				return
			}
			a.loadingCommitDiff = false
			if err != nil {
				a.commitErr = err.Error()
				return
			}
			a.commitDiff = res
			a.commitPatch = patch
			a.hasCommitDiff = true
		})
	}()
}

func (a *App) resetCommitSelection() {
	a.epochs.files.Add(1)
	a.epochs.body.Add(1)
	a.epochs.commitDiff.Add(1)
	a.selectedCommit = nil
	a.commitBody = ""
	a.loadingBody = false
	a.commitFiles = nil
	a.loadingFiles = false
	a.commitFile = nil
	a.commitDiff = gitx.DiffResult{}
	a.commitPatch = patchFile{}
	a.hasCommitDiff = false
	a.loadingCommitDiff = false
	a.commitErr = ""
	a.resetDiffGaps()
}

func (a *App) loadBranches() {
	root := a.status.Root
	if root == "" {
		return
	}
	epoch := a.epochs.branches.Add(1)
	a.loadingBranches = true
	a.branchErr = ""
	go func() {
		list, err := a.svc.Branches(context.Background())
		a.apply(func() {
			if epoch != a.epochs.branches.Load() || a.status.Root != root {
				return
			}
			a.loadingBranches = false
			if err != nil {
				// Keep the last successful list: a failed refresh should not
				// replace it with an error page. The view offers a retry.
				a.branchesLoaded = true
				a.branchErr = err.Error()
				log.Printf("shell: load branches for %q failed: %v", root, err)
				return
			}
			a.branches = list
			a.branchesLoaded = true
			a.branchesVersion++
		})
	}()
}

func (a *App) ensureBranches() {
	if a.status.Root != "" && !a.branchesLoaded && !a.loadingBranches {
		a.loadBranches()
	}
}

// refreshBranches reloads a list that is already loaded, so a fetch or branch
// change made outside Twill shows up when a branch view opens: the file
// watcher ignores .git and would otherwise never report it.
func (a *App) refreshBranches() {
	if a.branchesLoaded && !a.loadingBranches {
		a.loadBranches()
		return
	}
	a.ensureBranches()
}

// checkoutBranch switches to a local branch.
func (a *App) checkoutBranch(name string) {
	a.checkout(func() error { return a.svc.CheckoutBranch(context.Background(), name) })
}

// checkoutRemoteBranch creates and switches to the local counterpart of a
// remote-tracking branch.
func (a *App) checkoutRemoteBranch(name string) {
	a.checkout(func() error { return a.svc.CheckoutRemoteBranch(context.Background(), name) })
}

// checkout runs one branch switch and, on success, drops the state that
// belonged to the branch the user left and closes the branch popover: the tree
// it shows is about to be reloaded for the new branch.
//
// A failure reloads the branch list: the row came from a list that may predate
// a branch created in a terminal, and the next attempt should see it.
func (a *App) checkout(run func() error) {
	root := a.status.Root
	a.write(func() error {
		if err := run(); err != nil {
			a.apply(func() {
				if a.status.Root == root {
					a.loadBranches()
				}
			})
			return err
		}
		a.apply(func() {
			if a.status.Root != root {
				return
			}
			a.resetDiff()
			a.resetCommitSelection()
			a.branchPopupOpen = false
			a.cancelNewBranch()
		})
		return nil
	})
}

func (a *App) createBranch(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	a.write(func() error { return a.svc.CreateBranch(context.Background(), name) })
}

func (a *App) deleteBranch(name string) {
	a.confirmAction("Delete branch "+name+"?",
		"Only the branch pointer is removed. Git refuses a branch whose commits are not merged elsewhere.",
		"Delete", func() {
			a.write(func() error { return a.svc.DeleteBranch(context.Background(), name) })
		})
}

// canCommit mirrors the commit panel's disabled rules.
func (a *App) canCommit() bool {
	return !a.busy() && strings.TrimSpace(a.message) != "" &&
		(a.amend || len(a.status.Staged) > 0)
}

// openPicker asks for a repository and opens it; the service notifies the
// shell when the open completes.
func (a *App) openPicker() {
	if a.actions != nil {
		a.actions.OpenFromMenu()
	}
}

// showError reports a failed action without hiding the current view.
func (a *App) showError(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	// git's stderr often ends with a newline; a toast should not.
	msg = strings.TrimSpace(msg)
	a.toast = msg
	a.toastIsError = true
	if a.win != nil {
		// Clear the toast after a few seconds unless a newer one replaced it.
		// One timer per App: a burst of errors must not leave timers behind.
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
		text := msg
		a.toastTimer = time.AfterFunc(a.toastDur, func() {
			a.apply(func() {
				if a.toast == text {
					a.toast = ""
					a.toastIsError = false
					a.toastTimer = nil
				}
			})
		})
	}
}

func (a *App) confirmAction(title, detail, label string, run func()) {
	a.confirm = &confirmation{title: title, detail: detail, label: label, run: run}
	a.confirmOpen = true
}

// promptAction asks for one line of text and, optionally, one checkbox. Enter
// or the button runs it with the trimmed text; an empty field disables both
// unless the text is optional.
func (a *App) promptAction(spec promptSpec, run func(text string, option bool)) {
	a.confirm = &confirmation{title: spec.title, detail: spec.detail, label: spec.label,
		input: spec.placeholder, option: spec.option, optional: spec.optional, runInput: run}
	a.promptValue = ""
	a.promptOption = spec.option != ""
	a.focusPrompt = true
	a.confirmOpen = true
}

// helpers

func sameChange(a, b gitx.Change) bool {
	return a.Path == b.Path && a.Staged == b.Staged &&
		a.Untracked == b.Untracked && a.Conflicted == b.Conflicted
}

func allChanges(st gitx.Status) []gitx.Change {
	out := make([]gitx.Change, 0, len(st.Conflicted)+len(st.Staged)+len(st.Unstaged)+len(st.Untracked))
	out = append(out, st.Conflicted...)
	out = append(out, st.Staged...)
	out = append(out, st.Unstaged...)
	out = append(out, st.Untracked...)
	return out
}

func findChange(st gitx.Status, want gitx.Change) *gitx.Change {
	all := allChanges(st)
	for i := range all {
		if sameChange(all[i], want) {
			return &all[i]
		}
	}
	return nil
}

func findChangeByPath(st gitx.Status, path string) *gitx.Change {
	all := allChanges(st)
	for i := range all {
		if all[i].Path == path {
			return &all[i]
		}
	}
	return nil
}

func firstSelection(st gitx.Status) *gitx.Change {
	switch {
	case len(st.Conflicted) > 0:
		return &st.Conflicted[0]
	case len(st.Unstaged) > 0:
		return &st.Unstaged[0]
	case len(st.Staged) > 0:
		return &st.Staged[0]
	case len(st.Untracked) > 0:
		return &st.Untracked[0]
	}
	return nil
}

func repoName(root string) string {
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

func relativeTime(unix int64) string {
	delta := time.Until(time.Unix(unix, 0))
	abs := delta
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < time.Minute:
		return "just now"
	case abs < time.Hour:
		return fmt.Sprintf("%dm ago", int(abs.Minutes()))
	case abs < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(abs.Hours()))
	case abs < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(abs.Hours()/24))
	case abs < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(abs.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(abs.Hours()/(24*365)))
	}
}
