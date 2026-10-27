import { FolderOpen, GitBranch, RefreshCw } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useRepo } from "@/store/repo";

export function Toolbar() {
  const status = useRepo((s) => s.status);
  const loading = useRepo((s) => s.loadingStatus);
  const open = useRepo((s) => s.open);
  const refresh = useRepo((s) => s.refresh);
  const branch = status
    ? status.branch.head ||
      (status.branch.detached ? "detached HEAD" : "unknown branch")
    : "No repository";

  return (
    <header className="flex h-11 shrink-0 items-center gap-1.5 border-b px-2">
      <Button variant="ghost" size="sm" onClick={() => void open()}>
        <FolderOpen className="size-4" />
        Open
      </Button>

      <Button
        variant="ghost"
        size="sm"
        className="size-8 p-0"
        title="Refresh"
        disabled={!status?.root || loading}
        onClick={() => void refresh()}
      >
        <RefreshCw className={cn("size-4", loading && "animate-spin")} />
      </Button>

      <div className="mx-1 h-5 w-px bg-border" />

      <div className="flex min-w-0 items-center gap-1.5 text-xs">
        <GitBranch className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate font-medium">{branch}</span>
        {status?.root ? (
          <span className="hidden truncate text-muted-foreground sm:inline">{status.root}</span>
        ) : null}
      </div>

      <div className="ml-auto flex items-center gap-1.5">
        <Badge variant="secondary" className="font-normal">
          {status?.unstaged.length ?? 0} changes
        </Badge>
        <Badge variant="outline" className="font-normal">
          {status?.staged.length ?? 0} staged
        </Badge>
        {(status?.conflicted.length ?? 0) > 0 ? (
          <Badge variant="destructive" className="font-normal">
            {status?.conflicted.length} conflicts
          </Badge>
        ) : null}
      </div>
    </header>
  );
}
