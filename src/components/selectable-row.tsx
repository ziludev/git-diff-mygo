import { cn } from "@/lib/utils";

/**
 * A list row that behaves like a button: click, Enter or Space selects it.
 * The changes list and both history lists share it so keyboard handling and
 * focus styling cannot drift apart.
 */
export function SelectableRow({
  active,
  onSelect,
  className,
  style,
  children,
}: {
  active: boolean;
  onSelect: () => void;
  className?: string;
  style?: React.CSSProperties;
  children: React.ReactNode;
}) {
  return (
    <div
      role="button"
      tabIndex={0}
      style={style}
      onClick={onSelect}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect();
        }
      }}
      className={cn(
        "cursor-default outline-none",
        active
          ? "bg-accent text-accent-foreground"
          : "hover:bg-accent/50 focus-visible:bg-accent/50",
        className,
      )}
    >
      {children}
    </div>
  );
}
