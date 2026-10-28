# Remote sync

Status: implemented in M2 (2026-10-28).

## Why

The app could commit locally but had no way to exchange commits with a remote
without a terminal. Fetch, pull and push are the three operations every
repository needs, so they sit on the toolbar next to Refresh.

## UX

- The toolbar gets **Fetch**, **Pull** and **Push** buttons whenever a
  repository is open. They are text buttons like Refresh, not icons: the app
  has no icon set, and the words are unambiguous.
- Pull and Push need a checked-out branch with a commit. On a detached HEAD or
  an unborn branch they are disabled; Fetch stays enabled because it only
  updates remote-tracking refs.
- Only one network operation runs at a time. While it runs the three buttons
  are disabled, the toolbar shows "Fetching…", "Pulling…" or "Pushing…" next
  to the branch chip, and the commit button is disabled too: a commit racing a
  pull could capture a worktree that is halfway rewritten.
- Push publishes the current branch to its upstream. There is no force push
  and no refspec guessing: a branch without an upstream fails with git's own
  message, which tells the user what to run. The command pins
  `push.default=upstream` and `push.autoSetupRemote=false`, so a repository
  configured for matching pushes cannot publish every same-named branch from
  one click, and a missing upstream is never created silently.
- One operation is bounded to five minutes and is cancelled when another
  repository is opened. A credential helper that hangs, or a network that
  never answers, cannot leave the app with writes disabled forever.
- Pull fast-forwards the current branch only (`git pull --ff-only`). That is
  the one strategy the app can carry out without an editor or a strategy
  prompt: it never creates a merge commit and never leaves a conflicted state
  the app has no way to continue. A diverged branch is refused with git's
  "Not possible to fast-forward" message; rebasing or merging a divergence
  stays in the terminal.
- Fetch prunes remote-tracking refs the remote deleted, so a deleted branch
  disappears from History instead of lingering.
- Errors are toasts carrying git's message: no remote, no upstream, missing
  credentials, no network, a diverged branch. The repository is refreshed
  either way: a pull fetches before it fast-forwards, so its fetch half can
  have moved remote-tracking refs before the merge half refused.
- A successful operation has no toast. The branch chip's `↑N ↓M` counts and
  the refreshed history are the feedback, which keeps the window quiet.
- Nothing prompts in a terminal: `gitx` already runs git with
  `GIT_TERMINAL_PROMPT=0`, so a missing credential fails with git's error
  instead of hanging; credential helpers and SSH agents still work.

## Data flow

```
RepoService.fetch() -> gitx.Fetch -> git fetch --prune
RepoService.pull()  -> gitx.Pull  -> git pull --ff-only
RepoService.push()  -> gitx.Push  -> git push
```

`internal/shell.App` keeps one sync slot (`syncing`, `syncOp`, `syncCancel`)
beside `writing`. A sync refuses to start while a worktree write is in flight,
and `write` refuses to start while a sync is running; the stage, discard,
checkout and branch buttons disable through the same `busy()` check, so a click
cannot silently do nothing. The git process runs with a five-minute context
timeout. A per-slot epoch (like the status and diff epochs) drops a completion
that arrives after the user opened another repository; `resetRepoState` cancels
the process, bumps the epoch and clears the slot, so the buttons cannot stay
disabled on the new repository and a new operation cannot race the old one.

On completion the app reloads the status, the loaded commit page and the branch
list. The selected commit and file stay selected: they are read by object id,
so the same commit still resolves after the refresh.

## Git commands

| Call | Command |
|---|---|
| Fetch | `git fetch --prune` |
| Pull | `git pull --ff-only` |
| Push | `git -c push.default=upstream -c push.autoSetupRemote=false push` |

None of the three opens an editor, so none can hang on a terminal it does not
have, and the context timeout bounds a hung credential helper.

## Edge cases

- **No remote configured**: `git fetch` / `git pull` fail with git's message;
  nothing changes.
- **No upstream**: `git push` fails with git's "has no upstream branch" message
  instead of guessing `origin/<branch>` or letting `push.autoSetupRemote`
  create one.
- **Matching push configuration**: `push.default=matching` in the repository
  cannot turn one click into a push of every same-named branch.
- **Diverged branches**: pull refuses and the toast carries git's
  "Not possible to fast-forward" message; no merge commit, no conflicted
  worktree, nothing the app cannot undo.
- **Fast-forward pull**: changes arrive in the worktree and the counts drop;
  the selected commit and file survive because they are read by object id.
- **Long operations**: fetch, pull and push run in goroutines; the window keeps
  drawing while the network is slow, and the five-minute timeout ends a call
  that will not finish.
- **Repository switched mid-operation**: the process is cancelled, and the
  epoch and the root check drop its completion; the new repository's state is
  not refreshed by the old call.
- **Credentials**: a helper that needs a terminal fails fast rather than
  blocking the goroutine forever; `GIT_TERMINAL_PROMPT=0` is part of the
  runner's environment, and the timeout is the backstop for a helper that waits
  anyway.
- **Unborn branch / detached HEAD**: pull and push are disabled, fetch is not.
