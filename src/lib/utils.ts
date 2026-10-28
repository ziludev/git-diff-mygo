import { cn } from "cn";

export { cn };

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  return JSON.stringify(err);
}

/** Text colors for added and deleted line counts. */
export const addedTone = "text-emerald-600 dark:text-emerald-400";
export const deletedTone = "text-red-600 dark:text-red-400";

/** Text color for a git status letter (A, M, D, R, C, ?). */
export function toneFor(code: string): string {
  switch (code) {
    case "A":
      return addedTone;
    case "M":
      return "text-amber-600 dark:text-amber-400";
    case "D":
      return deletedTone;
    case "R":
    case "C":
      return "text-sky-600 dark:text-sky-400";
    default:
      return "text-muted-foreground";
  }
}

// The UI is English; a system-locale formatter would mix "4分钟前" into it.
const relativeTimeFormat = new Intl.RelativeTimeFormat("en", {
  numeric: "auto",
  style: "narrow",
});

/** Formats a commit time (unix seconds) as "3h ago" and similar. */
export function relativeTime(unixSeconds: number): string {
  const delta = Math.round(unixSeconds - Date.now() / 1000);
  const abs = Math.abs(delta);
  if (abs < 60) return relativeTimeFormat.format(delta, "second");
  if (abs < 3600) return relativeTimeFormat.format(Math.round(delta / 60), "minute");
  if (abs < 86400) return relativeTimeFormat.format(Math.round(delta / 3600), "hour");
  if (abs < 86400 * 30) return relativeTimeFormat.format(Math.round(delta / 86400), "day");
  if (abs < 86400 * 365) {
    return relativeTimeFormat.format(Math.round(delta / (86400 * 30)), "month");
  }
  return relativeTimeFormat.format(Math.round(delta / (86400 * 365)), "year");
}
