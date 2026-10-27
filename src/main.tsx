import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { events } from "./mygo";
import { useRepo } from "./store/repo";
import "./style.css";

// The Go side emits this after a menu action, a write, or a watcher event.
events.repoChanged.on(() => {
  void useRepo.getState().refresh();
});

// The window may already have a repository open: the Go side can open one
// from the command line before this page subscribes to events, so read the
// current state once on mount.
void useRepo.getState().refresh();

const root = document.getElementById("root");
if (!root) throw new Error("#root is missing");

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
