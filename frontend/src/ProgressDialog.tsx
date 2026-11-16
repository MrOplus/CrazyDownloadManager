import { useEffect, useState } from 'react';
import { api, errText, type Download } from './api';
import * as F from './format';
import { Modal } from './dialogs';

// Per-download window: status, speed, time left and a live map of
// connections, like the classic download-progress window.
export function ProgressDialog({ id, downloads, onClose, onError }: { id: string; downloads: Download[]; onClose: () => void; onError: (m: string) => void }) {
  const d = downloads.find(x => x.id === id);
  const [details, setDetails] = useState(true);
  const [limitOn, setLimitOn] = useState(false);
  const [limit, setLimit] = useState(512);

  useEffect(() => {
    if (d) { setLimitOn(d.speedLimit > 0); if (d.speedLimit > 0) setLimit(d.speedLimit); }
    // only on open
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  useEffect(() => { if (d?.status === 'completed') onClose(); }, [d?.status, onClose]);

  if (!d) return null;
  const p = F.percent(d);
  const active = F.isActive(d);
  const pct = p >= 0 ? `${p.toFixed(1)}%` : '';
  const segs = d.segments ?? [];
  const size = d.size > 0 ? d.size : 0;

  const applyLimit = (on: boolean, v: number) => {
    setLimitOn(on); setLimit(v);
    api.setSpeedLimit(d.id, on ? v : 0).catch(e => onError(errText(e)));
  };

  return (
    <Modal title={`${pct} ${d.fileName || d.url}`} onClose={onClose} width={600}
      footer={<>
        <button onClick={() => setDetails(x => !x)}>{details ? '<< Hide details' : 'Show details >>'}</button>
        <span className="grow" />
        {active
          ? <button onClick={() => api.pause([d.id])}>Pause</button>
          : <button className="primary" onClick={() => api.resume([d.id])}>Resume</button>}
        <button onClick={onClose}>Close</button>
      </>}>
      <div className="prog-tabs">
        <div className="kv">
          <span>URL</span><span className="ellipsis" title={d.url}>{d.url}</span>
          <span>Status</span><span className={`st st-${d.status}`}>{d.status === 'error' ? d.error : d.status === 'downloading' ? (d.kind === 'file' ? 'Receiving data…' : 'Recording stream…') : F.statusText(d)}</span>
          <span>File size</span><span>{d.size > 0 ? F.bytes(d.size) : 'Unknown'}{d.kind !== 'file' && d.size > 0 && ' (estimated)'}</span>
          <span>Downloaded</span><span>{F.bytes(d.downloaded)}{pct && ` (${pct})`}</span>
          <span>Transfer rate</span><span>{active ? F.speed(d.speed) || '—' : '—'}</span>
          <span>Time left</span><span>{active ? F.duration(d.eta) || '—' : '—'}</span>
          <span>Resume capability</span><span className={d.resumable ? 'ok' : 'warn'}>{d.kind === 'file' ? (d.resumable ? 'Yes' : 'No') : d.kind === 'media' ? 'Yes (yt-dlp)' : 'No (live/stream)'}</span>
          <span>Save to</span><span className="ellipsis" title={d.dir}>{d.dir}</span>
        </div>
        <div className="big-bar"><div style={{ width: `${p >= 0 ? p : 0}%` }} className={p < 0 && active ? 'indeterminate' : ''} /></div>
      </div>

      {details && <>
        {size > 0 && segs.length > 0 && (
          <div className="seg-map" title="Each block is a byte range handled by one connection">
            {segs.map((s, i) => (
              <div key={i} className="seg" style={{ left: `${(s.start / size) * 100}%`, width: `${((s.end - s.start) / size) * 100}%` }}>
                <div className={`seg-fill ${s.active ? 'live' : ''}`} style={{ width: s.end > s.start ? `${((s.pos - s.start) / (s.end - s.start)) * 100}%` : '100%' }} />
              </div>
            ))}
          </div>
        )}
        <div className="limit-row">
          <label className="check inline"><input type="checkbox" checked={limitOn} onChange={e => applyLimit(e.target.checked, limit)} /> Limit speed to</label>
          <input type="number" min={1} value={limit} disabled={!limitOn} onChange={e => applyLimit(limitOn, Math.max(1, +e.target.value))} />
          <span className="muted">KB/s</span>
          <span className="grow" />
          <span className="muted">{active ? `${d.activeConns} active connection${d.activeConns === 1 ? '' : 's'}` : `${segs.filter(s => s.end < 0 || s.pos < s.end).length} unfinished ranges`}</span>
        </div>
        {segs.length > 0 && (
          <div className="conn-table">
            <div className="conn-row head"><span>N.</span><span>Range</span><span>Downloaded</span><span>Info</span></div>
            {segs.map((s, i) => {
              const done = s.end >= 0 && s.pos >= s.end;
              return (
                <div key={i} className="conn-row">
                  <span>{i + 1}</span>
                  <span>{F.bytes(s.start)} – {s.end >= 0 ? F.bytes(s.end) : '?'}</span>
                  <span>{F.bytes(s.pos - s.start)}</span>
                  <span className={done ? 'ok' : active && s.active ? '' : 'muted'}>{done ? 'Complete' : !active ? 'Stopped' : s.active ? 'Receiving data…' : 'Waiting for a free connection'}</span>
                </div>
              );
            })}
          </div>
        )}
      </>}
    </Modal>
  );
}
