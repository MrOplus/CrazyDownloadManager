import type { Download } from './api';

export function bytes(n: number): string {
  if (n == null || n < 0) return '';
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v < 10 ? 2 : v < 100 ? 1 : 0)} ${units[i]}`;
}

export function speed(n: number): string {
  if (!n || n <= 0) return '';
  return `${bytes(n)}/sec`;
}

export function duration(sec: number): string {
  if (sec == null || sec < 0 || !isFinite(sec)) return '';
  if (sec < 60) return `${Math.max(1, Math.round(sec))} sec`;
  const m = Math.floor(sec / 60), s = Math.round(sec % 60);
  if (m < 60) return `${m} min ${s} sec`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h} hr ${m % 60} min`;
  return `${Math.floor(h / 24)} days`;
}

export function date(s: string): string {
  if (!s || s.startsWith('0001')) return '';
  const d = new Date(s);
  return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

export function percent(d: Download): number {
  if (d.status === 'completed') return 100;
  if (d.size > 0) return Math.min(100, (d.downloaded / d.size) * 100);
  return -1;
}

export function statusText(d: Download): string {
  switch (d.status) {
    case 'completed': return 'Complete';
    case 'queued': return 'Queued';
    case 'paused': {
      const p = percent(d);
      return p > 0 ? `Stopped ${p.toFixed(1)}%` : 'Stopped';
    }
    case 'connecting': return 'Connecting…';
    case 'processing': return 'Processing…';
    case 'error': return 'Error';
    case 'downloading': {
      const p = percent(d);
      return p >= 0 ? `${p.toFixed(2)}%` : bytes(d.downloaded);
    }
  }
  return d.status;
}

export const isActive = (d: Download) => d.status === 'connecting' || d.status === 'downloading' || d.status === 'processing';

export function ext(name: string): string {
  const i = name.lastIndexOf('.');
  return i > 0 ? name.slice(i + 1).toLowerCase() : '';
}

export function hostOf(u: string): string {
  try { return new URL(u).host; } catch { return ''; }
}

export function urlName(u: string): string {
  try {
    const x = new URL(u);
    const last = decodeURIComponent(x.pathname.split('/').filter(Boolean).pop() || '');
    return last || x.host || u;
  } catch { return u; }
}
