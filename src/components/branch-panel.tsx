import { useEffect, useMemo, useRef, useState } from "react";
import { Check, GitBranch, Plus, Trash2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
import { FilterInput } from "@/components/filter-input";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { SelectableRow } from "@/components/selectable-row";
import { Skeleton } from "@/components/ui/skeleton";
import { confirmAction } from "@/lib/confirm";
import { cn, relativeTime } from "@/lib/utils";
import { useRepo } from "@/store/repo";

/**
 * Local branches: the checked-out one on top, the rest by name. Clicking a
 * row checks it out; the header creates and the row action deletes.
 */
export function BranchPanel() {
  const status = useRepo((s) => s.status);
  const root = status?.root;
  const head = status?.branch;
  const detached = head?.detached ?? false;
  // An unborn HEAD has no commit, so git cannot point a branch at one.
  const unborn = Boolean(status) && !head?.oid && !head?.detached;
  const branches = useRepo((s) => s.branches);
  const loaded = useRepo((s) => s.branchesLoaded);
  const loading = useRepo((s) => s.loadingBranches);
  const error = useRepo((s) => s.branchError);
  const writing = useRepo((s) => s.writing);
  const ensureBranches = useRepo((s) => s.ensureBranches);
  const checkoutBranch = useRepo((s) => s.checkoutBranch);
  const createBranch = useRepo((s) => s.createBranch);
  const deleteBranch = useRepo((s) => s.deleteBranch);

  const [query, setQuery] = useState("");
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (root) ensureBranches();
  }, [root, ensureBranches]);

  useEffect(() => {
    if (creating) inputRef.current?.focus();
  }, [creating]);

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return branches;
    return branches.filter((b) => b.name.toLowerCase().includes(needle));
  }, [branches, query]);

  const filtering = query.trim() !== "";
  const emptyTitle = filtering
    ? "No matching branches"
    : unborn
      ? "No branches yet"
      : "No branches";
  const emptyDetail =
    !filtering && unborn ? "Branches appear after the first commit." : undefined;

  function closeCreate() {
    setCreating(false);
    setName("");
  }

  async function submitCreate() {
    const branch = name.trim();
    if (!branch || writing) return;
    closeCreate();
    await createBranch(branch);
  }

  async function requestDelete(branch: string) {
    const ok = await confirmAction({
      title: `Delete branch ${branch}?`,
      description:
        "Only the branch pointer is removed. Git refuses a branch whose commits are not merged elsewhere.",
      confirmLabel: "Delete",
    });
    if (ok) await deleteBranch(branch);
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 items-center gap-1.5 border-b p-2">
        <div className="min-w-0 flex-1">
          <FilterInput value={query} onChange={setQuery} placeholder="Filter branches" />
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="size-7 p-0"
          title="New branch from HEAD"
          disabled={writing || unborn}
          onClick={() => setCreating(true)}
        >
          <Plus className="size-4" />
        </Button>
      </div>

      {creating ? (
        <div className="flex shrink-0 items-center gap-1 border-b px-2 py-1.5">
          <Input
            ref={inputRef}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="new-branch-name"
            className="h-7 flex-1 font-mono text-xs"
            onKeyDown={(e) => {
              if (e.key === "Enter") void submitCreate();
              if (e.key === "Escape") closeCreate();
            }}
          />
          <Button
            size="sm"
            className="size-7 p-0"
            title="Create branch"
            disabled={!name.trim() || writing}
            onClick={() => void submitCreate()}
          >
            <Check className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="size-7 p-0"
            title="Cancel"
            onClick={closeCreate}
          >
            <X className="size-3.5" />
          </Button>
        </div>
      ) : null}

      <ScrollArea className="min-h-0 flex-1">
        {error ? (
          <EmptyState title="Could not load branches" detail={error} className="h-48" />
        ) : !loaded || (loading && branches.length === 0) ? (
          <div className="space-y-2 p-3">
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} className="h-7 w-full" />
            ))}
          </div>
        ) : (
          <div className="py-1">
            {detached ? (
              <p className="mx-2 mb-1 rounded border border-amber-600/40 px-2 py-1 text-[10px] text-amber-700 dark:text-amber-400">
                Detached HEAD — check out a branch to continue.
              </p>
            ) : null}
            {visible.length === 0 ? (
              <EmptyState
                icon={GitBranch}
                title={emptyTitle}
                detail={emptyDetail}
                className="h-40"
              />
            ) : (
              visible.map((branch) => (
                <SelectableRow
                  key={branch.name}
                  active={branch.current}
                  onSelect={() => {
                    if (!branch.current && !writing) void checkoutBranch(branch.name);
                  }}
                  className="group flex h-8 items-center gap-2 px-2 text-xs"
                >
                  <GitBranch
                    className={cn(
                      "size-3.5 shrink-0",
                      branch.current ? "text-foreground" : "text-muted-foreground",
                    )}
                  />
                  <span
                    className="min-w-0 flex-1 truncate font-mono text-[11px]"
                    title={branch.subject || branch.name}
                  >
                    {branch.name}
                  </span>

                  {branch.current ? (
                    <span className="flex shrink-0 items-center gap-0.5 text-[10px] text-muted-foreground">
                      <Check className="size-3" />
                      current
                    </span>
                  ) : null}
                  {branch.current && head && (head.ahead > 0 || head.behind > 0) ? (
                    <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                      {head.ahead > 0 ? `↑${head.ahead}` : ""}
                      {head.behind > 0 ? `↓${head.behind}` : ""}
                    </span>
                  ) : null}
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    {relativeTime(branch.time)}
                  </span>

                  {!branch.current ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="size-6 p-0 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100 group-focus-within:opacity-100"
                      title={`Delete branch ${branch.name}`}
                      disabled={writing}
                      onClick={(e) => {
                        e.stopPropagation();
                        void requestDelete(branch.name);
                      }}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  ) : null}
                </SelectableRow>
              ))
            )}
          </div>
        )}
      </ScrollArea>
    </div>
  );
}
