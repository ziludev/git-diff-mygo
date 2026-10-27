import type { LucideIcon } from "lucide-react";

import { cn } from "@/lib/utils";

/** Centered placeholder shared by the changes list and the diff panel. */
export function EmptyState({
  icon: Icon,
  title,
  detail,
  action,
  className,
}: {
  icon?: LucideIcon;
  title: string;
  detail?: string;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex h-full flex-col items-center justify-center gap-3 p-6 text-center",
        className,
      )}
    >
      {Icon ? <Icon className="size-6 text-muted-foreground" /> : null}
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        {detail ? (
          <p className="max-w-sm text-xs text-muted-foreground">{detail}</p>
        ) : null}
      </div>
      {action}
    </div>
  );
}
