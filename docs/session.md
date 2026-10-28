# Restoring the last repository

Status: implemented in M2 (2026-10-28).

## Why

Every launch started with an empty window, even when the same project was open
a second earlier. The app now reopens the last repository on start.

## How

- A successful open — picker, file drop, or command-line argument — writes the
  resolved worktree top level to `state.json` in MyGo's per-app user-data
  directory (`mygo.App.Path(mygo.PathUserData)`), as
  `{"lastRepo":"/absolute/path"}`. In a packaged build on macOS that is
  `~/Library/Application Support/Twill/state.json`; the dev app has its own
  name, so development state does not mix with the installed app.
- `service.OpenInitial` is the one startup entry point. The command-line
  argument wins; without one, the saved path is opened silently and the page
  learns about it through the usual `repo-changed` event or its first status
  read.
- The file is written atomically: a temporary file in the same directory, then
  `rename`. A crash mid-write leaves the previous state file intact instead of
  a truncated one.
- Read failures, a missing file and invalid JSON all mean "no saved
  repository"; they are not surfaced to the user.

## Edge cases

- **Dropped file**: the repository root is saved, not the dropped file's path;
  the same root is what `Discover` returns.
- **Canceled picker**: nothing is written, so the previous repository is still
  restored next time.
- **Path gone or no longer a repository**: the restore logs and the app starts
  empty. No dialog appears, so an unmounted volume cannot block startup.
- **Unwritable config directory**: saving logs and fails silently; opening the
  repository still works for the session.
- **Two launches**: the last successful open wins; there is one window and one
  state file.
- **CLI argument**: it is opened and saved like any other repository, so the
  next launch without arguments returns to it.

## Not in this change

A recent-repositories menu, and remembering the selected tab or diff per
repository. Window bounds are already persisted by MyGo's `StateKey`.
