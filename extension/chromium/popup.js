const $ = id => document.getElementById(id);

async function init() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  const st = await chrome.runtime.sendMessage({ type: 'cdm-status', tabId: tab.id, tabUrl: tab.url });
  $('status').className = 'dot ' + (st.online ? 'on' : 'off');
  $('status').title = st.online ? 'Connected' : 'Not connected';
  $('offline').hidden = st.online;
  $('enabled').checked = st.cfg.enabled;
  $('enabled').onchange = e => chrome.storage.local.set({ enabled: e.target.checked });

  const box = $('media');
  const items = st.media.map((m, i) => ({
    label: m.kind === 'stream' ? (m.url.includes('.mpd') ? 'DASH stream' : 'HLS stream') : (m.mime || 'media file'),
    sub: (m.size ? (m.size / 1048576).toFixed(1) + ' MB · ' : '') + m.url,
    msg: { type: 'cdm-download-media', tabId: tab.id, index: i },
  }));
  if (st.mediaHost) items.unshift({ label: 'This page’s video (yt-dlp, best quality)', sub: tab.url, msg: { type: 'cdm-download-media', tabId: tab.id, pageVideo: true } });
  if (items.length) {
    box.innerHTML = '';
    items.forEach(it => {
      const b = document.createElement('button');
      b.className = 'media';
      b.innerHTML = `<span class="l"></span><span class="s"></span>`;
      b.querySelector('.l').textContent = it.label;
      b.querySelector('.s').textContent = it.sub;
      b.onclick = async () => {
        const r = await chrome.runtime.sendMessage(it.msg);
        b.querySelector('.l').textContent = r.ok ? '✔ Sent to Crazy DM' : '✖ ' + r.error;
      };
      box.appendChild(b);
    });
  }
  $('all').onclick = async () => {
    const r = await chrome.runtime.sendMessage({ type: 'cdm-all-links', tabId: tab.id });
    if (r.ok) window.close(); else $('all').textContent = '✖ ' + r.error;
  };
}

$('opts').onclick = () => chrome.runtime.openOptionsPage();
$('opts1').onclick = e => { e.preventDefault(); chrome.runtime.openOptionsPage(); };
init();
