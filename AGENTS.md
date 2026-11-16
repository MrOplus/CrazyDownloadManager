# AGENTS.md

Guide for AI coding agents (and humans) working on Crazy Download Manager:
an open-source, IDM-style download manager for **Windows and macOS**, written in
**Go 1.27** with a **Wails v2 + React 19 + TypeScript** GUI and a Manifest V3
browser extension. See `README.md` for user-facing features.

## Ground rules

- **Stack is fixed:** Go 1.27 (`go.mod` pins `toolchain go1.27.0`), Wails v2,
  React + TS + Vite. Don't propose Electron, Tauri, or other GUI stacks.
- **No kernel drivers / traffic interception.** Download capture is userland only:
  browser extension, clipboard, drag and drop, command line.
- **Cross-platform.** Everything must build on Windows and macOS. OS-specific code
  goes in `_windows.go` / `_other.go` (or `_darwin.go`) files; see
  `internal/engine/proc_*.go` and `system.go`.
- **Keep the default User-Agent honest** (`config.DefaultUserAgent`). A fake Chrome
  UA with Go's TLS fingerprint gets connections dropped by CDNs.
- **Keep HTTP/2 disabled** in `NewHTTPClient`; it would multiplex every segment onto
  one TCP connection and kill acceleration.
- **Bridge security:** the local API listens only on `127.0.0.1`, requires the
  `X-CDM-Token` header, and answers CORS only for extension origins. Don't weaken this.
- Pure-Go dependencies only (no cgo); builds must work without a C toolchain.

## Layout

```
main.go, app.go, system.go   Wails entry, bound App methods (GUI API), CLI args, OS actions
internal/model               Shared types: Download, Segment, Status, Kind, AddRequest, ProbeResult
internal/config              settings.json: categories, queues, schedule, proxy, SMB creds; Store
internal/engine              Manager (queues, scheduler, limits, persistence, events)
  segmented.go               Dynamic segmentation (split largest busy range; release on conn caps)
  source_http.go / _ftp.go   Source interface: Probe + ranged Open; HTTP(S)+NTLM+proxy, FTP/FTPS
  stream.go                  ffmpeg (HLS/DASH/MMS/RTMP/RTSP), yt-dlp, conversions
  util.go                    file names, kind detection, FindTool (ffmpeg/yt-dlp lookup)
internal/storage             FS abstraction: local / UNC / smb:// (go-smb2)
internal/grabber             Site spider (files mode + mirror mode)
internal/bridge              Local HTTP API for the browser extension
frontend/src                 React UI: App.tsx (main window), dialogs, Options, Scheduler, Grabber
frontend/wailsjs             GENERATED Wails bindings — don't hand-edit; `wails build/dev` regenerates
extension/chromium           MV3 extension (Chrome/Edge/Brave/Opera + Firefox via the same manifest)
tools/genicons               Draws build/appicon.png and the extension icons
```

## Data flow

GUI → bound `App` methods in `app.go` → `engine.Manager`. The manager emits Wails
events (`downloads` every 500 ms, `download:complete`, `download:failed`,
`queue:done`, `grabber:progress`) that the frontend subscribes to in `App.tsx`.
The extension talks to `internal/bridge`, which calls `App.BridgeAdd`.

State lives in the OS config dir (`%APPDATA%\CrazyDownloadManager`,
`~/Library/Application Support/CrazyDownloadManager`): `settings.json` and
`downloads.json` (saved every 3 s and on shutdown). Segment progress is persisted,
so resume must survive a crash or restart. Part files use the `.cdmpart` suffix.

## Commands

```sh
go test ./internal/...        # engine, stream (needs ffmpeg, else skipped), grabber
go vet ./...
wails dev                     # live reload
wails build                   # -> build/bin/CrazyDownloadManager(.exe|.app)
cd frontend && npx tsc --noEmit   # type-check the UI
go run ./tools/genicons       # regenerate icons
```

`wails` is installed in `$(go env GOPATH)/bin`; it may not be on PATH.
No cgo, so `-race` is unavailable on machines without gcc; run tests without it.

## Conventions

- When adding a field to `model.Download` or `config.Settings`, also update the TS
  types in `frontend/src/api.ts` (the generated `wailsjs/go/models.ts` follows on build)
  and handle old saved JSON (zero values must be sensible; migrate in `config.normalize`).
- New GUI-callable functionality = an exported method on `App` in `app.go`, wrapped in
  `frontend/src/api.ts`.
- Engine changes need a test in `internal/engine/engine_test.go` using a local
  `httptest` server (see the existing range/no-range/pause-resume tests).
- UI follows the IDM look: toolbar, category tree, sortable list, modal dialogs.
  Colors are CSS tokens in `style.css` with light and dark variants; use them.
- Keep user-facing strings plain English (no i18n system yet).
- Error messages to the user should say what failed and what to do, not raw Go errors
  where avoidable.

## Verify before claiming done

Run `go vet ./...`, `go test ./internal/...`, and `wails build`. For engine or UI
changes, launch `build/bin/CrazyDownloadManager.exe` and exercise the feature.
The app is single-instance: close it (normal close, so state saves) before rebuilding,
otherwise the exe is locked.

## Known gaps / roadmap

Tray icon (needs Wails v3), multilingual UI, Kerberos SSO, link rewriting in mirror
mode, optional local proxy capture mode, macOS build not yet verified on real hardware.
