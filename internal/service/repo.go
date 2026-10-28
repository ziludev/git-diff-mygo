// Package service exposes the repository actions the web frontend calls.
package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"twill/internal/gitx"
)

// RepoChange is sent to the page when the open repository may have changed.
type RepoChange struct {
	Root   string `json:"root"`
	Reason string `json:"reason"`
}

// RepoChanged fires after a native open and an explicit refresh, and when the
// watcher sees a change the app did not make. Worktree writes deliberately do
// not emit it: their RPC result drives the one refresh the page performs.
var RepoChanged = mygo.NewEvent[RepoChange]("repo-changed")

// RepoService is bound to the frontend as `RepoService`.
type RepoService struct {
	mu       sync.Mutex
	root     string
	win      *mygo.Window
	lastOp   time.Time
	onChange func(root, reason string)

	// pickerOpen is held while the repository picker is up. AppKit refuses a
	// second sheet on the same window, and the second caller would then wait
	// for a dialog that never appears.
	pickerOpen atomic.Bool

	watchMu      sync.Mutex
	watchCancel  context.CancelFunc
	pendingWatch bool
}

// operationWindow is how long after a write the watcher's echo is ignored.
const operationWindow = 1200 * time.Millisecond

// Attach records the window menus and dialogs attach to. It is a package
// function, not a method, so it is not exposed to the page.
func Attach(s *RepoService, w *mygo.Window) {
	s.mu.Lock()
	s.win = w
	s.mu.Unlock()
}

// OnChange sets the callback for native UI windows: it runs on a background
// goroutine whenever the open repository may have changed, and replaces the
// repo-changed event the web page listens for. nil restores the event.
func (s *RepoService) OnChange(fn func(root, reason string)) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

// Open opens the repository at path, asking the user when path is empty.
func (s *RepoService) Open(ctx context.Context, path string) (gitx.Status, error) {
	return s.open(ctx, path, mygo.CallerWindow(ctx))
}

// MenuActions are actions the native menu and window call. The type is not
// bound, so the page cannot invoke them.
type MenuActions struct{ s *RepoService }

// NewMenuActions returns the native-side actions of s.
func NewMenuActions(s *RepoService) *MenuActions { return &MenuActions{s: s} }

// OpenFromMenu opens the repository picker.
func (m *MenuActions) OpenFromMenu() { m.s.openPath("", false) }

// OpenDropped opens a repository (or a file inside one) dropped on the
// window. The page cannot see absolute paths of dropped files, so the
// window's Go side handles the drop.
func (m *MenuActions) OpenDropped(path string) { m.s.openPath(path, false) }

// RefreshFromMenu asks the page to refresh.
func (m *MenuActions) RefreshFromMenu() {
	if root := m.s.Root(); root != "" {
		m.s.notify(root, "refresh")
	}
}

// openPath opens path in the background, asking the user when path is empty,
// and notifies the page. quiet logs a failure instead of showing a dialog,
// which is what the silent startup restore needs.
func (s *RepoService) openPath(path string, quiet bool) {
	go func() {
		root, err := s.activate(context.Background(), path, s.window())
		log.Printf("service: open %q: root=%q err=%v", path, root, err)
		if err != nil {
			log.Printf("service: open %q failed: %v", path, err)
			if !quiet {
				if w := s.window(); w != nil {
					mygo.Dialog.Error("Open Repository", err.Error())
				}
			}
			return
		}
		if root != "" {
			s.notify(root, "open")
		}
	}()
}

// Root returns the open repository's top level, or "".
func (s *RepoService) Root() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.root
}

// Status returns the current working tree state.
func (s *RepoService) Status(ctx context.Context) (gitx.Status, error) {
	root := s.Root()
	if root == "" {
		return gitx.NewStatus(""), nil
	}
	return gitx.New(root).Status(ctx)
}

// Diff returns one file's patch.
func (s *RepoService) Diff(ctx context.Context, req gitx.DiffRequest) (gitx.DiffResult, error) {
	r, err := s.runner()
	if err != nil {
		return gitx.DiffResult{}, err
	}
	return r.Diff(ctx, req)
}

// FileLines returns one side of a diff's text, for expanding the unchanged
// lines a patch omits. It never writes anything.
func (s *RepoService) FileLines(ctx context.Context, req gitx.FileTextRequest) ([]string, error) {
	r, err := s.runner()
	if err != nil {
		return nil, err
	}
	return r.FileLines(ctx, req)
}

// ApplyHunks stages, unstages, or discards selected hunks of one file.
func (s *RepoService) ApplyHunks(ctx context.Context, req gitx.ApplyRequest) error {
	return s.mutate(ctx, "apply", req.Path, func(r *gitx.Runner) error {
		if err := r.ApplyHunks(ctx, req); err != nil {
			return err
		}
		if len(req.Hunks) == 0 {
			log.Printf("service: applied all hunks to %q (op=%s)", req.Path, req.Op)
		} else {
			log.Printf("service: applied %d hunk(s) to %q (op=%s)", len(req.Hunks), req.Path, req.Op)
		}
		return nil
	})
}

// History returns one page of the commits reachable from HEAD, newest first.
func (s *RepoService) History(ctx context.Context, req gitx.LogRequest) ([]gitx.Commit, error) {
	r, err := s.runner()
	if err != nil {
		return []gitx.Commit{}, err
	}
	return r.Log(ctx, req)
}

// CommitFiles lists the files one commit changed, against its first parent.
func (s *RepoService) CommitFiles(ctx context.Context, oid string) ([]gitx.FileEntry, error) {
	r, err := s.runner()
	if err != nil {
		return []gitx.FileEntry{}, err
	}
	return r.CommitFiles(ctx, oid)
}

// CommitBody returns the body of a commit message, below its subject.
func (s *RepoService) CommitBody(ctx context.Context, oid string) (string, error) {
	r, err := s.runner()
	if err != nil {
		return "", err
	}
	return r.CommitBody(ctx, oid)
}

// CommitDiff returns one file's patch inside a commit.
func (s *RepoService) CommitDiff(ctx context.Context, req gitx.CommitDiffRequest) (gitx.DiffResult, error) {
	r, err := s.runner()
	if err != nil {
		return gitx.DiffResult{}, err
	}
	return r.CommitDiff(ctx, req)
}

// Fetch updates the remote-tracking refs and prunes deleted ones.
func (s *RepoService) Fetch(ctx context.Context) error {
	return s.mutate(ctx, "fetch", "", func(r *gitx.Runner) error { return r.Fetch(ctx) })
}

// Pull fast-forwards the current branch to its upstream.
func (s *RepoService) Pull(ctx context.Context) error {
	return s.mutate(ctx, "pull", "", func(r *gitx.Runner) error { return r.Pull(ctx) })
}

// Push publishes the current branch to its upstream.
func (s *RepoService) Push(ctx context.Context) error {
	return s.mutate(ctx, "push", "", func(r *gitx.Runner) error { return r.Push(ctx) })
}

// Branches lists local branches for the branch switcher.
func (s *RepoService) Branches(ctx context.Context) ([]gitx.BranchEntry, error) {
	r, err := s.runner()
	if err != nil {
		return []gitx.BranchEntry{}, err
	}
	return r.Branches(ctx)
}

// CheckoutBranch switches the work tree to a local branch.
func (s *RepoService) CheckoutBranch(ctx context.Context, name string) error {
	return s.mutate(ctx, "checkout", name, func(r *gitx.Runner) error {
		return r.CheckoutBranch(ctx, name)
	})
}

// CreateBranch creates a branch at HEAD and checks it out.
func (s *RepoService) CreateBranch(ctx context.Context, name string) error {
	return s.mutate(ctx, "create branch", name, func(r *gitx.Runner) error {
		return r.CreateBranch(ctx, name)
	})
}

// DeleteBranch removes a branch whose commits are merged elsewhere.
func (s *RepoService) DeleteBranch(ctx context.Context, name string) error {
	return s.mutate(ctx, "delete branch", name, func(r *gitx.Runner) error {
		return r.DeleteBranch(ctx, name)
	})
}

// Stage adds one path to the index.
func (s *RepoService) Stage(ctx context.Context, path string) error {
	return s.mutate(ctx, "stage", path, func(r *gitx.Runner) error { return r.Stage(ctx, path) })
}

// Unstage removes one path from the index. origPath comes with a staged
// rename or copy and must be reset too, or the old path stays deleted.
func (s *RepoService) Unstage(ctx context.Context, path, origPath string) error {
	return s.mutate(ctx, "unstage", path, func(r *gitx.Runner) error {
		return r.Unstage(ctx, gitx.Paths(path, origPath)...)
	})
}

// Commit creates a commit from the index and returns its object id.
func (s *RepoService) Commit(ctx context.Context, req gitx.CommitRequest) (string, error) {
	r, err := s.runner()
	if err != nil {
		return "", err
	}
	oid, err := r.Commit(ctx, req)
	if err != nil {
		log.Printf("service: commit failed: %v", err)
		return "", err
	}
	// Start the self-write window when the command finishes: the watcher
	// echo follows the write, not its start, and a slow hook must not let
	// its own echo through.
	s.markOperation()
	log.Printf("service: committed %s (amend=%t)", oid, req.Amend)
	return oid, nil
}

// Discard restores one tracked path in the worktree. Untracked files are
// refused; the UI asks the user to delete them explicitly.
func (s *RepoService) Discard(ctx context.Context, path string) error {
	return s.mutate(ctx, "discard", path, func(r *gitx.Runner) error { return r.Discard(ctx, path) })
}

// mutate runs one repository operation and logs its outcome. It is not only
// for worktree writes: fetch, pull and push share the self-write window too, so
// the watcher does not turn their ref changes into a second refresh.
func (s *RepoService) mutate(ctx context.Context, op, path string, run func(*gitx.Runner) error) error {
	r, err := s.runner()
	if err != nil {
		return err
	}
	what := op
	if path != "" {
		what = fmt.Sprintf("%s %q", op, path)
	}
	if err := run(r); err != nil {
		log.Printf("service: %s failed: %v", what, err)
		return err
	}
	// Start the self-write window when the command finishes: the watcher
	// echo follows the write, not its start, and a long operation must not
	// let its own echo through.
	s.markOperation()
	log.Printf("service: %s", what)
	return nil
}

// runner returns a git runner for the open repository.
func (s *RepoService) runner() (*gitx.Runner, error) {
	root := s.Root()
	if root == "" {
		return nil, errors.New("no repository open")
	}
	return gitx.New(root), nil
}

// open activates a repository and returns its status, which the page adopts.
func (s *RepoService) open(ctx context.Context, path string, parent *mygo.Window) (gitx.Status, error) {
	root, err := s.activate(ctx, path, parent)
	if err != nil || root == "" {
		return gitx.NewStatus(""), err
	}
	return gitx.New(root).Status(ctx)
}

// activate makes path the open repository and starts watching it. It returns
// "" when the user cancels the picker. Reading the status is left to the
// caller: the native open paths let the page load it once from its event.
func (s *RepoService) activate(ctx context.Context, path string, parent *mygo.Window) (string, error) {
	if path == "" {
		// One picker at a time: a click while the sheet is up would ask
		// AppKit for a second sheet on the same window, which it refuses, and
		// the caller would wait for a dialog that never appears.
		if !s.pickerOpen.CompareAndSwap(false, true) {
			return "", nil
		}
		defer s.pickerOpen.Store(false)
		log.Printf("service: showing the repository picker")
		picked, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    parent,
			Title:     "Open Repository",
			Message:   "Choose a Git repository",
			Directory: true,
		})
		log.Printf("service: picker returned %v, err=%v", picked, err)
		if err != nil {
			return "", err
		}
		if len(picked) == 0 {
			return "", nil // canceled
		}
		path = picked[0]
	}

	root, err := gitx.Discover(ctx, path)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.root = root
	s.mu.Unlock()
	log.Printf("service: opened repository at %s", root)
	saveLastRepo(root)

	// Watching walks the tree, so it must not delay the first status.
	go s.startWatch(root)
	return root, nil
}

func (s *RepoService) window() *mygo.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.win
}

func (s *RepoService) notify(root, reason string) {
	s.mu.Lock()
	cb := s.onChange
	s.mu.Unlock()
	if cb != nil {
		cb(root, reason)
		return
	}
	if w := s.window(); w != nil {
		_ = RepoChanged.Emit(w, RepoChange{Root: root, Reason: reason})
	}
}

// markOperation keeps the watcher from echoing the app's own writes back
// to the page, which would refresh the same diff twice.
func (s *RepoService) markOperation() {
	s.mu.Lock()
	s.lastOp = time.Now()
	s.mu.Unlock()
}

func (s *RepoService) operationRecently() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.lastOp.IsZero() && time.Since(s.lastOp) < operationWindow
}

// notifyLater schedules one watcher refresh for the end of the self-write
// window. Without it, a real external change that lands inside the window
// would be dropped together with the app's own echo.
func (s *RepoService) notifyLater(root string) {
	s.mu.Lock()
	if s.pendingWatch {
		s.mu.Unlock()
		return
	}
	s.pendingWatch = true
	s.mu.Unlock()

	time.AfterFunc(operationWindow, func() {
		s.mu.Lock()
		s.pendingWatch = false
		s.mu.Unlock()
		if s.Root() == root {
			s.notify(root, "watch")
		}
	})
}

// ignoredWatchDirs are dependency and cache trees whose churn never changes
// git status. Build output directories are deliberately absent: they can hold
// tracked files, and missing their changes would make the page stale.
var ignoredWatchDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".gradle":       true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".turbo":        true,
}

// startWatch follows the worktree and reports changes. HintsOnly keeps the
// watch cheap on large repositories: any hint is enough for a status refresh.
func (s *RepoService) startWatch(root string) {
	// Watches start asynchronously, so a slower start for a repository the
	// user already left must not replace the current one.
	if s.Root() != root {
		return
	}
	s.watchMu.Lock()
	if s.watchCancel != nil {
		s.watchCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.watchCancel = cancel
	s.watchMu.Unlock()

	w, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{
		Recursive: true,
		HintsOnly: true,
		Debounce:  250 * time.Millisecond,
		MaxDelay:  time.Second,
		Ignore: func(path string, isDir bool) bool {
			return isDir && ignoredWatchDirs[filepath.Base(path)]
		},
	})
	if err != nil {
		log.Printf("service: file watch on %s unavailable: %v", root, err)
		return
	}
	if s.Root() != root {
		w.Close()
		return
	}
	log.Printf("service: watching %s for changes", root)

	go func() {
		defer w.Close()
		for {
			if _, err := w.Next(ctx); err != nil {
				return
			}
			if s.Root() != root {
				continue // the user moved on to another repository
			}
			if s.operationRecently() {
				s.notifyLater(root)
				continue
			}
			s.notify(root, "watch")
		}
	}()
}
