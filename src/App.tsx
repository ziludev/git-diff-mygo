import { Group, Panel, Separator } from "react-resizable-panels";

import { ChangesPanel } from "@/components/changes-panel";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DiffPanel } from "@/components/diff-panel";
import { Toaster } from "@/components/ui/sonner";
import { Toolbar } from "@/components/toolbar";
import { useThemeType } from "@/hooks/use-theme";

export function App() {
  // Applies the system theme's `dark` class to <html> and feeds the diff
  // viewer and toasts, so only one media-query subscription exists.
  const theme = useThemeType();

  return (
    <div className="flex h-full min-h-0 flex-col bg-background text-foreground">
      <Toolbar />
      <Group orientation="horizontal" className="min-h-0 flex-1">
        <Panel defaultSize="26" minSize="18" maxSize="44" className="min-h-0">
          <ChangesPanel />
        </Panel>
        <Separator className="w-px bg-border transition-colors data-[separator=active]:bg-ring" />
        <Panel className="min-h-0">
          <DiffPanel theme={theme} />
        </Panel>
      </Group>
      <ConfirmDialog />
      <Toaster theme={theme} position="bottom-right" />
    </div>
  );
}
