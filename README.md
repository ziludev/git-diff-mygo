# Git Diff (MyGo)

A Git client in the spirit of IDEA's diff viewer: the window is a React page in
the OS webview, and every repository operation runs in Go through MyGo bindings.

Status: M0 — browse changes, stage/unstage/discard whole files and single
hunks, and commit. History, branches and the commit graph come next.

## Requirements

- Go 1.27+
- Bun
- `git` 2.25+ on `PATH`

## Run

```bash
bun install
bun run dev          # mygo dev: Vite dev server + native window
bun run build        # mygo build: signed .app / .dmg with embedded assets
```

The packaged binary opens a repository given as its first argument:

```bash
build/darwin-arm64/git-diff-mygo.app/Contents/MacOS/git-diff-mygo ~/code/some-repo
```

## Layout

| Path | Role |
|---|---|
| `main.go` | window, native menu, file drops, CLI argument |
| `internal/gitx` | the only package that starts `git`; porcelain v2 parsing, diffs, hunk apply, worktree writes |
| `internal/service` | bound `RepoService` for the page, change watcher, menu actions |
| `src/` | React UI: Zustand store, changes panel, diff panel, shadcn components |
| `src/mygo.ts` | generated TypeScript client — run `bun run generate` after changing bound methods |

## Tests

```bash
go test ./...
bun run typecheck
```

`internal/gitx` tests use real temporary repositories for the apply paths, so
partial staging, unstaging and discarding are checked against git itself.
