import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, errText, type AddRequest, type BatchRequest, type BridgeItem, type Download, type Kind, type ProbeResult, type Settings } from './api';
import * as F from './format';
import { FileBadge } from './icons';

// ---------------------------------------------------------------- primitives

export function Modal({ title, onClose, children, footer, width = 560, className = '' }: {
  title: string; onClose: () => void; children: ReactNode; footer?: ReactNode; width?: number; className?: string;
}) {
  useEffect(() => {
    const k = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', k);
    return () => window.removeEventListener('keydown', k);
  }, [onClose]);
  return (
    <div className="overlay" onMouseDown={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={`modal ${className}`} style={{ width }} role="dialog" aria-label={title}>
        <div className="modal-title">
          <span>{title}</span>
          <button className="x" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  );
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: ReactNode }) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      <div className="field-control">{children}</div>
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  );
}

export function DirInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="dir-input">
      <input value={value} onChange={e => onChange(e.target.value)} spellCheck={false} placeholder="C:\Downloads, \\server\share\dir or smb://host/share/dir" />
      <button type="button" onClick={async () => { const d = await api.chooseDir(value); if (d) onChange(d); }}>…</button>
    </div>
  );
}

export function Toast({ text, onDone }: { text: string; onDone: () => void }) {
  useEffect(() => { const t = setTimeout(onDone, 4500); return () => clearTimeout(t); }, [text, onDone]);
  return <div className="toast" onClick={onDone}>{text}</div>;
}

const joinPath = (dir: string, name: string) => {
  if (!dir) return name;
  const sep = dir.startsWith('smb://') || dir.startsWith('/') ? '/' : '\\';
  return dir.replace(/[\\/]+$/, '') + sep + name;
};

// ---------------------------------------------------------------- Add URL

export function AddUrlDialog({ onClose, onNext }: { onClose: () => void; onNext: (i: BridgeItem & { username?: string; password?: string }) => void }) {
  const [url, setUrl] = useState('');
  const [auth, setAuth] = useState(false);
  const [user, setUser] = useState('');
  const [pass, setPass] = useState('');
  const [adv, setAdv] = useState(false);
  const [referer, setReferer] = useState('');
  const [cookies, setCookies] = useState('');

  useEffect(() => {
    navigator.clipboard?.readText?.().then(t => {
      t = t.trim();
      if (/^(https?|ftps?|mms[ht]?|rtmps?|rtsp):\/\/\S+$/i.test(t)) setUrl(t);
    }).catch(() => { });
  }, []);

  const ok = /^[a-z][a-z0-9+.-]*:\/\/\S+$/i.test(url.trim());
  const submit = () => ok && onNext({ url: url.trim(), referer, cookies, ...(auth ? { username: user, password: pass } : {}) } as BridgeItem);

  return (
    <Modal title="Enter new address to download" onClose={onClose} width={560}
      footer={<><button onClick={onClose}>Cancel</button><button className="primary" disabled={!ok} onClick={submit}>OK</button></>}>
      <Field label="Address">
        <input autoFocus value={url} onChange={e => setUrl(e.target.value)} onKeyDown={e => e.key === 'Enter' && submit()}
          placeholder="https://…, ftp://…, mms://…, .m3u8 / .mpd stream, or a video page" spellCheck={false} />
      </Field>
      <label className="check"><input type="checkbox" checked={auth} onChange={e => setAuth(e.target.checked)} /> Use authorization</label>
      {auth && (
        <div className="grid2">
          <Field label="Login"><input value={user} onChange={e => setUser(e.target.value)} /></Field>
          <Field label="Password"><input type="password" value={pass} onChange={e => setPass(e.target.value)} /></Field>
        </div>
      )}
      <label className="check"><input type="checkbox" checked={adv} onChange={e => setAdv(e.target.checked)} /> Referer and cookies</label>
      {adv && <>
        <Field label="Referer"><input value={referer} onChange={e => setReferer(e.target.value)} spellCheck={false} /></Field>
        <Field label="Cookie header"><input value={cookies} onChange={e => setCookies(e.target.value)} spellCheck={false} placeholder="name=value; other=value" /></Field>
      </>}
    </Modal>
  );
}

// ---------------------------------------------------------------- Download File Info

export function DownloadInfoDialog({ item, settings, onClose, onError }: {
  item: BridgeItem & { username?: string; password?: string }; settings: Settings; onClose: () => void; onError: (m: string) => void;
}) {
  const [probe, setProbe] = useState<ProbeResult | null>(null);
  const [probing, setProbing] = useState(true);
  const [probeErr, setProbeErr] = useState('');
  const [name, setName] = useState(item.fileName ?? '');
  const [dir, setDir] = useState('');
  const [category, setCategory] = useState('');
  const [kind, setKind] = useState<Kind | ''>((item.kind as Kind) || '');
  const [desc, setDesc] = useState('');
  const [queueId, setQueueId] = useState('main');
  const [conns, setConns] = useState(settings.maxConnections);
  const [post, setPost] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    api.probe({ url: item.url, fileName: item.fileName, referer: item.referer, cookies: item.cookies, userAgent: item.userAgent, headers: item.headers, kind: item.kind || '', username: item.username, password: item.password })
      .then(r => {
        if (!alive) return;
        setProbe(r);
        setName(n => n || r.fileName);
        setDir(r.dir); setCategory(r.category); setKind(k => k || r.kind);
      })
      .catch(e => {
        if (!alive) return;
        setProbeErr(errText(e));
        const fallback = item.fileName || decodeURIComponent(item.url.split(/[?#]/)[0].split('/').pop() || 'download');
        setName(n => n || fallback);
        const c = settings.categories.find(c => c.extensions.includes(F.ext(fallback)));
        setDir(c?.dir || settings.defaultDir); setCategory(c?.name || 'General');
      })
      .finally(() => alive && setProbing(false));
    return () => { alive = false; };
  }, [item, settings]);

  const changeCategory = (c: string) => {
    setCategory(c);
    const cat = settings.categories.find(x => x.name === c);
    setDir(cat?.dir || settings.defaultDir);
  };

  const submit = async (startNow: boolean) => {
    setBusy(true);
    const req: AddRequest = {
      url: item.url, fileName: name, dir, category, queueId, kind, referer: item.referer, cookies: item.cookies,
      userAgent: item.userAgent, headers: item.headers, description: desc, connections: conns, postProcess: post,
      username: item.username, password: item.password, startNow, size: probe?.size ?? 0,
    };
    try {
      await api.add(req);
      onClose();
    } catch (e) { onError(errText(e)); setBusy(false); }
  };

  const isStream = kind === 'stream' || kind === 'media';
  return (
    <Modal title="Download File Info" onClose={onClose} width={620}
      footer={<>
        <button onClick={onClose}>Cancel</button>
        <button disabled={busy || !name && kind !== 'media'} onClick={() => submit(false)}>Download Later</button>
        <button className="primary" disabled={busy || !name && kind !== 'media'} onClick={() => submit(true)}>Start Download</button>
      </>}>
      <div className="info-head">
        <FileBadge category={category} kind={kind} name={name} />
        <div className="info-url" title={item.url}>{item.url}</div>
      </div>
      <div className="grid2">
        <Field label="Category">
          <select value={category} onChange={e => changeCategory(e.target.value)}>
            {[...settings.categories.map(c => c.name), 'General'].map(c => <option key={c}>{c}</option>)}
          </select>
        </Field>
        <Field label="Type">
          <select value={kind} onChange={e => setKind(e.target.value as Kind)}>
            <option value="file">File (multi-connection)</option>
            <option value="stream">Stream via ffmpeg (HLS / DASH / MMS / RTMP)</option>
            <option value="media">Video page via yt-dlp</option>
          </select>
        </Field>
      </div>
      <Field label="Save to"><DirInput value={dir} onChange={setDir} /></Field>
      <Field label="File name" hint={kind === 'media' && !name ? 'Leave empty to use the video title' : undefined}>
        <input value={name} onChange={e => setName(e.target.value)} spellCheck={false} />
      </Field>
      <Field label="Description"><input value={desc} onChange={e => setDesc(e.target.value)} /></Field>
      <div className="grid3">
        <Field label="Connections">
          <select value={conns} disabled={isStream} onChange={e => setConns(+e.target.value)}>
            {[1, 2, 4, 6, 8, 12, 16, 24, 32].map(n => <option key={n} value={n}>{n}</option>)}
          </select>
        </Field>
        <Field label="Queue">
          <select value={queueId} onChange={e => setQueueId(e.target.value)}>
            {settings.queues.map(q => <option key={q.id} value={q.id}>{q.name}</option>)}
          </select>
        </Field>
        <Field label="After download">
          <select value={post} onChange={e => setPost(e.target.value)}>
            <option value="">Nothing</option>
            <option value="mp3">Convert to MP3 (ffmpeg)</option>
            <option value="m4a">Convert to M4A (ffmpeg)</option>
            <option value="mp4">Convert to MP4 H.264 (ffmpeg)</option>
          </select>
        </Field>
      </div>
      <div className="probe-line">
        {probing ? <span className="muted"><span className="spinner" /> Retrieving file information…</span>
          : probeErr ? <span className="err">Could not query the server: {probeErr}. You can still add it.</span>
            : probe && <>
              <span>Size: <b>{probe.size > 0 ? F.bytes(probe.size) : 'unknown'}</b></span>
              <span>Resume: <b className={probe.resumable ? 'ok' : 'warn'}>{probe.kind === 'file' ? (probe.resumable ? 'Yes' : 'No') : 'n/a'}</b></span>
              {probe.contentType && <span className="muted">{probe.contentType}</span>}
              <span className="muted ellipsis" title={joinPath(dir, name)}>{joinPath(dir, name)}</span>
            </>}
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------- batch (Download all links)

export function BatchDialog({ req, settings, onClose, onError }: { req: BatchRequest; settings: Settings; onClose: () => void; onError: (m: string) => void }) {
  const items = useMemo(() => {
    const seen = new Set<string>();
    return req.items.filter(i => !seen.has(i.url) && seen.add(i.url));
  }, [req]);
  const nameOf = (i: BridgeItem) => i.fileName || decodeURIComponent(i.url.split(/[?#]/)[0].split('/').pop() || i.url);
  const [checked, setChecked] = useState<Set<string>>(() => new Set(items.filter(i => settings.interceptTypes.includes(F.ext(nameOf(i))) || i.kind === 'stream').map(i => i.url)));
  const [filter, setFilter] = useState('');
  const [dir, setDir] = useState('');
  const [queueId, setQueueId] = useState('main');
  const [startNow, setStartNow] = useState(true);

  const exts = useMemo(() => {
    const m = new Map<string, number>();
    items.forEach(i => { const e = F.ext(nameOf(i)) || '(none)'; m.set(e, (m.get(e) ?? 0) + 1); });
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  }, [items]);

  const shown = items.filter(i => !filter || `${nameOf(i)} ${i.url}`.toLowerCase().includes(filter.toLowerCase()));
  const toggle = (u: string) => setChecked(s => { const n = new Set(s); if (n.has(u)) { n.delete(u); } else { n.add(u); } return n; });
  const byExt = (e: string, on: boolean) => setChecked(s => {
    const n = new Set(s);
    items.forEach(i => { if ((F.ext(nameOf(i)) || '(none)') === e) { if (on) n.add(i.url); else n.delete(i.url); } });
    return n;
  });

  const submit = async () => {
    const reqs: AddRequest[] = items.filter(i => checked.has(i.url)).map(i => ({
      url: i.url, fileName: i.fileName, dir: dir || undefined, queueId, referer: i.referer, cookies: i.cookies,
      userAgent: i.userAgent, headers: i.headers, kind: i.kind || '', description: req.page, startNow: false,
    }));
    try {
      await api.addBatch(reqs);
      if (startNow) await api.startQueue(queueId);
      onClose();
    } catch (e) { onError(errText(e)); }
  };

  return (
    <Modal title={`Download all links${req.page ? ` — ${req.page}` : ''}`} onClose={onClose} width={820} className="tall"
      footer={<>
        <label className="check inline"><input type="checkbox" checked={startNow} onChange={e => setStartNow(e.target.checked)} /> Start queue now</label>
        <span className="grow" />
        <button onClick={onClose}>Cancel</button>
        <button className="primary" disabled={checked.size === 0} onClick={submit}>Add {checked.size} to queue</button>
      </>}>
      <div className="ext-chips">
        {exts.map(([e, n]) => {
          const all = items.filter(i => (F.ext(nameOf(i)) || '(none)') === e).every(i => checked.has(i.url));
          return <button key={e} className={`chip ${all ? 'on' : ''}`} onClick={() => byExt(e, !all)}>{e} <span>{n}</span></button>;
        })}
      </div>
      <div className="grid2">
        <Field label="Filter"><input value={filter} onChange={e => setFilter(e.target.value)} placeholder="Type to filter" /></Field>
        <Field label="Queue">
          <select value={queueId} onChange={e => setQueueId(e.target.value)}>{settings.queues.map(q => <option key={q.id} value={q.id}>{q.name}</option>)}</select>
        </Field>
      </div>
      <Field label="Save to" hint="Leave empty to sort into category folders"><DirInput value={dir} onChange={setDir} /></Field>
      <div className="pick-list">
        <div className="pick-row head">
          <input type="checkbox" checked={shown.length > 0 && shown.every(i => checked.has(i.url))}
            onChange={e => setChecked(s => { const n = new Set(s); shown.forEach(i => e.target.checked ? n.add(i.url) : n.delete(i.url)); return n; })} />
          <span>File</span><span>Address</span>
        </div>
        {shown.map(i => (
          <label key={i.url} className="pick-row">
            <input type="checkbox" checked={checked.has(i.url)} onChange={() => toggle(i.url)} />
            <span className="ellipsis">{nameOf(i)}</span>
            <span className="ellipsis muted">{i.url}</span>
          </label>
        ))}
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------- delete / complete

export function DeleteDialog({ count, onClose, onConfirm }: { count: number; onClose: () => void; onConfirm: (files: boolean) => void }) {
  const [files, setFiles] = useState(false);
  return (
    <Modal title="Confirm deletion" onClose={onClose} width={420}
      footer={<><button onClick={onClose}>Cancel</button><button className="danger" onClick={() => onConfirm(files)}>Delete</button></>}>
      <p>Remove {count === 1 ? 'this download' : `${count} downloads`} from the list?</p>
      <label className="check"><input type="checkbox" checked={files} onChange={e => setFiles(e.target.checked)} /> Also delete the downloaded files from disk</label>
      <p className="muted small">Unfinished part files are always removed.</p>
    </Modal>
  );
}

export function CompleteDialog({ d, onClose, onError }: { d: Download; onClose: () => void; onError: (m: string) => void }) {
  const go = (fn: () => Promise<void>) => fn().then(onClose).catch(e => onError(errText(e)));
  return (
    <Modal title="Download complete" onClose={onClose} width={520}
      footer={<>
        <button onClick={onClose}>Close</button>
        <button onClick={() => go(() => api.openFolder(d.id))}>Open folder</button>
        <button className="primary" onClick={() => go(() => api.openFile(d.id))}>Open</button>
      </>}>
      <div className="complete">
        <FileBadge category={d.category} kind={d.kind} name={d.fileName} />
        <div>
          <div className="complete-name">{d.fileName}</div>
          <div className="muted small">{F.bytes(d.size)} · saved to {d.dir}</div>
          <div className="muted small ellipsis" title={d.url}>{d.url}</div>
        </div>
      </div>
    </Modal>
  );
}
