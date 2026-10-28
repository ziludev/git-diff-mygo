import { useCallback, useMemo, useState } from "react";
import { getSingularPatch } from "@pierre/diffs";
import { PatchDiff, type DiffLineAnnotation } from "@pierre/diffs/react";
import {
  AlertTriangle,
  Columns2,
  FileWarning,
  Minus,
  Plus,
  Rows3,
  Undo2,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { confirmAction } from "@/lib/confirm";
import { addedTone, deletedTone } from "@/lib/utils";
import type { ThemeType } from "@/hooks/use-theme";
import { useRepo } from "@/store/repo";

type Layout = "split" | "unified";

export function DiffPanel({ theme }: { theme: ThemeType }) {
  const hasRepo = useRepo((s) => Boolean(s.status?.root));
  const history = useRepo((s) => s.view === "history");
  const worktreeSelection = useRepo((s) => s.selection);
  const worktreeDiff = useRepo((s) => s.diff);
  const loadingWorktreeDiff = useRepo((s) => s.loadingDiff);
  const worktreeError = useRepo((s) => s.error);
  const commit = useRepo((s) => s.expandedCommit);
  const commitFile = useRepo((s) => s.commitFile);
  const commitDiff = useRepo((s) => s.commitDiff);
  const loadingCommitDiff = useRepo((s) => s.loadingCommitDiff);
  const loadingCommitFiles = useRepo((s) => s.loadingCommitFiles);
  const commitError = useRepo((s) => s.commitError);
  const clearError = useRepo((s) => s.clearError);
  const applyHunks = useRepo((s) => s.applyHunks);
  const writing = useRepo((s) => s.writing);
  const [layout, setLayout] = useState<Layout>("split");

  // The panel renders one of two sources: the worktree row or a file inside a
  // commit. History diffs are read-only, so no hunk actions attach to them.
  const target = history
    ? {
        path: commitFile?.path,
        origPath: commitFile?.origPath,
        diff: commitDiff,
        loading: loadingCommitDiff,
        error: commitError,
        editable: false,
        errorTitle: "Could not load the commit",
        placeholder: !commit
          ? "Select a commit to see its files."
          : !commitFile && !loadingCommitFiles
            ? "This commit changed no files."
            : null,
      }
    : {
        path: worktreeSelection?.path,
        origPath: worktreeSelection?.origPath,
        diff: worktreeDiff,
        loading: loadingWorktreeDiff,
        error: worktreeError,
        // Conflicts, untracked files and renames cannot take a partial apply:
        // git needs a whole-file operation for those.
        editable: Boolean(
          worktreeSelection &&
            !worktreeSelection.untracked &&
            !worktreeSelection.conflicted &&
            !worktreeSelection.origPath,
        ),
        errorTitle: worktreeSelection
          ? "Could not load the diff"
          : "Could not load the repository",
        placeholder: worktreeSelection ? null : "Select a file to see its diff.",
      };
  const { diff, loading, error, path, origPath } = target;
  
  // One parse of the patch feeds both the stats and the hunk annotations.
  // The viewer parses the same patch, so anchors and hunk order agree.
  const fileDiff = useMemo(() => {
    if (!diff?.patch || diff.binary) return null;
    try {
      return getSingularPatch(diff.patch);
    } catch {
      return null;
    }
  }, [diff?.patch, diff?.binary]);

  const stats = useMemo(() => {
    let added = 0;
    let deleted = 0;
    for (const hunk of fileDiff?.hunks ?? []) {
      added += hunk.additionLines;
      deleted += hunk.deletionLines;
    }
    return { added, deleted };
  }, [fileDiff]);

  // Unlike the viewer's parse, this survives a patch the viewer rejects: a
  // rename or mode change has a header but no hunk to render.
  const hasHunks = useMemo(() => Boolean(diff?.patch.includes("@@ ")), [diff?.patch]);
  const canApplyHunks = target.editable;

  const annotations = useMemo<DiffLineAnnotation<number>[]>(() => {
    if (!canApplyHunks || !fileDiff) return [];
    return fileDiff.hunks.map((hunk, index) => {
      const onAdditions = hunk.additionLines > 0 && hunk.additionStart > 0;
      return {
        side: onAdditions ? "additions" : "deletions",
        lineNumber: onAdditions ? hunk.additionStart : hunk.deletionStart,
        metadata: index,
      };
    });
  }, [canApplyHunks, fileDiff]);

  const options = useMemo(
    () => ({
      diffStyle: layout === "split" ? ("split" as const) : ("unified" as const),
      diffIndicators: "bars" as const,
      hunkSeparators: "line-info" as const,
      lineDiffType: "word-alt" as const,
      collapsedContextThreshold: 2,
      expandUnchanged: false,
      overflow: "scroll" as const,
      disableFileHeader: true,
      theme: { light: "pierre-light", dark: "pierre-dark" } as const,
      themeType: theme,
    }),
    [layout, theme],
  );

  const renderHunkActions = useCallback(
    (annotation: DiffLineAnnotation<number>) => {
      const hunk = annotation.metadata;
      return (
        <div className="flex items-center gap-2 border-y border-border/60 bg-muted/40 px-3 py-1 text-[11px]">
          <span className="font-medium text-muted-foreground">
            Hunk {hunk + 1}
          </span>
          <div className="ml-auto flex items-center gap-1">
            {worktreeSelection?.staged ? (
              <Button
                size="xs"
                variant="ghost"
                disabled={writing}
                onClick={() => void applyHunks("unstage", hunk)}
              >
                <Minus className="size-3" />
                Unstage hunk
              </Button>
            ) : (
              <>
                <Button
                  size="xs"
                  variant="ghost"
                  disabled={writing}
                  onClick={() => void applyHunks("stage", hunk)}
                >
                  <Plus className="size-3" />
                  Stage hunk
                </Button>
                <Button
                  size="xs"
                  variant="ghost"
                  className="text-muted-foreground hover:text-destructive"
                  disabled={writing}
                  onClick={() => {
                    void confirmAction({
                      title: `Discard hunk ${hunk + 1} of ${worktreeSelection?.path}?`,
                      description:
                        "This cannot be undone. Only this hunk is reverted.",
                      confirmLabel: "Discard",
                    }).then((ok) => {
                      if (ok) void applyHunks("discard", hunk);
                    });
                  }}
                >
                  <Undo2 className="size-3" />
                  Discard hunk
                </Button>
              </>
            )}
          </div>
        </div>
      );
    },
    [applyHunks, worktreeSelection, writing],
  );

  if (!hasRepo) {
    return <EmptyState title="Open a repository to see its changes." />;
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
        <div className="flex min-w-0 items-center gap-1.5 text-xs">
          <span className="truncate font-mono font-medium">
            {path ?? "No file selected"}
          </span>
          {origPath ? (
            <>
              <span className="text-muted-foreground">←</span>
              <span className="truncate font-mono text-muted-foreground">
                {origPath}
              </span>
            </>
          ) : null}
        </div>

        {diff ? (
          <div className="flex items-center gap-1.5 font-mono text-[10px]">
            <span className={addedTone}>+{stats.added}</span>
            <span className={deletedTone}>−{stats.deleted}</span>
          </div>
        ) : null}

        <div className="ml-auto flex items-center gap-1">
          {history ? (
            commit ? (
              <>
                <Badge variant="outline" className="font-mono font-normal">
                  {commit.short}
                </Badge>
                <span
                  className="max-w-44 truncate text-[11px] text-muted-foreground"
                  title={commit.subject}
                >
                  {commit.subject}
                </span>
              </>
            ) : null
          ) : worktreeSelection?.conflicted ? (
            <Badge variant="destructive" className="font-normal">
              Conflict
            </Badge>
          ) : worktreeSelection?.staged ? (
            <Badge variant="outline" className="font-normal">
              Staged
            </Badge>
          ) : worktreeSelection?.untracked ? (
            <Badge variant="outline" className="font-normal">
              Untracked
            </Badge>
          ) : (
            <Badge variant="secondary" className="font-normal">
              Working tree
            </Badge>
          )}
          <Button
            variant={layout === "split" ? "secondary" : "ghost"}
            size="sm"
            className="size-7 p-0"
            title="Side by side"
            aria-pressed={layout === "split"}
            onClick={() => setLayout("split")}
          >
            <Columns2 className="size-4" />
          </Button>
          <Button
            variant={layout === "unified" ? "secondary" : "ghost"}
            size="sm"
            className="size-7 p-0"
            title="Unified"
            aria-pressed={layout === "unified"}
            onClick={() => setLayout("unified")}
          >
            <Rows3 className="size-4" />
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1">
        {error ? (
          <EmptyState
            icon={AlertTriangle}
            title={target.errorTitle}
            detail={error}
            action={
              <Button variant="outline" size="sm" onClick={clearError}>
                Dismiss
              </Button>
            }
          />
        ) : target.placeholder ? (
          <EmptyState title={target.placeholder} />
        ) : loading ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-4 w-5/6" />
            <Skeleton className="h-4 w-3/4" />
          </div>
        ) : !diff ? (
          // A dismissed error leaves the row selected but no patch; a skeleton
          // here would never end.
          <EmptyState title="No diff to show" detail="Select the file again to retry." />
        ) : diff.tooLarge ? (
          <EmptyState
            icon={FileWarning}
            title="File is too large to show"
            detail="Open it in another tool, or stage it directly from the file list."
          />
        ) : diff.binary ? (
          <EmptyState
            icon={FileWarning}
            title="Binary file"
            detail="Binary changes are not rendered yet."
          />
        ) : !diff.patch.trim() || !hasHunks ? (
          // A pure rename or mode change has a patch header but no hunks, and
          // the viewer would render an empty body for it.
          <EmptyState
            icon={FileWarning}
            title="No textual changes"
            detail={
              origPath
                ? "The file was renamed or moved without content changes."
                : "The file has no line changes."
            }
          />
        ) : (
          <PatchDiff<number>
            patch={diff.patch}
            className="h-full"
            lineAnnotations={annotations}
            renderAnnotation={renderHunkActions}
            options={options}
          />
        )}
      </div>
    </div>
  );
}
