import { useMemo, useState } from "react";
import { FileQuestion, FolderOpen, Minus, Plus, Search, Undo2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CommitPanel } from "@/components/commit-panel";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { EmptyState } from "@/components/empty-state";
import { confirmAction } from "@/lib/confirm";
import { cn } from "@/lib/utils";
import type { Change } from "@/mygo";
import { sameChange, useRepo } from "@/store/repo";

type Section = {
  key: string;
  title: string;
  items: Change[];
};

function letterFor(change: Change): string {
  const code = change.staged ? change.x : change.y;
  if (!code || code === ".") return "?";
  return code;
}

function toneFor(code: string): string {
  switch (code) {
    case "A":
      return "text-emerald-600 dark:text-emerald-400";
    case "M":
      return "text-amber-600 dark:text-amber-400";
    case "D":
      return "text-red-600 dark:text-red-400";
    case "R":
    case "C":
      return "text-sky-600 dark:text-sky-400";
    default:
      return "text-muted-foreground";
  }
}

export function ChangesPanel() {
  const status = useRepo((s) => s.status);
  const selection = useRepo((s) => s.selection);
  const select = useRepo((s) => s.select);
  const stage = useRepo((s) => s.stage);
  const unstage = useRepo((s) => s.unstage);
  const discard = useRepo((s) => s.discard);
  const open = useRepo((s) => s.open);
  const writing = useRepo((s) => s.writing);
  const [query, setQuery] = useState("");

  const sections = useMemo<Section[]>(() => {
    if (!status) return [];
    const match = (c: Change) =>
      c.path.toLowerCase().includes(query.trim().toLowerCase());
    return [
      { key: "conflicts", title: "Conflicts", items: status.conflicted.filter(match) },
      { key: "staged", title: "Staged", items: status.staged.filter(match) },
      { key: "unstaged", title: "Changes", items: status.unstaged.filter(match) },
      { key: "untracked", title: "Untracked", items: status.untracked.filter(match) },
    ].filter((s) => s.items.length > 0);
  }, [status, query]);

  if (!status?.root) {
    return (
      <EmptyState
        icon={FolderOpen}
        title="Open a repository"
        detail="Choose a folder, or drop one on the window."
        action={
          <Button size="sm" onClick={() => void open()}>
            <FolderOpen className="size-4" />
            Open Repository
          </Button>
        }
      />
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-sidebar">
      <div className="shrink-0 border-b p-2">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter files"
            className="h-7 pl-7 text-xs"
          />
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        {sections.length === 0 ? (
          <EmptyState
            icon={FileQuestion}
            title={query ? "No matching files" : "Working tree clean"}
            className="h-40"
          />
        ) : (
          <div className="py-1">
            {sections.map((section) => (
              <div key={section.key} className="pb-1">
                <div className="flex h-7 items-center gap-2 px-2 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  <span>{section.title}</span>
                  <Badge variant="secondary" className="h-4 px-1 text-[9px] font-medium">
                    {section.items.length}
                  </Badge>
                </div>
                {section.items.map((change) => {
                  const active = sameChange(selection, change);
                  const code = letterFor(change);
                  // A conflicted or staged row cannot be staged; any other row
                  // (including an untracked file) can.
                  const canStage = !change.staged && !change.conflicted;
                  const canUnstage = change.staged;
                  const canDiscard =
                    !change.staged && !change.untracked && !change.conflicted;
                  return (
                    <div
                      key={`${section.key}:${change.path}`}
                      role="button"
                      tabIndex={0}
                      onClick={() => void select(change)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          void select(change);
                        }
                      }}
                      className={cn(
                        "group flex h-7 cursor-default items-center gap-2 px-2 text-xs outline-none",
                        active
                          ? "bg-accent text-accent-foreground"
                          : "hover:bg-accent/50 focus-visible:bg-accent/50",
                      )}
                    >
                      <span
                        className={cn(
                          "w-3 shrink-0 text-center font-mono text-[10px] font-bold",
                          toneFor(code),
                        )}
                      >
                        {code}
                      </span>
                      <span
                        className="min-w-0 flex-1 truncate font-mono text-[11px]"
                        title={
                          change.origPath
                            ? `${change.origPath} → ${change.path}`
                            : change.path
                        }
                      >
                        {change.path}
                      </span>

                      <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
                        {canStage ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="size-6 p-0"
                            title="Stage"
                            disabled={writing}
                            onClick={(e) => {
                              e.stopPropagation();
                              void stage(change);
                            }}
                          >
                            <Plus className="size-3.5" />
                          </Button>
                        ) : null}
                        {canUnstage ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="size-6 p-0"
                            title="Unstage"
                            disabled={writing}
                            onClick={(e) => {
                              e.stopPropagation();
                              void unstage(change);
                            }}
                          >
                            <Minus className="size-3.5" />
                          </Button>
                        ) : null}
                        {canDiscard ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="size-6 p-0 text-muted-foreground hover:text-destructive"
                            title="Discard changes"
                            disabled={writing}
                            onClick={(e) => {
                              e.stopPropagation();
                              void confirmAction({
                                title: `Discard changes to ${change.path}?`,
                                description:
                                  "This cannot be undone. The file returns to the index version.",
                                confirmLabel: "Discard",
                              }).then((ok) => {
                                if (ok) void discard(change);
                              });
                            }}
                          >
                            <Undo2 className="size-3.5" />
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  );
                })}
              </div>
            ))}
          </div>
        )}
      </ScrollArea>
      <CommitPanel />
    </div>
  );
}
