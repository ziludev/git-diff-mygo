import { useState } from "react";
import { GitCommitVertical } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Textarea } from "@/components/ui/textarea";
import { useRepo } from "@/store/repo";

/** Commits the index. The message survives a failed commit so nothing is lost. */
export function CommitPanel() {
  const status = useRepo((s) => s.status);
  const writing = useRepo((s) => s.writing);
  const commit = useRepo((s) => s.commit);
  const [message, setMessage] = useState("");
  const [amend, setAmend] = useState(false);

  if (!status?.root) return null;

  const staged = status.staged.length;
  const canCommit = !writing && message.trim() !== "" && (amend || staged > 0);
  const target =
    status.branch.head || (status.branch.detached ? "detached HEAD" : "branch");

  async function submit() {
    if (!canCommit) return;
    const oid = await commit(message, amend);
    if (oid) {
      setMessage("");
      setAmend(false);
    }
  }

  return (
    <div className="shrink-0 border-t p-2">
      <Textarea
        value={message}
        onChange={(e) => setMessage(e.target.value)}
        onKeyDown={(e) => {
          if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
            e.preventDefault();
            void submit();
          }
        }}
        rows={3}
        placeholder="Commit message"
        disabled={writing}
        className="min-h-16 resize-none text-xs"
      />
      <div className="mt-1.5 flex items-center justify-between gap-2">
        <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <Checkbox
            checked={amend}
            onCheckedChange={(checked) => setAmend(checked === true)}
            disabled={writing}
          />
          Amend
        </label>
        <Button
          size="sm"
          className="h-7 max-w-[62%] text-xs"
          disabled={!canCommit}
          title={`Commit to ${target}`}
          onClick={() => void submit()}
        >
          <GitCommitVertical className="size-3.5" />
          <span className="truncate">Commit to {target}</span>
        </Button>
      </div>
      <p className="mt-1 text-[10px] text-muted-foreground">
        {staged > 0
          ? `${staged} staged file${staged === 1 ? "" : "s"} · Cmd+Enter`
          : "Nothing staged · Cmd+Enter"}
      </p>
    </div>
  );
}
