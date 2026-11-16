import { useState } from 'react';
import { api, errText, type Info, type Settings } from './api';
import { DirInput, Field, Modal } from './dialogs';

const tabs = [
  ['general', 'General'], ['browser', 'Browser'], ['types', 'File types'], ['save', 'Save to'],
  ['connection', 'Connection'], ['proxy', 'Proxy / Auth'], ['smb', 'Network shares'], ['media', 'Media (ffmpeg)'],
] as const;

export function OptionsDialog({ settings, info, initialTab, onClose, onSaved, onError }: {
  settings: Settings; info: Info; initialTab?: string; onClose: () => void; onSaved: () => void; onError: (m: string) => void;
}) {
  const [s, setS] = useState<Settings>(() => structuredClone(settings));
  const [tab, setTab] = useState(initialTab ?? 'general');
  const [smbTest, setSmbTest] = useState('');
  const [smbResult, setSmbResult] = useState('');
  const set = <K extends keyof Settings>(k: K, v: Settings[K]) => setS(x => ({ ...x, [k]: v }));
  const words = (v: string) => v.split(/[\s,;]+/).map(x => x.trim().replace(/^\./, '')).filter(Boolean);

  const save = async () => {
    try { await api.saveSettings(s); onSaved(); } catch (e) { onError(errText(e)); }
  };

  const proxyMode = s.proxy === '' || s.proxy === 'system' ? 'system' : s.proxy === 'direct' ? 'direct' : 'manual';

  return (
    <Modal title="Configuration" onClose={onClose} width={760} className="tall"
      footer={<><button onClick={onClose}>Cancel</button><button className="primary" onClick={save}>OK</button></>}>
      <div className="tabs">
        {tabs.map(([k, l]) => <button key={k} className={tab === k ? 'on' : ''} onClick={() => setTab(k)}>{l}</button>)}
      </div>
      <div className="tab-body">
        {tab === 'general' && <>
          <Field label="Theme">
            <select value={s.theme} onChange={e => set('theme', e.target.value as Settings['theme'])}>
              <option value="system">Follow system</option><option value="light">Light</option><option value="dark">Dark</option>
            </select>
          </Field>
          <label className="check"><input type="checkbox" checked={s.showAddDialog} onChange={e => set('showAddDialog', e.target.checked)} /> Show "Download File Info" dialog for browser downloads</label>
          <label className="check"><input type="checkbox" checked={s.showCompleteDialog} onChange={e => set('showCompleteDialog', e.target.checked)} /> Show "Download complete" dialog</label>
          <label className="check"><input type="checkbox" checked={s.clipboardMonitor} onChange={e => set('clipboardMonitor', e.target.checked)} /> Watch the clipboard for download links</label>
          <Field label="Virus scanner" hint={<>Runs after each download. <code>%f</code> is replaced by the file path, e.g. <code>"C:\Program Files\Windows Defender\MpCmdRun.exe" -Scan -ScanType 3 -File "%f"</code></>}>
            <div className="dir-input">
              <input value={s.antivirusCmd} onChange={e => set('antivirusCmd', e.target.value)} spellCheck={false} />
              <button onClick={async () => { const f = await api.chooseFile('Choose scanner'); if (f) set('antivirusCmd', `"${f}" "%f"`); }}>…</button>
            </div>
          </Field>
          <p className="muted small">Settings folder: {info.configDir}</p>
          <p className="muted small">Command line: <code>CrazyDownloadManager /d URL [/p dir] [/f name] [/n] [/a] [/s]</code> (same switches as IDM; /n = silent, /a = add to queue only, /s = start main queue)</p>
        </>}

        {tab === 'browser' && <>
          <label className="check"><input type="checkbox" checked={s.browserIntercept} onChange={e => set('browserIntercept', e.target.checked)} /> Take over downloads from the browser</label>
          <div className="grid2">
            <Field label="Bridge port" hint="Local only (127.0.0.1)"><input type="number" value={s.bridgePort} onChange={e => set('bridgePort', +e.target.value)} /></Field>
            <Field label="Extension pairing token">
              <div className="dir-input">
                <input readOnly value={s.bridgeToken} onFocus={e => e.target.select()} />
                <button title="Copy" onClick={() => api.copy(s.bridgeToken)}>Copy</button>
                <button title="Generate a new token" onClick={async () => set('bridgeToken', await api.regenerateToken())}>New</button>
              </div>
            </Field>
          </div>
          <Field label="Do not take over downloads from these sites"><textarea rows={3} value={s.skipSites.join('\n')} onChange={e => set('skipSites', e.target.value.split(/\n/).map(x => x.trim()).filter(Boolean))} placeholder="*.update.microsoft.com&#10;accounts.google.com" /></Field>
          <div className="howto">
            <b>Install the browser extension</b>
            <ol>
              <li>Open <code>chrome://extensions</code> (or <code>edge://extensions</code>, <code>brave://extensions</code>, <code>opera://extensions</code>) and turn on <i>Developer mode</i>.</li>
              <li>Click <i>Load unpacked</i> and choose the <code>extension/chromium</code> folder that ships with Crazy Download Manager.</li>
              <li>Open the extension's options, paste the token above and press <i>Test connection</i>.</li>
            </ol>
            <p className="muted small">Firefox: open <code>about:debugging</code> → <i>This Firefox</i> → <i>Load Temporary Add-on</i> and pick <code>extension/chromium/manifest.json</code>.</p>
          </div>
        </>}

        {tab === 'types' && <>
          <Field label="Take over these file types from the browser" hint="Space separated extensions. Streams (.m3u8, .mpd) and video pages are handled by the extension's video panel.">
            <textarea rows={6} value={s.interceptTypes.join(' ')} onChange={e => set('interceptTypes', words(e.target.value))} />
          </Field>
          <button onClick={() => set('interceptTypes', ['3gp', '7z', 'aac', 'apk', 'avi', 'bin', 'bz2', 'deb', 'dmg', 'exe', 'flac', 'flv', 'gz', 'iso', 'm4a', 'm4v', 'mkv', 'mov', 'mp3', 'mp4', 'mpeg', 'mpg', 'msi', 'ogg', 'pdf', 'pkg', 'rar', 'rpm', 'tar', 'tgz', 'wav', 'webm', 'wma', 'wmv', 'xz', 'zip', 'zst'])}>Restore defaults</button>
        </>}

        {tab === 'save' && <>
          <Field label="Default download folder"><DirInput value={s.defaultDir} onChange={v => set('defaultDir', v)} /></Field>
          <div className="section-title">Categories</div>
          <div className="cat-table">
            {s.categories.map((c, i) => (
              <div key={i} className="cat-row">
                <input value={c.name} onChange={e => set('categories', s.categories.map((x, k) => k === i ? { ...x, name: e.target.value } : x))} />
                <input value={c.extensions.join(' ')} title="File extensions" onChange={e => set('categories', s.categories.map((x, k) => k === i ? { ...x, extensions: words(e.target.value) } : x))} />
                <DirInput value={c.dir} onChange={v => set('categories', s.categories.map((x, k) => k === i ? { ...x, dir: v } : x))} />
                <button className="icon" title="Remove" onClick={() => set('categories', s.categories.filter((_, k) => k !== i))}>✕</button>
              </div>
            ))}
          </div>
          <button onClick={() => set('categories', [...s.categories, { name: 'New category', extensions: [], dir: s.defaultDir }])}>Add category</button>
          <p className="muted small">Folders can be local, UNC (<code>\\nas\share\Downloads</code>), a mounted share (<code>/Volumes/Share</code>), or <code>smb://nas/share/Downloads</code> to write directly over SMB2/3 with the credentials from the <i>Network shares</i> tab.</p>
        </>}

        {tab === 'connection' && <>
          <div className="grid2">
            <Field label="Max. connections per download" hint="Dynamic segmentation splits the file across this many connections">
              <select value={s.maxConnections} onChange={e => set('maxConnections', +e.target.value)}>
                {[1, 2, 4, 6, 8, 12, 16, 24, 32].map(n => <option key={n} value={n}>{n}</option>)}
              </select>
            </Field>
            <Field label="Global speed limit (KB/s)" hint="0 = unlimited"><input type="number" min={0} value={s.globalSpeedLimit} onChange={e => set('globalSpeedLimit', Math.max(0, +e.target.value))} /></Field>
            <Field label="Timeout (seconds)"><input type="number" min={5} value={s.timeoutSec} onChange={e => set('timeoutSec', +e.target.value)} /></Field>
            <Field label="Retries per connection"><input type="number" min={0} value={s.maxRetries} onChange={e => set('maxRetries', +e.target.value)} /></Field>
            <Field label="Retry delay (seconds)"><input type="number" min={0} value={s.retryDelaySec} onChange={e => set('retryDelaySec', +e.target.value)} /></Field>
          </div>
          <Field label="User agent"><input value={s.userAgent} onChange={e => set('userAgent', e.target.value)} spellCheck={false} /></Field>
        </>}

        {tab === 'proxy' && <>
          <label className="check"><input type="radio" checked={proxyMode === 'system'} onChange={() => set('proxy', 'system')} /> Use system proxy (HTTP_PROXY / HTTPS_PROXY / NO_PROXY)</label>
          <label className="check"><input type="radio" checked={proxyMode === 'direct'} onChange={() => set('proxy', 'direct')} /> No proxy</label>
          <label className="check"><input type="radio" checked={proxyMode === 'manual'} onChange={() => set('proxy', 'http://127.0.0.1:8080')} /> Manual proxy</label>
          {proxyMode === 'manual' && <Field label="Proxy URL" hint="http://, https:// or socks5:// — include user:password@ if required"><input value={s.proxy} onChange={e => set('proxy', e.target.value)} spellCheck={false} /></Field>}
          <p className="muted small">Server authentication: Basic, NTLM and Negotiate are handled automatically when you enter a login in the Add URL dialog or put it in the URL (<code>https://user:pass@host/file</code>, <code>ftp://user:pass@host/file</code>).</p>
        </>}

        {tab === 'smb' && <>
          <p className="muted small">Credentials used for <code>smb://host/share/folder</code> destinations. Windows UNC paths and mounted macOS shares use the OS login instead and need nothing here.</p>
          <div className="cat-table">
            {s.smb.map((c, i) => (
              <div key={i} className="smb-row">
                <input placeholder="host / IP" value={c.host} onChange={e => set('smb', s.smb.map((x, k) => k === i ? { ...x, host: e.target.value } : x))} />
                <input placeholder="domain" value={c.domain} onChange={e => set('smb', s.smb.map((x, k) => k === i ? { ...x, domain: e.target.value } : x))} />
                <input placeholder="user" value={c.user} onChange={e => set('smb', s.smb.map((x, k) => k === i ? { ...x, user: e.target.value } : x))} />
                <input placeholder="password" type="password" value={c.password} onChange={e => set('smb', s.smb.map((x, k) => k === i ? { ...x, password: e.target.value } : x))} />
                <input placeholder="445" type="number" value={c.port || ''} onChange={e => set('smb', s.smb.map((x, k) => k === i ? { ...x, port: +e.target.value } : x))} />
                <button className="icon" onClick={() => set('smb', s.smb.filter((_, k) => k !== i))}>✕</button>
              </div>
            ))}
          </div>
          <button onClick={() => set('smb', [...s.smb, { host: '', user: '', password: '', domain: '', port: 0 }])}>Add server</button>
          <div className="section-title">Test a destination</div>
          <div className="dir-input">
            <input placeholder="smb://nas/share/Downloads" value={smbTest} onChange={e => setSmbTest(e.target.value)} />
            <button onClick={async () => {
              setSmbResult('Testing… (save first if you changed credentials)');
              try { await api.saveSettings(s); await api.testSMB(smbTest); setSmbResult('✔ Share is reachable and writable'); }
              catch (e) { setSmbResult('✖ ' + errText(e)); }
            }}>Test</button>
          </div>
          {smbResult && <p className="small">{smbResult}</p>}
        </>}

        {tab === 'media' && <>
          <Field label="ffmpeg" hint={info.ffmpeg ? `Found: ${info.ffmpeg}` : 'Not found. Install ffmpeg (winget install ffmpeg / brew install ffmpeg) or point to ffmpeg.exe. Needed for HLS, DASH, MMS, RTMP, RTSP and conversions.'}>
            <div className="dir-input">
              <input value={s.ffmpegPath} placeholder="auto-detect" onChange={e => set('ffmpegPath', e.target.value)} />
              <button onClick={async () => { const f = await api.chooseFile('Locate ffmpeg'); if (f) set('ffmpegPath', f); }}>…</button>
            </div>
          </Field>
          <Field label="yt-dlp (optional)" hint={info.ytdlp ? `Found: ${info.ytdlp}` : 'Not found. Install yt-dlp to download from video pages (YouTube, Vimeo, …).'}>
            <div className="dir-input">
              <input value={s.ytDlpPath} placeholder="auto-detect" onChange={e => set('ytDlpPath', e.target.value)} />
              <button onClick={async () => { const f = await api.chooseFile('Locate yt-dlp'); if (f) set('ytDlpPath', f); }}>…</button>
            </div>
          </Field>
          <p className="muted small">Streams are recorded with <code>-c copy</code> (no re-encoding). Stopping a live stream finalises the file so it stays playable. MMS links are tried over MMSH first, then MMST.</p>
        </>}
      </div>
    </Modal>
  );
}
