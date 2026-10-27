import { toast } from "sonner";
import { create } from "zustand";

import {
  RepoService,
  type ApplyOp,
  type Change,
  type DiffResult,
  type Status,
} from "../mygo";
import { errorMessage } from "../lib/utils";

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

  open: () => Promise<void>;
  refresh: () => Promise<void>;
  select: (change: Change) => Promise<void>;
  stage: (change: Change) => Promise<void>;
  unstage: (change: Change) => Promise<void>;
  discard: (change: Change) => Promise<void>;
  applyHunks: (op: ApplyOp, hunk: number) => Promise<void>;
  /** Commits the index; resolves to the new commit id, or undefined on error. */
  commit: (message: string, amend: boolean) => Promise<string | undefined>;
  clearError: () => void;
};

let diffRequest = 0;
let statusRequest = 0;

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

    refresh: reloadStatus,

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

    commit: (message, amend) =>
      withWrite(() => RepoService.commit({ message, amend })),

    clearError: () => set({ error: null }),
  };
});
