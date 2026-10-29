# Twill

A Git history and diff viewer in the spirit of IDEA's diff viewer. The window
is drawn natively from Go with `github.com/egoist/mygo/ui`, and every
repository operation runs in the same process: no web view, no TypeScript
bindings. The name comes from twill, a weave whose diagonal threads run like
the lanes of the commit graph.

Status: feature-complete for daily use — browse changes, stage/unstage/discard
whole files and single hunks, commit, read the repository history with a
colored lane graph, branch and tag labels, changed files and per-file patches,
switch/create/delete local branches and check out remote ones, and reopen the
last repository on launch.
Stash, conflict editing and a pull strategy come next.

## Requirements

- Go 1.27+
- Bun (only to run the `mygo-cli` build wrapper)
- `git` 2.25+ on `PATH`

## Run

```bash
bun install
bun run dev          # mygo dev: builds and starts the native window
bun run build        # mygo build: signed .app / .dmg
```

The packaged binary opens a repository given as its first argument:

```bash
build/darwin-arm64/Twill.app/Contents/MacOS/Twill ~/code/some-repo
```

## Layout

| Path | Role |
|---|---|
| `main.go` | window, native menu, file drops, CLI argument |
| `internal/gitx` | the only package that starts `git`; porcelain v2 parsing, diffs, hunk apply, worktree writes |
| `internal/service` | repository actions, change watcher, last-repository state, menu actions |
| `internal/shell` | native UI state, async loaders, view functions, commit graph and patch parser |
| `assets/icon.svg` | app icon: three diagonal lanes woven as twill; `resources/icon.png` is its rasterization |
| `docs/native-ui.md` | native UI migration plan, verification notes and known gaps |
| `docs/history.md` | history view design: commands, state, edge cases |
| `docs/branches.md` | branch switcher design: commands, actions, edge cases |
| `docs/git-ux.md` | IDEA-style UX plan: icons, branch popover and trees by batch |
| `docs/stash.md` | stash tab: commands, apply/pop/drop, edge cases |
| `docs/keyboard.md` | list keys: arrows, Space to stage, Cmd+F, edge cases |
| `docs/session.md` | last-repository restore: state file, atomic write, edge cases |
| `package.json` | only `mygo-cli`; the app has no JavaScript dependencies |

## Tests

```bash
go test ./...
```

`internal/gitx` tests use real temporary repositories for the apply paths, so
partial staging, unstaging and discarding are checked against git itself.
`internal/shell` tests drive the view through `ui.NewTester`.
