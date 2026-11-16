import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, on, errText, type BatchRequest, type BridgeItem, type Download, type Info, type Settings } from './api';
import * as F from './format';
import * as I from './icons';
import { AddUrlDialog, BatchDialog, CompleteDialog, DeleteDialog, DownloadInfoDialog, Toast } from './dialogs';
import { ProgressDialog } from './ProgressDialog';
import { OptionsDialog } from './OptionsDialog';
import { SchedulerDialog } from './SchedulerDialog';
import { GrabberDialog } from './GrabberDialog';

type Filter = { group: 'all' | 'unfinished' | 'finished' | 'queue'; value?: string };
type SortKey = 'fileName' | 'size' | 'status' | 'eta' | 'speed' | 'lastTry' | 'added' | 'description';

type DialogState =
  | { kind: 'none' }
  | { kind: 'addUrl' }
  | { kind: 'info'; item: BridgeItem }
  | { kind: 'batch'; req: BatchRequest }
  | { kind: 'progress'; id: string }
  | { kind: 'delete'; ids: string[] }
  | { kind: 'options'; tab?: string }
  | { kind: 'scheduler' }
  | { kind: 'grabber' }
  | { kind: 'complete'; d: Download };

const columns: { key: SortKey; label: string; width: string; align?: 'right' }[] = [
  { key: 'fileName', label: 'File Name', width: 'minmax(220px, 3fr)' },
  { key: 'size', label: 'Size', width: '90px', align: 'right' },
  { key: 'status', label: 'Status', width: '150px' },
  { key: 'eta', label: 'Time left', width: '110px' },
  { key: 'speed', label: 'Transfer rate', width: '110px', align: 'right' },
  { key: 'lastTry', label: 'Last Try', width: '120px' },
  { key: 'description', label: 'Description', width: 'minmax(120px, 1.4fr)' },
];
const gridTemplate = columns.map(c => c.width).join(' ');

export default function App() {
  const [downloads, setDownloads] = useState<Download[]>([]);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [info, setInfo] = useState<Info | null>(null);
  const [queueOn, setQueueOn] = useState<Record<string, boolean>>({});
  const [filter, setFilter] = useState<Filter>({ group: 'all' });
  const [catFilter, setCatFilter] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [anchor, setAnchor] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: 'added', desc: false });
  const [dialog, setDialog] = useState<DialogState>({ kind: 'none' });
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({ all: true, unfinished: false, finished: false, queue: true });
  const settingsRef = useRef<Settings | null>(null);
  settingsRef.current = settings;
  const dialogRef = useRef(dialog);
  dialogRef.current = dialog;

  const notify = useCallback((msg: string) => setToast(msg), []);

  const reloadSettings = useCallback(async () => {
    const [s, i] = await Promise.all([api.settings(), api.info()]);
    setSettings(s);
    setInfo(i);
  }, []);

  // ---------------------------------------------------------------- wiring
  useEffect(() => {
    api.list().then(setDownloads);
    reloadSettings();
    const offs = [
      on<Download[]>('downloads', setDownloads),
      on<BridgeItem[]>('ui:add', items => {
        if (items.length === 1) setDialog({ kind: 'info', item: items[0] });
        else setDialog({ kind: 'batch', req: { items, batch: true, silent: false, page: '' } });
      }),
      on<BatchRequest>('ui:batch', req => setDialog({ kind: 'batch', req })),
      on<Download>('download:complete', d => {
        if (settingsRef.current?.showCompleteDialog && dialogRef.current.kind === 'none') setDialog({ kind: 'complete', d });
        else notify(`Download complete: ${d.fileName}`);
      }),
      on<Download>('download:failed', d => notify(`Failed: ${d.fileName || d.url} — ${d.error}`)),
      on<{ queue: { name: string } }>('queue:done', q => notify(`${q.queue.name} finished`)),
    ];
    const t = setInterval(() => api.queueStates().then(setQueueOn), 1000);
    return () => { offs.forEach(o => o()); clearInterval(t); };
  }, [notify, reloadSettings]);

  // Theme
  useEffect(() => {
    const t = settings?.theme ?? 'system';
    if (t === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.setAttribute('data-theme', t);
  }, [settings?.theme]);

  // Links dropped onto the window (from a browser) become downloads.
  useEffect(() => {
    const over = (e: DragEvent) => { e.preventDefault(); };
    const drop = (e: DragEvent) => {
      e.preventDefault();
      const dt = e.dataTransfer;
      if (!dt) return;
      const text = dt.getData('text/uri-list') || dt.getData('text/plain');
      const urls = text.split(/\r?\n/).map(s => s.trim()).filter(s => /^(https?|ftps?|mms[ht]?|rtmps?|rtsp):\/\//i.test(s));
      if (urls.length === 1) setDialog({ kind: 'info', item: { url: urls[0] } });
      else if (urls.length > 1) setDialog({ kind: 'batch', req: { items: urls.map(url => ({ url })), batch: true, silent: false, page: '' } });
    };
    window.addEventListener('dragover', over);
    window.addEventListener('drop', drop);
    return () => { window.removeEventListener('dragover', over); window.removeEventListener('drop', drop); };
  }, []);

  // ---------------------------------------------------------------- derived
  const categories = useMemo(() => {
    const names = (settings?.categories ?? []).map(c => c.name);
    const extra = new Set(downloads.map(d => d.category).filter(c => c && !names.includes(c)));
    return [...names, ...extra, ...(extra.has('General') || names.includes('General') ? [] : ['General'])];
  }, [settings, downloads]);

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    let list = downloads.filter(d => {
      if (filter.group === 'unfinished' && d.status === 'completed') return false;
      if (filter.group === 'finished' && d.status !== 'completed') return false;
      if (filter.group === 'queue' && d.queueId !== filter.value) return false;
      if (catFilter && (d.category || 'General') !== catFilter) return false;
      if (q && !(`${d.fileName} ${d.url} ${d.description ?? ''}`.toLowerCase().includes(q))) return false;
      return true;
    });
    const k = sort.key;
    const val = (d: Download): number | string => {
      switch (k) {
        case 'fileName': return (d.fileName || d.url).toLowerCase();
        case 'size': return d.size;
        case 'status': return F.percent(d) + (d.status === 'completed' ? 1000 : 0);
        case 'eta': return d.eta < 0 ? Number.MAX_SAFE_INTEGER : d.eta;
        case 'speed': return d.speed;
        case 'lastTry': return d.lastTry;
        case 'added': return d.added;
        case 'description': return d.description ?? '';
      }
    };
    list = [...list].sort((a, b) => {
      const x = val(a), y = val(b);
      const c = x < y ? -1 : x > y ? 1 : 0;
      return sort.desc ? -c : c;
    });
    return list;
  }, [downloads, filter, catFilter, search, sort]);

  const sel = useMemo(() => downloads.filter(d => selected.has(d.id)), [downloads, selected]);
  const selIds = sel.map(d => d.id);
  const totalSpeed = downloads.reduce((s, d) => s + (F.isActive(d) ? d.speed : 0), 0);
  const activeCount = downloads.filter(F.isActive).length;
  const queues = settings?.queues ?? [];

  // ---------------------------------------------------------------- actions
  const act = async (fn: () => Promise<unknown>) => {
    try { await fn(); } catch (e) { notify(errText(e)); }
  };
  const resume = () => act(() => api.resume(sel.filter(d => d.status !== 'completed').map(d => d.id)));
  const stop = () => act(() => api.pause(selIds));
  const del = () => selIds.length && setDialog({ kind: 'delete', ids: selIds });
  const startQueue = () => act(() => api.startQueue(filter.group === 'queue' && filter.value ? filter.value : 'main'));
  const stopQueue = () => act(() => api.stopQueue(filter.group === 'queue' && filter.value ? filter.value : 'main'));

  const openRow = (d: Download) => {
    if (d.status === 'completed') act(() => api.openFile(d.id));
    else setDialog({ kind: 'progress', id: d.id });
  };

  const clickRow = (e: React.MouseEvent, d: Download) => {
    const next = new Set(e.ctrlKey || e.metaKey ? selected : []);
    if (e.shiftKey && anchor) {
      const ids = visible.map(v => v.id);
      const a = ids.indexOf(anchor), b = ids.indexOf(d.id);
      ids.slice(Math.min(a, b), Math.max(a, b) + 1).forEach(id => next.add(id));
    } else if ((e.ctrlKey || e.metaKey) && selected.has(d.id)) {
      next.delete(d.id);
    } else {
      next.add(d.id);
      setAnchor(d.id);
    }
    setSelected(next);
  };

  // Keyboard shortcuts
  useEffect(() => {
    const k = (e: KeyboardEvent) => {
      if (dialog.kind !== 'none' || (e.target as HTMLElement)?.tagName === 'INPUT') return;
      if (e.key === 'Delete') del();
      else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'a') { e.preventDefault(); setSelected(new Set(visible.map(v => v.id))); }
      else if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') { e.preventDefault(); setDialog({ kind: 'addUrl' }); }
      else if (e.key === 'Enter' && sel.length === 1) openRow(sel[0]);
    };
    window.addEventListener('keydown', k);
    return () => window.removeEventListener('keydown', k);
  });

  const close = () => setDialog({ kind: 'none' });

  // ---------------------------------------------------------------- render
  const anyResumable = sel.some(d => d.status === 'paused' || d.status === 'error' || d.status === 'queued');
  const anyActive = sel.some(F.isActive);
  const curQueue = filter.group === 'queue' && filter.value ? filter.value : 'main';

  const tb = [
    { label: 'Add URL', icon: <I.IconAdd />, onClick: () => setDialog({ kind: 'addUrl' }), title: 'Add a download (Ctrl+N)' },
    { label: 'Resume', icon: <I.IconResume />, onClick: resume, disabled: !anyResumable },
    { label: 'Stop', icon: <I.IconStop />, onClick: stop, disabled: !anyActive && !sel.some(d => d.status === 'queued') },
    { label: 'Stop All', icon: <I.IconStopAll />, onClick: () => act(api.pauseAll), disabled: activeCount === 0 },
    'sep',
    { label: 'Delete', icon: <I.IconDelete />, onClick: del, disabled: sel.length === 0 },
    { label: 'Delete Co…', icon: <I.IconDeleteDone />, onClick: () => act(api.removeCompleted), title: 'Delete completed downloads from the list' },
    'sep',
    { label: 'Options', icon: <I.IconOptions />, onClick: () => setDialog({ kind: 'options' }) },
    { label: 'Scheduler', icon: <I.IconScheduler />, onClick: () => setDialog({ kind: 'scheduler' }) },
    { label: 'Start Queue', icon: <I.IconQueueStart />, onClick: startQueue, disabled: queueOn[curQueue] },
    { label: 'Stop Queue', icon: <I.IconQueueStop />, onClick: stopQueue, disabled: !queueOn[curQueue] },
    'sep',
    { label: 'Grabber', icon: <I.IconGrabber />, onClick: () => setDialog({ kind: 'grabber' }) },
    { label: 'Extension', icon: <I.IconBrowser />, onClick: () => setDialog({ kind: 'options', tab: 'browser' }), title: 'Browser integration' },
  ] as const;

  const TreeItem = ({ id, label, group, value, count, icon, children }: { id: string; label: string; group: Filter['group']; value?: string; count?: number; icon?: React.ReactNode; children?: React.ReactNode }) => {
    const active = filter.group === group && filter.value === value && !catFilter;
    return (
      <div className="tree-node">
        <div className={`tree-row ${active ? 'active' : ''}`} onClick={() => { setFilter({ group, value }); setCatFilter(null); }}>
          {children ? (
            <button className="tree-toggle" onClick={e => { e.stopPropagation(); setExpanded(x => ({ ...x, [id]: !x[id] })); }}>
              <I.Chevron open={!!expanded[id]} />
            </button>
          ) : <span className="tree-toggle" />}
          {icon ?? <I.Folder />}
          <span className="tree-label">{label}</span>
          {count !== undefined && count > 0 && <span className="tree-count">{count}</span>}
        </div>
        {children && expanded[id] && <div className="tree-children">{children}</div>}
      </div>
    );
  };

  const catNodes = (group: Filter['group']) => categories.map(c => {
    const n = downloads.filter(d => (d.category || 'General') === c && (group === 'all' || (group === 'finished') === (d.status === 'completed'))).length;
    const active = filter.group === group && catFilter === c;
    return (
      <div key={group + c} className={`tree-row leaf ${active ? 'active' : ''}`} onClick={() => { setFilter({ group }); setCatFilter(c); }}>
        <span className="tree-toggle" />
        <span className="cat-dot" style={{ background: I.categoryColor(c) }} />
        <span className="tree-label">{c}</span>
        {n > 0 && <span className="tree-count">{n}</span>}
      </div>
    );
  });

  return (
    <div className="app" onClick={() => setMenu(null)}>
      {info?.initError && <div className="banner error">Settings could not be loaded: {info.initError}</div>}
      {info?.bridgeError && <div className="banner warn">Browser integration is offline: {info.bridgeError}. Change the port in Options → Browser.</div>}

      <header className="toolbar">
        {tb.map((b, i) => b === 'sep' ? <div key={i} className="tb-sep" /> : (
          <button key={b.label} className="tb-btn" onClick={b.onClick} disabled={'disabled' in b ? b.disabled : false} title={'title' in b ? b.title : b.label}>
            {b.icon}
            <span>{b.label}</span>
          </button>
        ))}
        <div className="tb-spacer" />
        <input className="search" placeholder="Search downloads" value={search} onChange={e => setSearch(e.target.value)} />
      </header>

      <div className="main">
        <aside className="sidebar">
          <div className="sidebar-title">Categories</div>
          <TreeItem id="all" label="All Downloads" group="all" count={downloads.length} icon={<I.Folder color="#5f9ee6" />}>{catNodes('all')}</TreeItem>
          <TreeItem id="unfinished" label="Unfinished" group="unfinished" count={downloads.filter(d => d.status !== 'completed').length} icon={<I.Folder color="#e0a13a" />}>{catNodes('unfinished')}</TreeItem>
          <TreeItem id="finished" label="Finished" group="finished" count={downloads.filter(d => d.status === 'completed').length} icon={<I.Folder color="#3fae5b" />}>{catNodes('finished')}</TreeItem>
          <TreeItem id="queue" label="Queues" group="queue" value={queues[0]?.id} icon={<I.Folder color="#8a95a5" />}>
            {queues.map(q => (
              <div key={q.id} className={`tree-row leaf ${filter.group === 'queue' && filter.value === q.id ? 'active' : ''}`} onClick={() => { setFilter({ group: 'queue', value: q.id }); setCatFilter(null); }}>
                <span className="tree-toggle" />
                <span className={`queue-dot ${queueOn[q.id] ? 'on' : ''}`} />
                <span className="tree-label">{q.name}</span>
                <span className="tree-count">{downloads.filter(d => d.queueId === q.id && d.status !== 'completed').length || ''}</span>
              </div>
            ))}
          </TreeItem>
        </aside>

        <section className="list" onContextMenu={e => e.preventDefault()}>
          <div className="list-head" style={{ gridTemplateColumns: gridTemplate }}>
            {columns.map(c => (
              <div key={c.key} className={`th ${c.align === 'right' ? 'right' : ''}`} onClick={() => setSort(s => ({ key: c.key, desc: s.key === c.key ? !s.desc : false }))}>
                {c.label}{sort.key === c.key && <span className="sort">{sort.desc ? '▼' : '▲'}</span>}
              </div>
            ))}
          </div>
          <div className="list-body" onClick={e => { if (e.target === e.currentTarget) setSelected(new Set()); }}>
            {visible.length === 0 && (
              <div className="empty">
                <I.IconAdd size={48} />
                <p>No downloads here yet.</p>
                <p className="muted">Click <b>Add URL</b>, drop a link on this window, paste a link while clipboard monitoring is on, or click a download link in the browser with the extension installed.</p>
              </div>
            )}
            {visible.map(d => {
              const p = F.percent(d);
              return (
                <div key={d.id}
                  className={`row ${selected.has(d.id) ? 'selected' : ''} st-${d.status}`}
                  style={{ gridTemplateColumns: gridTemplate }}
                  onClick={e => { e.stopPropagation(); clickRow(e, d); }}
                  onDoubleClick={() => openRow(d)}
                  onContextMenu={e => {
                    e.preventDefault(); e.stopPropagation();
                    if (!selected.has(d.id)) { setSelected(new Set([d.id])); setAnchor(d.id); }
                    setMenu({ x: e.clientX, y: e.clientY });
                  }}>
                  <div className="td name" title={d.url}>
                    <I.FileBadge category={d.category} kind={d.kind} name={d.fileName || F.urlName(d.url)} />
                    <span className="ellipsis">{d.fileName || F.urlName(d.url)}</span>
                    {queueOn[d.queueId] && d.status === 'queued' && <span className="q-tag">Q</span>}
                  </div>
                  <div className="td right">{F.bytes(d.size)}</div>
                  <div className="td status-cell" title={d.error || ''}>
                    {(F.isActive(d) || d.status === 'paused') && p >= 0 && <div className="mini-bar"><div style={{ width: `${p}%` }} /></div>}
                    <span className={`st st-${d.status} ellipsis`}>{F.statusText(d)}</span>
                  </div>
                  <div className="td">{F.isActive(d) ? F.duration(d.eta) : ''}</div>
                  <div className="td right">{F.isActive(d) ? F.speed(d.speed) : ''}</div>
                  <div className="td">{F.date(d.lastTry || d.added)}</div>
                  <div className="td ellipsis muted">{d.status === 'error' ? d.error : d.description}</div>
                </div>
              );
            })}
          </div>
        </section>
      </div>

      <footer className="statusbar">
        <span>{downloads.length} downloads</span>
        <span>{activeCount} active</span>
        <span>{totalSpeed > 0 ? `↓ ${F.speed(totalSpeed)}` : 'Idle'}</span>
        {settings && settings.globalSpeedLimit > 0 && <span>Limit {settings.globalSpeedLimit} KB/s</span>}
        <span className="grow" />
        {queues.filter(q => queueOn[q.id]).map(q => <span key={q.id} className="pill">▶ {q.name}</span>)}
        <span className="muted">v{info?.version}</span>
      </footer>

      {menu && sel.length > 0 && (
        <div className="ctx" style={{ left: Math.min(menu.x, window.innerWidth - 220), top: Math.min(menu.y, window.innerHeight - 330) }} onClick={e => e.stopPropagation()}>
          {sel.length === 1 && sel[0].status === 'completed' && <>
            <button onClick={() => { setMenu(null); act(() => api.openFile(sel[0].id)); }}><b>Open</b></button>
            <button onClick={() => { setMenu(null); act(() => api.openFolder(sel[0].id)); }}>Open folder</button>
            <hr />
          </>}
          {sel.length === 1 && sel[0].status !== 'completed' && <button onClick={() => { setMenu(null); setDialog({ kind: 'progress', id: sel[0].id }); }}><b>Show progress</b></button>}
          {sel.length === 1 && sel[0].status !== 'completed' && <button onClick={() => { setMenu(null); act(() => api.openFolder(sel[0].id)); }}>Open folder</button>}
          <button disabled={!anyResumable} onClick={() => { setMenu(null); resume(); }}>Resume download</button>
          <button disabled={!anyActive} onClick={() => { setMenu(null); stop(); }}>Stop download</button>
          {sel.length === 1 && <button onClick={() => { setMenu(null); act(() => api.redownload(sel[0].id)); }}>Redownload</button>}
          <hr />
          <div className="ctx-label">Move to queue</div>
          {queues.map(q => <button key={q.id} className="indent" onClick={() => { setMenu(null); act(() => api.moveToQueue(selIds, q.id)); }}>{q.name}</button>)}
          <hr />
          {sel.length === 1 && <button onClick={() => { setMenu(null); api.copy(sel[0].url); notify('Address copied'); }}>Copy address</button>}
          <button onClick={() => { setMenu(null); del(); }}>Delete…</button>
        </div>
      )}

      {dialog.kind === 'addUrl' && <AddUrlDialog onClose={close} onNext={item => setDialog({ kind: 'info', item })} />}
      {dialog.kind === 'info' && settings && <DownloadInfoDialog item={dialog.item} settings={settings} onClose={close} onError={notify} />}
      {dialog.kind === 'batch' && settings && <BatchDialog req={dialog.req} settings={settings} onClose={close} onError={notify} />}
      {dialog.kind === 'delete' && <DeleteDialog count={dialog.ids.length} onClose={close} onConfirm={files => { close(); setSelected(new Set()); act(() => api.remove(dialog.ids, files)); }} />}
      {dialog.kind === 'progress' && <ProgressDialog id={dialog.id} downloads={downloads} onClose={close} onError={notify} />}
      {dialog.kind === 'complete' && <CompleteDialog d={dialog.d} onClose={close} onError={notify} />}
      {dialog.kind === 'options' && settings && info && <OptionsDialog settings={settings} info={info} initialTab={dialog.tab} onClose={close} onSaved={() => { reloadSettings(); close(); }} onError={notify} />}
      {dialog.kind === 'scheduler' && settings && <SchedulerDialog settings={settings} downloads={downloads} queueOn={queueOn} onClose={close} onSaved={reloadSettings} onError={notify} />}
      {dialog.kind === 'grabber' && settings && <GrabberDialog settings={settings} onClose={close} onError={notify} onDone={n => notify(`${n} files added to the queue`)} />}
      {toast && <Toast text={toast} onDone={() => setToast(null)} />}
    </div>
  );
}
