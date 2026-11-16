// Toolbar and file-type icons, drawn as inline SVG so they stay crisp and
// follow the theme. Colours echo the classic download-manager toolbar.
import type { JSX } from 'react';

type P = { size?: number };

const S = ({ size = 30, children }: { size?: number; children: JSX.Element | JSX.Element[] }) => (
  <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">{children}</svg>
);

export const IconAdd = ({ size }: P) => (
  <S size={size}>
    <circle cx="14" cy="14" r="11" fill="#2f7bd8" />
    <path d="M3 14h22M14 3c3.5 3 5 7 5 11s-1.5 8-5 11c-3.5-3-5-7-5-11s1.5-8 5-11z" stroke="#cfe3fb" strokeWidth="1.4" fill="none" />
    <circle cx="24" cy="24" r="7" fill="#27a844" />
    <path d="M24 20.5v7M20.5 24h7" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" />
  </S>
);
export const IconResume = ({ size }: P) => (
  <S size={size}>
    <circle cx="16" cy="16" r="13" fill="#27a844" />
    <path d="M10 16a6 6 0 1 0 2-4.5" stroke="#fff" strokeWidth="2.6" fill="none" strokeLinecap="round" />
    <path d="M9 8.5v5h5" stroke="#fff" strokeWidth="2.6" fill="none" strokeLinecap="round" strokeLinejoin="round" />
  </S>
);
export const IconStop = ({ size }: P) => (
  <S size={size}>
    <circle cx="16" cy="16" r="13" fill="#e0443e" />
    <rect x="10.5" y="10.5" width="11" height="11" rx="1.5" fill="#fff" />
  </S>
);
export const IconStopAll = ({ size }: P) => (
  <S size={size}>
    <circle cx="13" cy="13" r="10" fill="#f08a84" />
    <circle cx="18" cy="18" r="11" fill="#c9302c" />
    <rect x="13.5" y="13.5" width="9" height="9" rx="1.2" fill="#fff" />
  </S>
);
export const IconDelete = ({ size }: P) => (
  <S size={size}>
    <path d="M8 9h16l-1.4 18.2a2 2 0 0 1-2 1.8h-9.2a2 2 0 0 1-2-1.8z" fill="#7d8796" />
    <rect x="6" y="6" width="20" height="3.5" rx="1.2" fill="#5b6472" />
    <rect x="13" y="3.5" width="6" height="3" rx="1" fill="#5b6472" />
    <path d="M13 13v11M16 13v11M19 13v11" stroke="#e8ebef" strokeWidth="1.6" strokeLinecap="round" />
  </S>
);
export const IconDeleteDone = ({ size }: P) => (
  <S size={size}>
    <path d="M6 9h14l-1.2 16a2 2 0 0 1-2 1.8H9.2a2 2 0 0 1-2-1.8z" fill="#7d8796" />
    <rect x="4" y="6" width="18" height="3.2" rx="1.1" fill="#5b6472" />
    <circle cx="24" cy="23" r="7" fill="#27a844" />
    <path d="M20.8 23l2.2 2.2 4.2-4.4" stroke="#fff" strokeWidth="2.2" fill="none" strokeLinecap="round" strokeLinejoin="round" />
  </S>
);
export const IconOptions = ({ size }: P) => (
  <S size={size}>
    <path fill="#6a7686" d="M14 2h4l.8 3.6a11 11 0 0 1 3 1.3l3.1-2 2.8 2.8-2 3.1a11 11 0 0 1 1.3 3l3.6.8v4l-3.6.8a11 11 0 0 1-1.3 3l2 3.1-2.8 2.8-3.1-2a11 11 0 0 1-3 1.3L18 30h-4l-.8-3.6a11 11 0 0 1-3-1.3l-3.1 2-2.8-2.8 2-3.1a11 11 0 0 1-1.3-3L1.4 18v-4l3.6-.8a11 11 0 0 1 1.3-3l-2-3.1 2.8-2.8 3.1 2a11 11 0 0 1 3-1.3z" />
    <circle cx="16" cy="16" r="5" fill="#dfe5ec" />
  </S>
);
export const IconScheduler = ({ size }: P) => (
  <S size={size}>
    <circle cx="16" cy="17" r="12" fill="#f2a531" />
    <circle cx="16" cy="17" r="9.5" fill="#fff7e8" />
    <path d="M16 11v6.5l4 2.5" stroke="#9a5b06" strokeWidth="2.2" fill="none" strokeLinecap="round" />
    <path d="M5 7.5l4-3.5M27 7.5l-4-3.5" stroke="#d27f0a" strokeWidth="2.6" strokeLinecap="round" />
  </S>
);
export const IconQueueStart = ({ size }: P) => (
  <S size={size}>
    <rect x="3" y="5" width="16" height="4" rx="1.2" fill="#5f9ee6" />
    <rect x="3" y="12" width="16" height="4" rx="1.2" fill="#5f9ee6" />
    <rect x="3" y="19" width="10" height="4" rx="1.2" fill="#5f9ee6" />
    <circle cx="23" cy="22" r="8" fill="#27a844" />
    <path d="M21 18.5v7l5.5-3.5z" fill="#fff" />
  </S>
);
export const IconQueueStop = ({ size }: P) => (
  <S size={size}>
    <rect x="3" y="5" width="16" height="4" rx="1.2" fill="#5f9ee6" />
    <rect x="3" y="12" width="16" height="4" rx="1.2" fill="#5f9ee6" />
    <rect x="3" y="19" width="10" height="4" rx="1.2" fill="#5f9ee6" />
    <circle cx="23" cy="22" r="8" fill="#e0443e" />
    <rect x="20" y="19" width="6" height="6" rx="1" fill="#fff" />
  </S>
);
export const IconGrabber = ({ size }: P) => (
  <S size={size}>
    <g stroke="#8a95a5" strokeWidth="1.1" fill="none">
      <path d="M16 2v28M2 16h28M6 6l20 20M26 6L6 26" />
      <circle cx="16" cy="16" r="5" /><circle cx="16" cy="16" r="9.5" /><circle cx="16" cy="16" r="13.5" />
    </g>
    <ellipse cx="16" cy="17" rx="3.4" ry="4.2" fill="#3a3f47" />
    <circle cx="16" cy="12" r="2.4" fill="#3a3f47" />
    <path d="M13 15l-4-2M13 18l-4.5 0M13 20.5l-3.5 3M19 15l4-2M19 18l4.5 0M19 20.5l3.5 3" stroke="#3a3f47" strokeWidth="1.4" strokeLinecap="round" />
  </S>
);
export const IconBrowser = ({ size }: P) => (
  <S size={size}>
    <rect x="3" y="5" width="26" height="22" rx="3" fill="#5f6b7a" />
    <rect x="5" y="10" width="22" height="15" rx="1.5" fill="#eef2f7" />
    <circle cx="7" cy="7.5" r="1" fill="#ff6159" /><circle cx="10" cy="7.5" r="1" fill="#ffbd2e" /><circle cx="13" cy="7.5" r="1" fill="#28c941" />
    <path d="M16 13v8m-3.5-3.5L16 21l3.5-3.5" stroke="#2f7bd8" strokeWidth="2.2" fill="none" strokeLinecap="round" strokeLinejoin="round" />
  </S>
);

// ------------------------------------------------------------ small icons

export const Chevron = ({ open }: { open: boolean }) => (
  <svg width="10" height="10" viewBox="0 0 10 10" style={{ transform: open ? 'rotate(90deg)' : undefined, transition: 'transform .12s' }}>
    <path d="M3 1.5L7 5 3 8.5" stroke="currentColor" strokeWidth="1.5" fill="none" />
  </svg>
);

export const Folder = ({ color = '#e6b23c' }: { color?: string }) => (
  <svg width="16" height="16" viewBox="0 0 16 16"><path d="M1.5 3.5h4.5l1.5 1.5h7v8.5h-13z" fill={color} /><path d="M1.5 6h13v7.5h-13z" fill={color} opacity=".8" /></svg>
);

const typeColors: Record<string, [string, string]> = {
  Compressed: ['#8a5cd6', 'ZIP'], Documents: ['#2f7bd8', 'DOC'], Music: ['#d6487e', '♪'],
  Programs: ['#4f5d70', 'EXE'], Video: ['#e2721b', '▶'], General: ['#7d8796', '•'], Grabber: ['#3e9e6a', 'WEB'],
};

export function FileBadge({ category, kind, name }: { category: string; kind?: string; name?: string }) {
  let [color, label] = kind === 'stream' ? ['#c2410c', 'LIVE'] : kind === 'media' ? ['#dc2626', '▶'] : (typeColors[category] ?? typeColors.General);
  // Show the file's own extension (ISO, RAR, PDF…); the category only picks the color.
  const dot = name ? name.lastIndexOf('.') : -1;
  const ext = name && dot > 0 ? name.slice(dot + 1) : '';
  if (kind !== 'stream' && /^[a-z0-9]{1,4}$/i.test(ext)) label = ext.toUpperCase();
  return <span className="file-badge" style={{ background: color }}>{label}</span>;
}

export function categoryColor(name: string): string {
  return (typeColors[name] ?? typeColors.General)[0];
}
