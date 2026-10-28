import { GitBranch, GitCommitVertical, ListChecks } from "lucide-react";

import { BranchPanel } from "@/components/branch-panel";
import { ChangesPanel } from "@/components/changes-panel";
import { HistoryPanel } from "@/components/history-panel";
import { cn } from "@/lib/utils";
import { useRepo } from "@/store/repo";

/**
 * The left sidebar: working tree changes, the history of every ref, or the
 * local branches. The tabs share the diff panel on the right.
 */
export function RepoPanel() {
  const hasRepo = useRepo((s) => Boolean(s.status?.root));
  const view = useRepo((s) => s.view);
  const setView = useRepo((s) => s.setView);
  // A number selector keeps the panel from re-rendering on unrelated status
  // fields; summing the buckets avoids allocating the row list per store tick.
  const changeCount = useRepo((s) =>
    s.status
      ? s.status.conflicted.length +
        s.status.staged.length +
        s.status.unstaged.length +
        s.status.untracked.length
      : 0,
  );

  // Without a repository the changes panel holds the "open" call to action.
  if (!hasRepo) return <ChangesPanel />;

  return (
    <div className="flex h-full min-h-0 flex-col bg-sidebar">
      <div
        role="tablist"
        aria-label="Repository views"
        className="flex h-9 shrink-0 items-center gap-1 border-b px-2"
      >
        <Tab active={view === "changes"} onClick={() => setView("changes")}>
          <ListChecks className="size-3.5" />
          Changes
          <span className="font-mono text-[10px] opacity-70">{changeCount}</span>
        </Tab>
        <Tab active={view === "history"} onClick={() => setView("history")}>
          <GitCommitVertical className="size-3.5" />
          History
        </Tab>
        <Tab active={view === "branches"} onClick={() => setView("branches")}>
          <GitBranch className="size-3.5" />
          Branches
        </Tab>
      </div>
      {view === "history" ? (
        <HistoryPanel />
      ) : view === "branches" ? (
        <BranchPanel />
      ) : (
        <ChangesPanel />
      )}
    </div>
  );
}

function Tab({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={cn(
        "flex h-6 items-center gap-1.5 rounded-md px-2 text-xs outline-none transition-colors focus-visible:ring-1 focus-visible:ring-ring",
        active
          ? "bg-accent font-medium text-accent-foreground"
          : "text-muted-foreground hover:bg-accent/50",
      )}
    >
      {children}
    </button>
  );
}
