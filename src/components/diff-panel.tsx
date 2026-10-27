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
import type { ThemeType } from "@/hooks/use-theme";
import { useRepo } from "@/store/repo";

type Layout = "split" | "unified";

export function DiffPanel({ theme }: { theme: ThemeType }) {
  const hasRepo = useRepo((s) => Boolean(s.status?.root));
  const selection = useRepo((s) => s.selection);
  const diff = useRepo((s) => s.diff);
  const loading = useRepo((s) => s.loadingDiff);
  const error = useRepo((s) => s.error);
  const clearError = useRepo((s) => s.clearError);
  const applyHunks = useRepo((s) => s.applyHunks);
  const writing = useRepo((s) => s.writing);
  const [layout, setLayout] = useState<Layout>("split");

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

  // Conflicts, untracked files, and renames cannot take a partial apply: git
  // needs a whole-file operation for those.
  const canApplyHunks = Boolean(
    selection && !selection.untracked && !selection.conflicted && !selection.origPath,
  );

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
            {selection?.staged ? (
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
                      title: `Discard hunk ${hunk + 1} of ${selection?.path}?`,
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
    [applyHunks, selection, writing],
  );

  if (!hasRepo) {
    return <EmptyState title="Open a repository to see its changes." />;
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
        <div className="flex min-w-0 items-center gap-1.5 text-xs">
          <span className="truncate font-mono font-medium">
            {selection?.path ?? "No file selected"}
          </span>
          {selection?.origPath ? (
            <>
              <span className="text-muted-foreground">←</span>
              <span className="truncate font-mono text-muted-foreground">
                {selection.origPath}
              </span>
            </>
          ) : null}
        </div>

        {diff ? (
          <div className="flex items-center gap-1.5 font-mono text-[10px]">
            <span className="text-emerald-600 dark:text-emerald-400">
              +{stats.added}
            </span>
            <span className="text-red-600 dark:text-red-400">
              −{stats.deleted}
            </span>
          </div>
        ) : null}

        <div className="ml-auto flex items-center gap-1">
          {selection?.conflicted ? (
            <Badge variant="destructive" className="font-normal">
              Conflict
            </Badge>
          ) : selection?.staged ? (
            <Badge variant="outline" className="font-normal">
              Staged
            </Badge>
          ) : selection?.untracked ? (
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
            title={
              selection ? "Could not load the diff" : "Could not load the repository"
            }
            detail={error}
            action={
              <Button variant="outline" size="sm" onClick={clearError}>
                Dismiss
              </Button>
            }
          />
        ) : !selection ? (
          <EmptyState title="Select a file to see its diff." />
        ) : loading || !diff ? (
          <div className="space-y-2 p-4">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-4 w-5/6" />
            <Skeleton className="h-4 w-3/4" />
          </div>
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
        ) : !diff.patch.trim() ? (
          <EmptyState
            icon={FileWarning}
            title="No textual changes"
            detail="The file has no line changes."
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
