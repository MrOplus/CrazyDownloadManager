// Crazy Download Manager – browser integration (Manifest V3).
//
// 1. Download takeover: when the browser starts a download whose type is in
//    the app's list, the extension cancels it and hands URL + cookies +
//    referrer + user agent to the desktop app over http://127.0.0.1:<port>.
//    If the app is not running the browser download simply continues.
// 2. Context menus: "Download with Crazy DM" and "Download all links".
// 3. Media sniffing: video/audio responses and HLS/DASH manifests seen in a
//    tab are listed in the popup and offered by an in-page panel.

const DEFAULTS = { port: 47100, token: '', enabled: true, videoPanel: true, minMediaKB: 300 };
const MEDIA_HOSTS = ['youtube.com', 'youtu.be', 'vimeo.com', 'dailymotion.com', 'twitch.tv', 'tiktok.com', 'x.com', 'twitter.com', 'facebook.com', 'instagram.com', 'soundcloud.com', 'bilibili.com', 'aparat.com'];

let cfg = { ...DEFAULTS };
let appCfg = { enabled: true, types: [], skip: [] };
let appOnline = false;
const bypass = new Map();       // url -> expiry (Alt+click, or app refused)
const media = new Map();        // tabId -> Map(url -> item)

// ------------------------------------------------------------ settings and state
//
// MV3 service workers are stopped after ~30 s idle and restarted by the next
// event, so everything above starts empty again. The token must be loaded
// before the first request (otherwise a download that wakes the worker is
// sent with an empty token and stays in the browser), and the media list is
// kept in storage.session so the popup and panel survive a restart.

async function loadCfg() {
  cfg = { ...DEFAULTS, ...(await chrome.storage.local.get(Object.keys(DEFAULTS))) };
}

async function restoreMedia() {
  try {
    const { media: saved } = await chrome.storage.session.get('media');
    for (const [tabId, items] of Object.entries(saved || {})) media.set(+tabId, new Map(items));
  } catch { }
}

let saveTimer = null;
function saveMedia() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    const out = {};
    for (const [tabId, list] of media) out[tabId] = [...list];
    chrome.storage.session.set({ media: out }).catch(() => { });
  }, 300);
}

const ready = Promise.all([loadCfg(), restoreMedia()]);

chrome.storage.onChanged.addListener((_, area) => { if (area === 'local') loadCfg().then(refreshAppCfg); });

async function api(path, body) {
  await ready;
  const res = await fetch(`http://127.0.0.1:${cfg.port}${path}`, {
    method: body ? 'POST' : 'GET',
    headers: { 'Content-Type': 'application/json', 'X-CDM-Token': cfg.token },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

async function refreshAppCfg() {
  try {
    appCfg = await api('/api/config');
    appOnline = true;
  } catch {
    appOnline = false;
  }
  chrome.action.setBadgeBackgroundColor({ color: appOnline ? '#2f7bd8' : '#888' });
  return appOnline;
}

// ------------------------------------------------------------ helpers

const extOf = name => { const m = /\.([a-z0-9]{1,6})$/i.exec(name || ''); return m ? m[1].toLowerCase() : ''; };
const baseName = p => decodeURIComponent((p || '').split(/[\\/]/).pop() || '');
const hostOf = u => { try { return new URL(u).hostname; } catch { return ''; } };

function globMatch(pattern, host) {
  const re = new RegExp('^' + pattern.trim().toLowerCase().replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.') + '$');
  return re.test(host.toLowerCase());
}
const skipped = (...urls) => urls.some(u => u && (appCfg.skip || []).some(p => p && globMatch(p, hostOf(u))));

async function cookieHeader(url) {
  try {
    const list = await chrome.cookies.getAll({ url });
    return list.map(c => `${c.name}=${c.value}`).join('; ');
  } catch { return ''; }
}

async function send(items, opts = {}) {
  for (const it of items) {
    if (!it.cookies) it.cookies = await cookieHeader(it.url);
    if (!it.userAgent) it.userAgent = navigator.userAgent;
  }
  return api('/api/add', { items, batch: !!opts.batch, silent: !!opts.silent, page: opts.page || '' });
}

function isMediaHost(url) {
  const h = hostOf(url).replace(/^(www|m)\./, '');
  return MEDIA_HOSTS.some(m => h === m || h.endsWith('.' + m));
}

// ------------------------------------------------------------ download takeover

function wantsTakeover(item) {
  if (!cfg.enabled || !appCfg.enabled) return false;
  const url = item.finalUrl || item.url;
  if (!/^(https?|ftp):/i.test(url)) return false;            // blob:, data: stay in the browser
  if (item.byExtensionId === chrome.runtime.id) return false;
  const exp = bypass.get(url) || bypass.get(item.url);
  if (exp && exp > Date.now()) return false;
  if (skipped(url, item.referrer)) return false;
  const ext = extOf(item.filename) || extOf(new URL(url).pathname);
  return (appCfg.types || []).includes(ext);
}

async function takeOver(item) {
  if (!(await refreshAppCfg()) || !wantsTakeover(item)) return false;
  const url = item.finalUrl || item.url;
  try {
    await send([{ url, fileName: baseName(item.filename), referer: item.referrer || '', size: item.totalBytes > 0 ? item.totalBytes : 0, mime: item.mime || '' }]);
  } catch (e) {
    console.warn('Crazy DM refused the download, keeping it in the browser:', e.message);
    return false;
  }
  try { await chrome.downloads.cancel(item.id); } catch { }
  try { await chrome.downloads.erase({ id: item.id }); } catch { }
  return true;
}

if (chrome.downloads.onDeterminingFilename) {
  // Chromium: decide before the file name prompt so the "Save as" dialog never shows.
  chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
    takeOver(item).finally(() => { try { suggest(); } catch { } });
    return true;
  });
} else {
  // Firefox
  chrome.downloads.onCreated.addListener(item => { takeOver(item); });
}

// ------------------------------------------------------------ context menus

function buildMenus() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: 'cdm-link', title: 'Download with Crazy DM', contexts: ['link', 'image', 'video', 'audio'] });
    chrome.contextMenus.create({ id: 'cdm-all', title: 'Download all links with Crazy DM', contexts: ['page', 'selection'] });
    chrome.contextMenus.create({ id: 'cdm-page-video', title: 'Download video from this page (yt-dlp)', contexts: ['page', 'video'] });
  });
}
chrome.runtime.onInstalled.addListener(details => {
  buildMenus();
  if (details.reason === 'install') chrome.runtime.openOptionsPage(); // not on every update
});
chrome.runtime.onStartup.addListener(buildMenus);

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  try {
    if (info.menuItemId === 'cdm-link') {
      const url = info.linkUrl || info.srcUrl;
      await send([{ url, referer: info.pageUrl }]);
    } else if (info.menuItemId === 'cdm-all') {
      const [{ result }] = await chrome.scripting.executeScript({ target: { tabId: tab.id }, func: collectLinks, args: [!!info.selectionText] });
      if (!result || !result.length) return notifyTab(tab.id, 'No links found');
      await send(result.map(l => ({ url: l.url, fileName: '', referer: tab.url })), { batch: true, page: tab.title });
    } else if (info.menuItemId === 'cdm-page-video') {
      await send([{ url: tab.url, kind: 'media', referer: tab.url }], { page: tab.title });
    }
  } catch (e) {
    notifyTab(tab && tab.id, 'Crazy Download Manager is not reachable: ' + e.message);
  }
});

// Runs inside the page.
function collectLinks(selectionOnly) {
  const out = new Map();
  const add = (u, t) => { try { const x = new URL(u, location.href); if (/^(https?|ftp):$/.test(x.protocol)) out.set(x.href, { url: x.href, text: t || '' }); } catch { } };
  let root = document;
  if (selectionOnly) {
    const sel = getSelection();
    if (sel && sel.rangeCount) {
      const div = document.createElement('div');
      for (let i = 0; i < sel.rangeCount; i++) div.appendChild(sel.getRangeAt(i).cloneContents());
      root = div;
    }
  }
  root.querySelectorAll('a[href]').forEach(a => add(a.getAttribute('href'), a.textContent.trim()));
  root.querySelectorAll('img[src]').forEach(i => add(i.getAttribute('src')));
  root.querySelectorAll('video[src], audio[src], source[src]').forEach(v => add(v.getAttribute('src')));
  return [...out.values()];
}

function notifyTab(tabId, text) {
  if (tabId == null) return;
  chrome.tabs.sendMessage(tabId, { type: 'cdm-toast', text }).catch(() => { });
}

// ------------------------------------------------------------ media sniffing

const STREAM_TYPES = /mpegurl|dash\+xml/i;
const MEDIA_TYPES = /^(video|audio)\//i;
const SEGMENT_EXT = /\.(ts|m4s|aac|vtt|webvtt|key|cmfv|cmfa)(\?|$)/i;

// The listeners below must wait for `ready`: on a cold start the media list
// is still being restored, and touching it earlier would either be undone by
// the restore or bring back the previous page's videos.
chrome.webRequest.onHeadersReceived.addListener(details => { ready.then(() => sniff(details)); },
  { urls: ['<all_urls>'], types: ['media', 'xmlhttprequest', 'other', 'object'] }, ['responseHeaders']);

function sniff(details) {
  if (details.tabId < 0 || details.statusCode >= 400) return;
  const h = Object.fromEntries((details.responseHeaders || []).map(x => [x.name.toLowerCase(), x.value || '']));
  const type = (h['content-type'] || '').split(';')[0].trim();
  const url = details.url;
  const path = (() => { try { return new URL(url).pathname.toLowerCase(); } catch { return ''; } })();
  let kind = null;
  if (STREAM_TYPES.test(type) || /\.(m3u8|mpd)$/.test(path)) kind = 'stream';
  else if (MEDIA_TYPES.test(type) && !SEGMENT_EXT.test(path) && type !== 'video/mp2t') kind = 'file';
  if (!kind) return;

  let size = 0;
  const cr = h['content-range'];
  if (cr && cr.includes('/')) size = parseInt(cr.split('/').pop(), 10) || 0;
  else size = parseInt(h['content-length'] || '0', 10) || 0;
  if (kind === 'file' && size > 0 && size < cfg.minMediaKB * 1024) return;
  // Skip HLS variant playlists that are obviously audio/subtitle renditions.
  if (kind === 'stream' && /(audio|subtitle|caption)[^/]*\.m3u8/i.test(path)) return;

  const list = media.get(details.tabId) || new Map();
  const key = url.replace(/([?&])(range|bytestart|byteend|_t|t)=[^&]*/gi, '$1');
  if (list.has(key)) return;
  list.set(key, { url, kind, mime: type, size, found: Date.now() });
  media.set(details.tabId, list);
  saveMedia();
  chrome.action.setBadgeText({ tabId: details.tabId, text: String(list.size) });
  if (cfg.videoPanel) chrome.tabs.sendMessage(details.tabId, { type: 'cdm-media', count: list.size }).catch(() => { });
}

chrome.tabs.onUpdated.addListener((tabId, change) => {
  if (change.status !== 'loading' || !change.url) return;
  ready.then(() => {
    if (media.delete(tabId)) saveMedia();
    chrome.action.setBadgeText({ tabId, text: '' }).catch(() => { });
  });
});
chrome.tabs.onRemoved.addListener(tabId => { ready.then(() => { if (media.delete(tabId)) saveMedia(); }); });

function mediaList(tabId) {
  return [...(media.get(tabId)?.values() || [])].sort((a, b) => (b.kind === 'stream') - (a.kind === 'stream') || b.size - a.size);
}

async function downloadMedia(tab, item) {
  const title = (tab.title || 'video').replace(/[\\/:*?"<>|]+/g, ' ').trim().slice(0, 120);
  const ext = item.kind === 'stream' ? (/mms/.test(item.url) ? 'asf' : 'mp4') : (extOf(new URL(item.url).pathname) || (item.mime.split('/')[1] || 'mp4'));
  await send([{ url: item.url, kind: item.kind === 'stream' ? 'stream' : 'file', fileName: `${title}.${ext}`, referer: tab.url, mime: item.mime, size: item.size }], { page: tab.title });
}

// ------------------------------------------------------------ messages from popup / content

chrome.runtime.onMessage.addListener((msg, sender, reply) => {
  (async () => {
    await ready;
    switch (msg.type) {
      case 'cdm-bypass':
        bypass.set(msg.url, Date.now() + 15000);
        return reply({ ok: true });
      case 'cdm-status': {
        const online = await refreshAppCfg();
        return reply({ online, cfg, appCfg, mediaHost: msg.tabUrl ? isMediaHost(msg.tabUrl) : false, media: msg.tabId != null ? mediaList(msg.tabId) : [] });
      }
      case 'cdm-test': {
        await loadCfg(); // the options page has just saved new values
        try { await api('/api/ping'); return reply({ ok: true }); } catch (e) { return reply({ ok: false, error: e.message }); }
      }
      case 'cdm-download-media': {
        const tab = msg.tabId != null ? await chrome.tabs.get(msg.tabId) : sender.tab;
        const list = mediaList(tab.id);
        try {
          if (msg.pageVideo) await send([{ url: tab.url, kind: 'media', referer: tab.url }], { page: tab.title });
          else if (!list[msg.index ?? 0]) throw new Error('that video is no longer listed, reload the page');
          else await downloadMedia(tab, list[msg.index ?? 0]);
          return reply({ ok: true });
        } catch (e) { return reply({ ok: false, error: e.message }); }
      }
      case 'cdm-media-list': {
        const tabId = sender.tab ? sender.tab.id : msg.tabId;
        return reply({ media: mediaList(tabId), mediaHost: sender.tab ? isMediaHost(sender.tab.url) : false, enabled: cfg.videoPanel });
      }
      case 'cdm-all-links': {
        try {
          const tab = await chrome.tabs.get(msg.tabId);
          // Throws on pages extensions can't script (browser pages, the web store).
          const [{ result }] = await chrome.scripting.executeScript({ target: { tabId: tab.id }, func: collectLinks, args: [false] });
          if (!result || !result.length) return reply({ ok: false, error: 'No links found on this page' });
          await send(result.map(l => ({ url: l.url, referer: tab.url })), { batch: true, page: tab.title });
          reply({ ok: true });
        } catch (e) { reply({ ok: false, error: e.message }); }
        return;
      }
      default:
        reply({ ok: false, error: 'unknown message' });
    }
  })();
  return true;
});

ready.then(refreshAppCfg);
setInterval(refreshAppCfg, 60000);
