import { memo, useEffect, useMemo } from "react";
import { ChevronRight, GitBranch, GitCommitVertical, Tag } from "lucide-react";

import { Button } from "@/components/ui/button";
import { CommitGraph } from "@/components/commit-graph";
import { EmptyState } from "@/components/empty-state";
import { ScrollArea } from "@/components/ui/scroll-area";
import { SelectableRow } from "@/components/selectable-row";
import { Skeleton } from "@/components/ui/skeleton";
import { graphLaneWidth, layoutGraph, type GraphRow } from "@/lib/graph";
import { addedTone, cn, deletedTone, relativeTime, toneFor } from "@/lib/utils";
import type { Commit, FileEntry, Ref } from "@/mygo";
import { sameFile, useRepo } from "@/store/repo";

/**
 * The history of HEAD, newest first. A commit expands in place into its
 * changed files, and a file row drives the diff panel.
 */
export function HistoryPanel() {
  const root = useRepo((s) => s.status?.root);
  const commits = useRepo((s) => s.commits);
  const loaded = useRepo((s) => s.commitsLoaded);
  const hasMore = useRepo((s) => s.hasMoreCommits);
  const loading = useRepo((s) => s.loadingCommits);
  const error = useRepo((s) => s.historyError);
  const expanded = useRepo((s) => s.expandedCommit);
  const files = useRepo((s) => s.commitFiles);
  const loadingFiles = useRepo((s) => s.loadingCommitFiles);
  const selectedFile = useRepo((s) => s.commitFile);
  const loadCommits = useRepo((s) => s.loadCommits);
  const ensureCommits = useRepo((s) => s.ensureCommits);
  const selectCommit = useRepo((s) => s.selectCommit);
  const selectCommitFile = useRepo((s) => s.selectCommitFile);

  // Lanes depend only on the loaded commits, so a "load older" recomputes the
  // whole list once instead of threading lane state through the store.
  const graph = useMemo(() => layoutGraph(commits), [commits]);
  const indent = graph.lanes * graphLaneWidth + 8;

  // The panel may mount before any page was requested (tab restore, another
  // repository); ensureCommits keeps the list correct without a click.
  useEffect(() => {
    if (root) ensureCommits();
  }, [root, ensureCommits]);

  return (
    <ScrollArea className="min-h-0 flex-1">
      {error ? (
        <EmptyState title="Could not load history" detail={error} className="h-48" />
      ) : !loaded || (loading && commits.length === 0) ? (
        <div className="space-y-2 p-3">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-9 w-full" />
          ))}
        </div>
      ) : commits.length === 0 ? (
        <EmptyState
          icon={GitCommitVertical}
          title="No commits yet"
          detail="Commits on this branch will show up here."
          className="h-48"
        />
      ) : (
        <div className="py-1">
          {graph.rows.map((row, index) => {
            const commit = commits[index];
            if (!commit) return null;
            return (
              <CommitRow
                key={commit.oid}
                commit={commit}
                graph={row}
                lanes={graph.lanes}
                indent={indent}
                expanded={expanded?.oid === commit.oid}
                files={files}
                loadingFiles={loadingFiles}
                selectedFile={selectedFile}
                onToggle={selectCommit}
                onSelectFile={selectCommitFile}
              />
            );
          })}
          {hasMore ? (
            <div className="px-2 py-2">
              <Button
                variant="ghost"
                size="sm"
                className="w-full text-xs"
                disabled={loading}
                onClick={() => void loadCommits(true)}
              >
                {loading ? "Loading…" : "Load older commits"}
              </Button>
            </div>
          ) : null}
        </div>
      )}
    </ScrollArea>
  );
}

/**
 * One row of the commit list. Memoized: the tab shell re-renders whenever the
 * status changes, and every commit in the page must not pay for it.
 */
const CommitRow = memo(function CommitRow({
  commit,
  graph,
  lanes,
  indent,
  expanded,
  files,
  loadingFiles,
  selectedFile,
  onToggle,
  onSelectFile,
}: {
  commit: Commit;
  graph: GraphRow;
  lanes: number;
  indent: number;
  expanded: boolean;
  files: FileEntry[];
  loadingFiles: boolean;
  selectedFile: FileEntry | null;
  onToggle: (commit: Commit) => void;
  onSelectFile: (file: FileEntry) => void;
}) {
  return (
    <div>
      <SelectableRow
        active={expanded}
        onSelect={() => onToggle(commit)}
        className="flex h-12 items-center gap-1 pr-2 text-xs"
      >
        <CommitGraph row={graph} lanes={lanes} />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-1">
            <ChevronRight
              className={cn(
                "size-3 shrink-0 text-muted-foreground transition-transform",
                expanded && "rotate-90",
              )}
            />
            <span className="min-w-0 flex-1 truncate font-medium" title={commit.subject}>
              {commit.subject || "(no message)"}
            </span>
            <CommitRefs refs={commit.refs} />
          </div>
          <div className="flex min-w-0 items-center gap-2 pl-4 text-[10px] text-muted-foreground">
            <span className="shrink-0 font-mono">{commit.short}</span>
            <span className="truncate">{commit.author}</span>
            <span className="ml-auto shrink-0">{relativeTime(commit.time)}</span>
          </div>
        </div>
      </SelectableRow>

      {expanded ? (
        <div className="pb-1">
          {loadingFiles ? (
            <div className="space-y-1 py-1 pr-2" style={{ paddingLeft: indent }}>
              <Skeleton className="h-5 w-full" />
              <Skeleton className="h-5 w-2/3" />
            </div>
          ) : files.length === 0 ? (
            <p className="py-1 pr-2 text-[10px] text-muted-foreground" style={{ paddingLeft: indent }}>
              No files changed.
            </p>
          ) : (
            files.map((file) => (
              <FileRow
                key={file.path}
                file={file}
                indent={indent}
                selected={sameFile(selectedFile, file)}
                onSelect={() => onSelectFile(file)}
              />
            ))
          )}
        </div>
      ) : null}
    </div>
  );
});

/** Branch and tag labels of one commit, capped so the subject keeps its room. */
function CommitRefs({ refs }: { refs: Ref[] }) {
  if (refs.length === 0) return null;
  const shown = refs.slice(0, 2);
  const hidden = refs.slice(shown.length);
  return (
    <span className="flex min-w-0 shrink items-center gap-1">
      {shown.map((ref) => (
        <span
          key={`${ref.kind}:${ref.name}`}
          title={`${ref.kind}: ${ref.name}`}
          className={cn(
            "flex max-w-24 min-w-0 items-center gap-0.5 rounded border px-1 text-[9px] leading-4",
            ref.kind === "tag"
              ? "border-amber-600/40 text-amber-700 dark:text-amber-400"
              : "border-border text-muted-foreground",
          )}
        >
          {ref.kind === "tag" ? (
            <Tag className="size-2.5 shrink-0" />
          ) : (
            <GitBranch className="size-2.5 shrink-0" />
          )}
          <span className="truncate">{ref.name}</span>
        </span>
      ))}
      {hidden.length > 0 ? (
        <span
          className="shrink-0 text-[9px] text-muted-foreground"
          title={hidden.map((ref) => ref.name).join(", ")}
        >
          +{hidden.length}
        </span>
      ) : null}
    </span>
  );
}

function FileRow({
  file,
  indent,
  selected,
  onSelect,
}: {
  file: FileEntry;
  indent: number;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <SelectableRow
      active={selected}
      onSelect={onSelect}
      className="flex h-6 items-center gap-2 pr-2 text-xs"
      style={{ paddingLeft: indent }}
    >
      <span
        className={cn("w-3 shrink-0 text-center font-mono text-[10px] font-bold", toneFor(file.status))}
      >
        {file.status}
      </span>
      <span
        className="min-w-0 flex-1 truncate font-mono text-[11px]"
        title={file.origPath ? `${file.origPath} → ${file.path}` : file.path}
      >
        {file.path}
      </span>
      {file.binary ? (
        <span className="shrink-0 text-[10px] text-muted-foreground">bin</span>
      ) : (
        <span className="shrink-0 font-mono text-[10px]">
          <span className={addedTone}>+{file.additions}</span>{" "}
          <span className={deletedTone}>−{file.deletions}</span>
        </span>
      )}
    </SelectableRow>
  );
}
