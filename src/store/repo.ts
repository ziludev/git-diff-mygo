import { toast } from "sonner";
import { create } from "zustand";

import {
  RepoService,
  type ApplyOp,
  type BranchEntry,
  type Change,
  type Commit,
  type DiffResult,
  type FileEntry,
  type Status,
} from "../mygo";
import { errorMessage } from "../lib/utils";

/** The left sidebar's tab. */
export type View = "changes" | "history" | "branches";

/** One page of history; "load older" appends another page of this size. */
const historyPage = 100;

type RepoState = {
  status: Status | null;
  /** The row the diff panel shows: a `Change` from the current status. */
  selection: Change | null;
  diff: DiffResult | null;
  loadingStatus: boolean;
  loadingDiff: boolean;
  /** True while a worktree write is in flight; actions are disabled. */
  writing: boolean;
  error: string | null;

  view: View;
  commits: Commit[];
  /** True once a page was requested, so an empty list means "no commits". */
  commitsLoaded: boolean;
  hasMoreCommits: boolean;
  loadingCommits: boolean;
  historyError: string | null;
  /** The commit expanded into a file list, or null. */
  expandedCommit: Commit | null;
  commitFiles: FileEntry[];
  loadingCommitFiles: boolean;
  commitFile: FileEntry | null;
  commitDiff: DiffResult | null;
  loadingCommitDiff: boolean;
  commitError: string | null;

  branches: BranchEntry[];
  branchesLoaded: boolean;
  loadingBranches: boolean;
  branchError: string | null;

  open: () => Promise<void>;
  /** Reloads the working tree; reason is the event's, when there was one. */
  refresh: (reason?: string) => Promise<void>;
  select: (change: Change) => Promise<void>;
  stage: (change: Change) => Promise<void>;
  unstage: (change: Change) => Promise<void>;
  discard: (change: Change) => Promise<void>;
  applyHunks: (op: ApplyOp, hunk: number) => Promise<void>;
  /** Commits the index; resolves to the new commit id, or undefined on error. */
  commit: (message: string, amend: boolean) => Promise<string | undefined>;
  setView: (view: View) => void;
  /** Loads the first page of commits when none was requested yet. */
  ensureCommits: () => void;
  /** Loads the first page, or appends the next one when more is true. */
  loadCommits: (more?: boolean) => Promise<void>;
  selectCommit: (commit: Commit) => Promise<void>;
  selectCommitFile: (file: FileEntry) => Promise<void>;
  /** Loads the branch list when none was requested yet. */
  ensureBranches: () => void;
  loadBranches: () => Promise<void>;
  checkoutBranch: (name: string) => Promise<void>;
  /** Creates a branch at HEAD and checks it out. */
  createBranch: (name: string) => Promise<void>;
  deleteBranch: (name: string) => Promise<void>;
  clearError: () => void;
};

let diffRequest = 0;
let statusRequest = 0;
let commitsRequest = 0;
let commitFilesRequest = 0;
let commitDiffRequest = 0;
let branchesRequest = 0;

/** Identity of a commit file row: path plus the old side of a rename. */
export function sameFile(a: FileEntry | null, b: FileEntry | null): boolean {
  return (
    a !== null &&
    b !== null &&
    a.path === b.path &&
    a.origPath === b.origPath
  );
}

/** Identity of a row: the same path can appear in several buckets. */
export function sameChange(a: Change | null, b: Change | null): boolean {
  return (
    a !== null &&
    b !== null &&
    a.path === b.path &&
    a.staged === b.staged &&
    a.untracked === b.untracked &&
    a.conflicted === b.conflicted
  );
}

function allChanges(status: Status): Change[] {
  return [
    ...status.conflicted,
    ...status.staged,
    ...status.unstaged,
    ...status.untracked,
  ];
}

function firstSelection(status: Status): Change | null {
  // Conflicts come first, matching the order of the changes panel.
  return (
    status.conflicted[0] ??
    status.unstaged[0] ??
    status.staged[0] ??
    status.untracked[0] ??
    null
  );
}

export const useRepo = create<RepoState>((set, get) => {
  function clearDiff() {
    diffRequest++;
    set({ selection: null, diff: null, loadingDiff: false });
  }

  /** Collapses the expanded commit and cancels its pending loads. */
  function resetCommitSelection() {
    commitFilesRequest++;
    commitDiffRequest++;
    set({
      expandedCommit: null,
      commitFiles: [],
      loadingCommitFiles: false,
      commitFile: null,
      commitDiff: null,
      loadingCommitDiff: false,
      commitError: null,
    });
  }

  /** Drops another repository's branches and any in-flight branch load. */
  function clearBranches() {
    branchesRequest++;
    set({
      branches: [],
      branchesLoaded: false,
      loadingBranches: false,
      branchError: null,
    });
  }

  /** Drops the loaded history: another repository's commits are not ours. */
  function clearHistory() {
    commitsRequest++;
    resetCommitSelection();
    set({
      commits: [],
      commitsLoaded: false,
      hasMoreCommits: false,
      loadingCommits: false,
      historyError: null,
    });
  }

  async function loadDiff(change: Change) {
    const request = ++diffRequest;
    set({ selection: change, loadingDiff: true, error: null });
    try {
      const diff = await RepoService.diff({
        path: change.path,
        origPath: change.origPath,
        staged: change.staged,
        untracked: change.untracked,
      });
      if (request === diffRequest) {
        set({ diff, loadingDiff: false });
      }
    } catch (err) {
      if (request === diffRequest) {
        set({ error: errorMessage(err), loadingDiff: false });
      }
    }
  }

  /**
   * Installs a status the backend already computed, then keeps the selected
   * row when it still exists and loads its diff.
   */
  async function adoptStatus(status: Status) {
    // A different repository invalidates the commit list, the branch list and
    // their selections.
    if (get().status?.root !== status.root) {
      clearHistory();
      clearBranches();
    }
    set({ status });
    if (!status.root) {
      clearDiff();
      return;
    }
    const current = get().selection;
    const all = current ? allChanges(status) : [];
    // Keep the exact row when it still exists, otherwise follow the same
    // path into its new bucket (staging moves a row to Staged).
    const kept =
      current &&
      (all.find((c) => sameChange(c, current)) ??
        all.find((c) => c.path === current.path));
    const next = kept ?? firstSelection(status);
    if (next) {
      await loadDiff(next);
    } else {
      clearDiff();
    }
  }

  async function reloadStatus() {
    const request = ++statusRequest;
    set({ loadingStatus: true, error: null });
    try {
      const status = await RepoService.status();
      if (request !== statusRequest) return; // a newer load won
      set({ loadingStatus: false });
      await adoptStatus(status);
    } catch (err) {
      if (request === statusRequest) {
        set({ error: errorMessage(err), loadingStatus: false });
      }
    }
  }

  /**
   * Reloads the lists derived from repository state; every write, checkout and
   * watcher event goes through it. Watch events fire for each edit, so they
   * skip the 100-commit page; the branch list is one for-each-ref and follows
   * every event.
   */
  function reloadDerived(reason?: string) {
    const state = get();
    const tasks: Promise<void>[] = [];
    if (reason !== "watch" && state.commitsLoaded) {
      tasks.push(state.loadCommits());
    }
    if (state.branchesLoaded) {
      tasks.push(state.loadBranches());
    }
    return Promise.all(tasks).then(() => undefined);
  }

  /**
   * Runs one index/worktree write, refreshes the status, and reports failures
   * in a toast: a failed stage should not replace the diff with an error page.
   * The Go side does not emit an event for these writes, so this refresh is
   * the only one.
   */
  async function withWrite<T>(run: () => Promise<T>): Promise<T | undefined> {
    // Serialize writes: a second click while git runs would otherwise apply
    // a hunk index computed from the patch this write is about to replace.
    if (get().writing) return undefined;
    set({ writing: true });
    try {
      const result = await run();
      await reloadStatus();
      return result;
    } catch (err) {
      toast.error(errorMessage(err));
      return undefined;
    } finally {
      set({ writing: false });
    }
  }

  function write(run: () => Promise<void>): Promise<void> {
    return withWrite(run).then(() => undefined);
  }

  return {
    status: null,
    selection: null,
    diff: null,
    loadingStatus: false,
    loadingDiff: false,
    writing: false,
    error: null,

    view: "changes",
    commits: [],
    commitsLoaded: false,
    hasMoreCommits: false,
    loadingCommits: false,
    historyError: null,
    expandedCommit: null,
    commitFiles: [],
    loadingCommitFiles: false,
    commitFile: null,
    commitDiff: null,
    loadingCommitDiff: false,
    commitError: null,

    branches: [],
    branchesLoaded: false,
    loadingBranches: false,
    branchError: null,

    open: async () => {
      const request = ++statusRequest;
      set({ loadingStatus: true, error: null });
      try {
        const status = await RepoService.open("");
        if (request !== statusRequest) return;
        set({ loadingStatus: false });
        if (status.root) await adoptStatus(status);
      } catch (err) {
        if (request === statusRequest) {
          set({ error: errorMessage(err), loadingStatus: false });
        }
      }
    },

    refresh: async (reason) => {
      await reloadStatus();
      await reloadDerived(reason);
    },

    select: loadDiff,

    stage: (change) => write(() => RepoService.stage(change.path)),

    // A staged rename needs both sides reset, or the old path stays deleted.
    unstage: (change) =>
      write(() => RepoService.unstage(change.path, change.origPath ?? "")),

    /** Applies one hunk of the selected file, keeping the others untouched. */
    applyHunks: (op, hunk) => {
      const selection = get().selection;
      if (!selection) return Promise.resolve();
      return write(() =>
        RepoService.applyHunks({
          path: selection.path,
          origPath: selection.origPath,
          op,
          hunks: [hunk],
          untracked: selection.untracked,
        }),
      );
    },

    discard: (change) => write(() => RepoService.discard(change.path)),

    commit: async (message, amend) => {
      const oid = await withWrite(() => RepoService.commit({ message, amend }));
      if (oid) await reloadDerived();
      return oid;
    },

    // The panels load their own list when they mount, so switching tabs needs
    // no side effect here.
    setView: (view) => {
      if (get().view !== view) set({ view });
    },

    ensureCommits: () => {
      if (!get().commitsLoaded && !get().loadingCommits) void get().loadCommits();
    },

    // A page always replaces the list from the top: a newer commit must not
    // shift an older page the user already scrolled through.
    loadCommits: async (more = false) => {
      const state = get();
      if (!state.status?.root) return;
      // A refresh may supersede an in-flight "load older", but a second append
      // would duplicate the page the first one is about to add.
      if (more && state.loadingCommits) return;
      const request = ++commitsRequest;
      const skip = more ? state.commits.length : 0;
      set({ loadingCommits: true, historyError: null });
      try {
        const page = await RepoService.history({ limit: historyPage, skip });
        if (request !== commitsRequest) return;
        set({
          commits: more ? [...get().commits, ...page] : page,
          commitsLoaded: true,
          hasMoreCommits: page.length === historyPage,
          loadingCommits: false,
        });
      } catch (err) {
        if (request !== commitsRequest) return;
        set({ commitsLoaded: true, loadingCommits: false });
        // A failed append keeps the page the user already has; only the first
        // load owns the full-panel error.
        if (more) toast.error(errorMessage(err));
        else set({ historyError: errorMessage(err) });
      }
    },

    selectCommit: async (commit) => {
      // Clicking the expanded commit collapses it and drops its selection.
      if (get().expandedCommit?.oid === commit.oid) {
        resetCommitSelection();
        return;
      }
      resetCommitSelection();
      const request = ++commitFilesRequest;
      set({ expandedCommit: commit, loadingCommitFiles: true });
      try {
        const files = await RepoService.commitFiles(commit.oid);
        if (request !== commitFilesRequest) return;
        set({ commitFiles: files, loadingCommitFiles: false });
        // Show the first file right away: an expanded commit with an empty
        // right panel looks like the commit is still loading.
        if (files[0]) await get().selectCommitFile(files[0]);
      } catch (err) {
        if (request === commitFilesRequest) {
          set({ commitError: errorMessage(err), loadingCommitFiles: false });
        }
      }
    },

    selectCommitFile: async (file) => {
      const commit = get().expandedCommit;
      if (!commit) return;
      const request = ++commitDiffRequest;
      set({ commitFile: file, loadingCommitDiff: true, commitDiff: null, commitError: null });
      try {
        const diff = await RepoService.commitDiff({
          oid: commit.oid,
          path: file.path,
          origPath: file.origPath,
        });
        if (request === commitDiffRequest) {
          set({ commitDiff: diff, loadingCommitDiff: false });
        }
      } catch (err) {
        if (request === commitDiffRequest) {
          set({ commitError: errorMessage(err), loadingCommitDiff: false });
        }
      }
    },

    ensureBranches: () => {
      if (!get().branchesLoaded && !get().loadingBranches) {
        void get().loadBranches();
      }
    },

    loadBranches: async () => {
      if (!get().status?.root) return;
      const request = ++branchesRequest;
      set({ loadingBranches: true, branchError: null });
      try {
        const branches = await RepoService.branches();
        if (request !== branchesRequest) return;
        set({ branches, branchesLoaded: true, loadingBranches: false });
      } catch (err) {
        if (request !== branchesRequest) return;
        set({
          branchesLoaded: true,
          loadingBranches: false,
          branchError: errorMessage(err),
        });
      }
    },

    checkoutBranch: async (name) => {
      await withWrite(async () => {
        await RepoService.checkoutBranch(name);
        // The selected diff and expanded commit belong to the branch we left.
        clearDiff();
        resetCommitSelection();
      });
      await reloadDerived();
    },

    createBranch: async (name) => {
      await withWrite(() => RepoService.createBranch(name));
      await reloadDerived();
    },

    deleteBranch: async (name) => {
      await withWrite(() => RepoService.deleteBranch(name));
      await reloadDerived();
    },

    clearError: () => set({ error: null, commitError: null, branchError: null }),
  };
});
