import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { answerConfirm, useConfirm } from "@/lib/confirm";

/** Hosts the one confirmation dialog the app uses for destructive actions. */
export function ConfirmDialog() {
  const open = useConfirm((s) => s.open);
  const title = useConfirm((s) => s.title);
  const description = useConfirm((s) => s.description);
  const confirmLabel = useConfirm((s) => s.confirmLabel);

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) answerConfirm(false);
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description ? (
            <AlertDialogDescription>{description}</AlertDialogDescription>
          ) : null}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => answerConfirm(true)}>
            {confirmLabel ?? "Confirm"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
