import { useState } from 'react';
import { api, errText, type Download, type Queue, type Settings } from './api';
import * as F from './format';
import { Field, Modal } from './dialogs';

const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

export function SchedulerDialog({ settings, downloads, queueOn, onClose, onSaved, onError }: {
  settings: Settings; downloads: Download[]; queueOn: Record<string, boolean>;
  onClose: () => void; onSaved: () => void; onError: (m: string) => void;
}) {
  const [queues, setQueues] = useState<Queue[]>(() => structuredClone(settings.queues));
  const [cur, setCur] = useState(queues[0]?.id ?? 'main');
  const [tab, setTab] = useState<'schedule' | 'files'>('schedule');
  const q = queues.find(x => x.id === cur) ?? queues[0];

  const upd = (fn: (q: Queue) => Queue) => setQueues(qs => qs.map(x => x.id === q.id ? fn(x) : x));
  const persist = async () => {
    try { await api.saveSettings({ ...settings, queues }); onSaved(); return true; }
    catch (e) { onError(errText(e)); return false; }
  };
  const items = downloads.filter(d => d.queueId === q?.id && d.status !== 'completed');

  return (
    <Modal title="Scheduler" onClose={onClose} width={780} className="tall"
      footer={<>
        <button disabled={queueOn[q?.id]} className="primary" onClick={async () => { if (await persist()) api.startQueue(q.id); }}>Start now</button>
        <button disabled={!queueOn[q?.id]} onClick={() => api.stopQueue(q.id)}>Stop</button>
        <span className="grow" />
        <button onClick={persist}>Apply</button>
        <button onClick={async () => { if (await persist()) onClose(); }}>Close</button>
      </>}>
      <div className="split">
        <div className="queue-list">
          {queues.map(x => (
            <div key={x.id} className={`queue-item ${x.id === q?.id ? 'active' : ''}`} onClick={() => setCur(x.id)}>
              <span className={`queue-dot ${queueOn[x.id] ? 'on' : ''}`} /> {x.name}
            </div>
          ))}
          <div className="queue-actions">
            <button onClick={() => {
              const id = 'q' + Date.now().toString(36);
              setQueues(qs => [...qs, { id, name: 'New queue', maxConcurrent: 2, schedule: { enabled: false, startTime: '', stopTime: '', days: [], onComplete: 'none' } }]);
              setCur(id);
            }}>New queue</button>
            <button disabled={!q || q.id === 'main'} onClick={() => { setQueues(qs => qs.filter(x => x.id !== q.id)); setCur('main'); }}>Delete</button>
          </div>
        </div>
        {q && <div className="queue-detail">
          <Field label="Queue name"><input value={q.name} onChange={e => upd(x => ({ ...x, name: e.target.value }))} /></Field>
          <div className="tabs">
            <button className={tab === 'schedule' ? 'on' : ''} onClick={() => setTab('schedule')}>Schedule</button>
            <button className={tab === 'files' ? 'on' : ''} onClick={() => setTab('files')}>Files in the queue ({items.length})</button>
          </div>
          {tab === 'schedule' ? <div className="tab-body">
            <label className="check"><input type="checkbox" checked={q.schedule.enabled} onChange={e => upd(x => ({ ...x, schedule: { ...x.schedule, enabled: e.target.checked } }))} /> Run this queue on a schedule</label>
            <fieldset disabled={!q.schedule.enabled} className="plain">
              <div className="grid2">
                <Field label="Start download at"><input type="time" value={q.schedule.startTime} onChange={e => upd(x => ({ ...x, schedule: { ...x.schedule, startTime: e.target.value } }))} /></Field>
                <Field label="Stop download at" hint="Optional"><input type="time" value={q.schedule.stopTime} onChange={e => upd(x => ({ ...x, schedule: { ...x.schedule, stopTime: e.target.value } }))} /></Field>
              </div>
              <div className="field-label">On these days (none = every day)</div>
              <div className="days">
                {days.map((d, i) => {
                  const on = q.schedule.days.includes(i);
                  return <button key={d} className={`chip ${on ? 'on' : ''}`} onClick={() => upd(x => ({ ...x, schedule: { ...x.schedule, days: on ? x.schedule.days.filter(k => k !== i) : [...x.schedule.days, i].sort() } }))}>{d}</button>;
                })}
              </div>
              <Field label="When the queue is done">
                <select value={q.schedule.onComplete} onChange={e => upd(x => ({ ...x, schedule: { ...x.schedule, onComplete: e.target.value } }))}>
                  <option value="none">Do nothing</option>
                  <option value="exit">Exit Crazy Download Manager</option>
                  <option value="sleep">Put the computer to sleep</option>
                  <option value="shutdown">Turn off the computer</option>
                </select>
              </Field>
            </fieldset>
            <Field label="Download at the same time" hint="Files from this queue running in parallel">
              <select value={q.maxConcurrent} onChange={e => upd(x => ({ ...x, maxConcurrent: +e.target.value }))}>
                {[1, 2, 3, 4, 5, 6, 8, 10, 16].map(n => <option key={n} value={n}>{n}</option>)}
              </select>
            </Field>
          </div> : <div className="pick-list">
            {items.length === 0 && <p className="muted pad">No waiting files. Use "Download later" or right-click → Move to queue.</p>}
            {items.map(d => (
              <div key={d.id} className="pick-row three">
                <span className="ellipsis">{d.fileName || d.url}</span>
                <span className="muted">{F.bytes(d.size)}</span>
                <span className={`st st-${d.status}`}>{F.statusText(d)}</span>
              </div>
            ))}
          </div>}
        </div>}
      </div>
    </Modal>
  );
}
