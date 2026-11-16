# CLAUDE.md

@AGENTS.md

## Claude Code notes

- Shell is Windows (PowerShell / Git Bash). Use `$(go env GOPATH)/bin/wails` if `wails`
  isn't on PATH.
- Commits: plain messages, **no `Co-Authored-By: Claude` or "Generated with Claude Code"
  trailers** in commits or PRs.
- Don't commit build output (`build/bin`, `frontend/dist`) or anything from the user's
  config dir (it contains the bridge token).
- When you test real downloads, delete the downloaded files and test entries afterwards.
