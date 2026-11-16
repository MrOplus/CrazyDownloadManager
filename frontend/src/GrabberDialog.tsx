import { useEffect, useMemo, useState } from 'react';
import { api, errText, on, type GrabItem, type GrabOptions, type GrabProgress, type Settings } from './api';
import { DirInput, Field, Modal } from './dialogs';

const presets: Record<string, string> = {
  Pictures: '*.jpg *.jpeg *.png *.gif *.webp *.bmp *.svg *.avif',
  Audio: '*.mp3 *.m4a *.flac *.ogg *.wav *.aac *.wma',
  Video: '*.mp4 *.mkv *.webm *.avi *.mov *.wmv *.flv',
  Documents: '*.pdf *.doc *.docx *.xls *.xlsx *.ppt *.pptx *.txt *.epub',
  Archives: '*.zip *.rar *.7z *.gz *.tar *.iso',
  'All files': '',
};

// Site grabber wizard: 1) start page, 2) what to grab, 3) scan and pick.
export function GrabberDialog({ settings, onClose, onError, onDone }: { settings: Settings; onClose: () => void; onError: (m: string) => void; onDone: (n: number) => void }) {
  const [step, setStep] = useState(1);
  const [opt, setOpt] = useState<GrabOptions>({
    startUrl: '', mode: 'files', depth: 1, sameHost: true, stayBelow: false, include: presets.Pictures.split(' '),
    exclude: [], maxPages: 300, concurrency: 4, cookies: '', userAgent: '', username: '', password: '',
  });
  const [dir, setDir] = useState(settings.defaultDir.replace(/[\\/]+$/, '') + (settings.defaultDir.startsWith('/') ? '/Grabber' : '\\Grabber'));
  const [preset, setPreset] = useState('Pictures');
  const [scanning, setScanning] = useState(false);
  const [progress, setProgress] = useState<GrabProgress | null>(null);
  const [items, setItems] = useState<GrabItem[]>([]);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [queueId, setQueueId] = useState('main');
  const [startNow, setStartNow] = useState(true);
  const [filter, setFilter] = useState('');

  useEffect(() => on<GrabProgress>('grabber:progress', setProgress), []);
  const set = <K extends keyof GrabOptions>(k: K, v: GrabOptions[K]) => setOpt(o => ({ ...o, [k]: v }));
  const validUrl = /^https?:\/\/\S+$/i.test(opt.startUrl.trim());

  const scan = async () => {
    setStep(3); setScanning(true); setItems([]); setProgress(null);
    try {
      const r = await api.grabScan(opt);
      setItems(r ?? []);
      setChecked(new Set((r ?? []).map(i => i.url)));
    } catch (e) { onError(errText(e)); }
    setScanning(false);
  };

  const add = async () => {
    try {
      const n = await api.grabAdd({ items: items.filter(i => checked.has(i.url)), dir, queueId, startNow, cookies: opt.cookies, username: opt.username, password: opt.password });
      onDone(n); onClose();
    } catch (e) { onError(errText(e)); }
  };

  const shown = useMemo(() => items.filter(i => !filter || i.url.toLowerCase().includes(filter.toLowerCase())), [items, filter]);
  const close = () => { if (scanning) api.grabCancel(); onClose(); };

  return (
    <Modal title={`Site Grabber — step ${step} of 3`} onClose={close} width={780} className="tall"
      footer={<>
        {step > 1 && !scanning && <button onClick={() => setStep(step - 1)}>&lt; Back</button>}
        <span className="grow" />
        <button onClick={close}>Cancel</button>
        {step === 1 && <button className="primary" disabled={!validUrl} onClick={() => setStep(2)}>Next &gt;</button>}
        {step === 2 && <button className="primary" onClick={scan}>Scan site</button>}
        {step === 3 && scanning && <button onClick={() => api.grabCancel()}>Stop scanning</button>}
        {step === 3 && !scanning && <button className="primary" disabled={checked.size === 0} onClick={add}>Add {checked.size} files</button>}
      </>}>
      {step === 1 && <>
        <p className="muted">The grabber explores a web site from a start page and collects files for you — all pictures of a gallery, every PDF of a documentation site, or a whole site for offline browsing.</p>
        <Field label="Start page"><input autoFocus value={opt.startUrl} onChange={e => set('startUrl', e.target.value)} placeholder="https://example.com/gallery/" spellCheck={false} /></Field>
        <Field label="Save files to (project folder)"><DirInput value={dir} onChange={setDir} /></Field>
        <div className="grid2">
          <Field label="Login (if the site needs it)"><input value={opt.username} onChange={e => set('username', e.target.value)} /></Field>
          <Field label="Password"><input type="password" value={opt.password} onChange={e => set('password', e.target.value)} /></Field>
        </div>
        <Field label="Cookies" hint="Paste a Cookie header to crawl pages behind a login form"><input value={opt.cookies} onChange={e => set('cookies', e.target.value)} spellCheck={false} /></Field>
      </>}

      {step === 2 && <>
        <div className="mode-cards">
          <button className={`mode-card ${opt.mode === 'files' ? 'on' : ''}`} onClick={() => set('mode', 'files')}>
            <b>Files of specific types</b><span>Pictures, music, video, documents, archives…</span>
          </button>
          <button className={`mode-card ${opt.mode === 'mirror' ? 'on' : ''}`} onClick={() => set('mode', 'mirror')}>
            <b>Whole site (offline copy)</b><span>Pages, images, styles and scripts in a folder tree</span>
          </button>
        </div>
        {opt.mode === 'files' && <>
          <div className="days">
            {Object.keys(presets).map(p => <button key={p} className={`chip ${preset === p ? 'on' : ''}`} onClick={() => { setPreset(p); set('include', presets[p] ? presets[p].split(' ') : []); }}>{p}</button>)}
          </div>
          <Field label="File name patterns" hint="Space separated, e.g. *.jpg *.png report-*.pdf"><input value={opt.include.join(' ')} onChange={e => { setPreset(''); set('include', e.target.value.split(/\s+/).filter(Boolean)); }} /></Field>
        </>}
        <div className="grid3">
          <Field label="Link depth" hint="0 = start page only"><input type="number" min={0} max={20} value={opt.depth} onChange={e => set('depth', +e.target.value)} /></Field>
          <Field label="Max pages"><input type="number" min={1} value={opt.maxPages} onChange={e => set('maxPages', +e.target.value)} /></Field>
          <Field label="Parallel requests"><input type="number" min={1} max={16} value={opt.concurrency} onChange={e => set('concurrency', +e.target.value)} /></Field>
        </div>
        <label className="check"><input type="checkbox" checked={opt.sameHost} onChange={e => set('sameHost', e.target.checked)} /> Stay on this server</label>
        <label className="check"><input type="checkbox" checked={opt.stayBelow} onChange={e => set('stayBelow', e.target.checked)} /> Don't go above the start folder</label>
        <Field label="Exclude" hint="Patterns or URL fragments to skip, e.g. *thumb* /login logout"><input value={opt.exclude.join(' ')} onChange={e => set('exclude', e.target.value.split(/\s+/).filter(Boolean))} /></Field>
      </>}

      {step === 3 && <>
        <div className="scan-status">
          {scanning ? <><span className="spinner" /> Scanning… {progress ? `${progress.pages} pages explored, ${progress.found} files found` : ''}</>
            : <>{items.length} files found{progress ? ` on ${progress.pages} pages` : ''}.</>}
          {scanning && progress && <div className="muted small ellipsis">{progress.current}</div>}
        </div>
        {!scanning && <>
          <div className="grid3">
            <Field label="Filter"><input value={filter} onChange={e => setFilter(e.target.value)} /></Field>
            <Field label="Queue"><select value={queueId} onChange={e => setQueueId(e.target.value)}>{settings.queues.map(q => <option key={q.id} value={q.id}>{q.name}</option>)}</select></Field>
            <label className="check" style={{ alignSelf: 'end' }}><input type="checkbox" checked={startNow} onChange={e => setStartNow(e.target.checked)} /> Start queue now</label>
          </div>
          <div className="pick-list">
            <div className="pick-row head">
              <input type="checkbox" checked={shown.length > 0 && shown.every(i => checked.has(i.url))} onChange={e => setChecked(s => { const n = new Set(s); shown.forEach(i => e.target.checked ? n.add(i.url) : n.delete(i.url)); return n; })} />
              <span>Save as</span><span>Address</span>
            </div>
            {shown.map(i => (
              <label key={i.url} className="pick-row">
                <input type="checkbox" checked={checked.has(i.url)} onChange={() => setChecked(s => { const n = new Set(s); if (n.has(i.url)) { n.delete(i.url); } else { n.add(i.url); } return n; })} />
                <span className="ellipsis">{i.saveAs}</span>
                <span className="ellipsis muted">{i.url}</span>
              </label>
            ))}
          </div>
        </>}
      </>}
    </Modal>
  );
}
