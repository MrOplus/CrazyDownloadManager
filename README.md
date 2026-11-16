# Crazy Download Manager

An open-source, cross-platform (Windows and macOS) download accelerator with an
Internet Download Manager–style interface, written in **Go 1.27** with a
**Wails v2 + React** GUI.

* Multi-connection downloads with **dynamic segmentation** over HTTP, HTTPS, FTP and FTPS
* **Streams through ffmpeg**: HLS (`.m3u8`), DASH (`.mpd`), **MMS** (`mms://`, `mmsh://`, `mmst://`), RTMP, RTSP, SRT
* Video pages (YouTube, Vimeo, …) through **yt-dlp**
* Save straight to **SMB shares** (`smb://nas/share/dir`), UNC paths, or mounted volumes
* Browser extension for Chrome, Edge, Brave, Opera and Firefox: download takeover, a "Download this video" panel, and "Download all links"
* **Site grabber**: crawl a site for files of chosen types, or mirror it for offline browsing
* Queues, a scheduler, speed limiter, categories, resume after crash or reboot, clipboard monitor, IDM-compatible command line

## Feature parity with IDM

| IDM feature (features2.html) | Crazy Download Manager |
|---|---|
| Download acceleration, dynamic file segmentation, connection reuse | ✅ `internal/engine/segmented.go`: each new or idle connection takes the second half of the largest remaining range. Default 8 connections, up to 32 |
| One-click download from browsers | ✅ MV3 extension: Chrome, Edge, Brave, Opera, Vivaldi, Firefox 121+ |
| "Download with …" context menu | ✅ Link, image, video and audio context menu |
| HTTP, HTTPS, FTP, MMS | ✅ HTTP/HTTPS/FTP/FTPS natively. MMS, plus HLS/DASH/RTMP/RTSP, via ffmpeg |
| Resume / error recovery | ✅ Per-range progress saved every 3 s and at shutdown; retries; per-connection stall watchdog |
| Download All (links on page) | ✅ Context menu and popup button, with an extension filter dialog |
| Scheduler, queues, shutdown when done | ✅ Multiple queues, start and stop times, weekdays, exit/sleep/shut down when done |
| Site spider / grabber | ✅ File-type mode and whole-site mirror mode, depth, host and path limits, exclude patterns |
| Video grabber ("Download this video" panel) | ✅ Media sniffing in the extension, in-page panel, popup list, yt-dlp for video sites |
| Download limits / speed limiter | ✅ Global limit and per-download limit (live, from the progress window) |
| Automatic categories | ✅ By extension, each category with its own folder |
| Proxy servers | ✅ System, none, or manual HTTP/HTTPS/SOCKS5 with authentication |
| Basic, NTLM, Negotiate authentication | ✅ via `go-ntlmssp` (Kerberos SSO not implemented) |
| Drag and drop links | ✅ Drop links from a browser onto the window |
| Antivirus check on completion | ✅ Any scanner command (`%f` = file path) |
| Dark theme / customizable UI | ✅ Light, dark, or follow the system |
| Command line | ✅ `CrazyDownloadManager /d URL /p dir /f name /n /a /s` (IDM's switches) |
| Clipboard monitoring | ✅ Optional |
| Advanced browser integration (catch downloads from *any* application) | ⚠️ Not implemented. Capture is userland only: browser extension, clipboard, drag and drop, command line |
| Multilingual UI | ⏳ Not yet (strings are in English) |
| Tray icon | ⏳ Not yet (Wails v2 has no tray API; planned with Wails v3) |

## Architecture

```
┌────────────── Browser ──────────────┐        ┌────────── CrazyDownloadManager.exe / .app ──────────┐
│ extension/chromium (MV3)            │  HTTP  │ internal/bridge   127.0.0.1:47100 + X-CDM-Token     │
│  • downloads.onDeterminingFilename ─┼───────▶│ app.go            Wails bindings, CLI, clipboard    │
│  • webRequest media sniffing        │        │ internal/engine   Manager: queues, scheduler,       │
│  • context menus / Download all     │        │                   speed limits, persistence         │
│  • in-page video panel              │        │   ├ segmented.go  dynamic segmentation (HTTP, FTP)  │
└─────────────────────────────────────┘        │   ├ stream.go     ffmpeg (HLS/DASH/MMS/RTMP/RTSP)   │
                                               │   │               yt-dlp (video pages), conversions │
┌──── frontend (React + TS) ──────────┐ events │   └ source_*.go   HTTP(S)+NTLM+proxy, FTP/FTPS      │
│ IDM-style toolbar, categories, list,│◀──────▶│ internal/storage  local / UNC / smb:// (SMB2/3)     │
│ progress window, options, scheduler,│        │ internal/grabber  site spider                       │
│ site grabber                        │        │ internal/config   settings.json                     │
└─────────────────────────────────────┘        └─────────────────────────────────────────────────────┘
```

Some design notes:

* **HTTP/2 is disabled on purpose.** It multiplexes every request to one TCP
  connection, which defeats multi-connection acceleration.
* **Part files are sparse on Windows** (`FSCTL_SET_SPARSE`). Without that, NTFS
  zero-fills the gap when a segment writes far into the file. APFS creates
  sparse files on its own.
* **The connection count adapts.** If a server caps connections per client, the
  extra connections hand their ranges back instead of failing the download.
* **The default User-Agent is honest** (`CrazyDownloadManager/x`). Some CDNs
  drop connections that claim to be Chrome but have Go's TLS fingerprint. When
  a browser-supplied UA is refused this way, the engine retries once with its
  own UA.
* **Stopping a live stream finalises the file.** ffmpeg gets a clean `q`, so the
  recording stays playable.
* **SMB:** `smb://` destinations write directly over SMB2/3 using
  [`go-smb2`](https://github.com/hirochachacha/go-smb2), with credentials from
  *Options → Network shares*. ffmpeg and yt-dlp output goes to a local temp
  folder first and is uploaded when done.

Data lives in `%APPDATA%\CrazyDownloadManager` (Windows) or
`~/Library/Application Support/CrazyDownloadManager` (macOS).

## Building

Requirements: Go 1.27 (any Go ≥ 1.21 fetches it automatically, because `go.mod`
pins `toolchain go1.27.0`), Node 20+, and the Wails CLI:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor            # checks WebView2 (Windows) / Xcode CLT (macOS)

wails dev               # live-reload development
wails build             # → build/bin/CrazyDownloadManager(.exe|.app)
wails build -nsis       # Windows installer (needs NSIS)
wails build -platform darwin/universal   # on a Mac: Intel + Apple Silicon
```

Optional runtime tools (auto-detected on `PATH`, next to the executable, in
`bin/`, or Homebrew paths):

* **ffmpeg**: streams (HLS/DASH/MMS/RTMP/RTSP) and conversions. `winget install ffmpeg` / `brew install ffmpeg`
* **yt-dlp**: video sites. `winget install yt-dlp` / `brew install yt-dlp`

Tests (local HTTP servers; the HLS test generates a stream with ffmpeg, if installed):

```sh
go test ./internal/...
```

To regenerate icons: `go run ./tools/genicons`

## Installing the browser extension

1. Open `chrome://extensions` (or `edge://extensions`, `brave://extensions`)
   and turn on **Developer mode**.
2. Click **Load unpacked** and choose `extension/chromium`.
3. In the extension options, paste the token from **Options → Browser** in the
   app and click **Test connection**.

Firefox: `about:debugging` → *This Firefox* → *Load Temporary Add-on* →
`extension/chromium/manifest.json`. For permanent install, sign it through
addons.mozilla.org.

Usage:
* Clicking a download link whose type is in *Options → File types* sends it to
  the app. Hold **Alt** while clicking to let the browser handle it.
* Right-click → **Download with Crazy DM** / **Download all links with Crazy DM**.
* Play a video, then use the floating **Download this video** button or the
  toolbar popup.

**Bridge security:** the app listens only on `127.0.0.1`. Every request needs
the pairing token, and requests from web-page origins get `403 Forbidden`.

## Command line

```
CrazyDownloadManager /d URL [/p local\path] [/f filename] [/n] [/a] [/s]
  /d URL   download URL (a bare URL works too)
  /p dir   save to this folder (local, UNC or smb://)
  /f name  save as this file name
  /n       silent: no dialog, start immediately
  /a       add to the main queue, don't start
  /s       start the main queue
```

If the app is already running, the arguments are handed to that instance.

## License

MIT
