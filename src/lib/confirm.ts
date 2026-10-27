import { create } from "zustand";

type ConfirmOptions = {
  title: string;
  description?: string;
  confirmLabel?: string;
};

type ConfirmState = ConfirmOptions & {
  open: boolean;
  resolve: ((ok: boolean) => void) | null;
};

export const useConfirm = create<ConfirmState>(() => ({
  open: false,
  title: "",
  resolve: null,
}));

/**
 * Shows the in-app confirmation dialog. Using one global dialog keeps the
 * native webview panels out of the app's visual language.
 */
export function confirmAction(options: ConfirmOptions): Promise<boolean> {
  const previous = useConfirm.getState().resolve;
  previous?.(false); // a second request supersedes an unanswered one
  return new Promise<boolean>((resolve) => {
    useConfirm.setState({ ...options, open: true, resolve });
  });
}

/** Resolves the open request; a no-op when nothing is pending. */
export function answerConfirm(ok: boolean) {
  const { resolve } = useConfirm.getState();
  useConfirm.setState({ open: false, resolve: null });
  resolve?.(ok);
}
