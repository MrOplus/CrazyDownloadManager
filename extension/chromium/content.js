// In-page helpers: Alt+click bypass, the floating "Download this video"
// panel, and toasts from the background worker.
(() => {
  if (window.__cdmLoaded) return;
  window.__cdmLoaded = true;

  // Alt+click on a link = let the browser download it (like IDM).
  document.addEventListener('click', e => {
    if (!e.altKey) return;
    const a = e.target.closest && e.target.closest('a[href]');
    if (a) chrome.runtime.sendMessage({ type: 'cdm-bypass', url: a.href });
  }, true);

  let panel = null;
  let target = null;
  let hideTimer = null;

  function bestVideo() {
    let best = null, area = 0;
    document.querySelectorAll('video').forEach(v => {
      const r = v.getBoundingClientRect();
      const a = r.width * r.height;
      if (a > area && r.width > 180 && r.height > 100) { best = v; area = a; }
    });
    return best;
  }

  function place() {
    if (!panel || !target) return;
    const r = target.getBoundingClientRect();
    if (r.width === 0 || r.bottom < 0 || r.top > innerHeight) { panel.style.display = 'none'; return; }
    panel.style.display = 'flex';
    panel.style.top = `${Math.max(4, r.top + 8)}px`;
    panel.style.left = `${Math.min(innerWidth - panel.offsetWidth - 8, r.right - panel.offsetWidth - 8)}px`;
  }

  function showPanel(info) {
    const v = bestVideo();
    if (!v || (!info.media.length && !info.mediaHost)) return;
    target = v;
    if (!panel) {
      panel = document.createElement('div');
      panel.className = 'cdm-panel';
      panel.innerHTML = '<button class="cdm-main" title="Send this video to Crazy Download Manager"><span class="cdm-arrow">⬇</span><span class="cdm-label">Download this video</span></button><button class="cdm-more" title="More">▾</button><button class="cdm-close" title="Hide">×</button><div class="cdm-menu"></div>';
      document.documentElement.appendChild(panel);
      panel.querySelector('.cdm-close').onclick = () => { panel.remove(); panel = null; };
      panel.querySelector('.cdm-more').onclick = () => panel.classList.toggle('open');
      addEventListener('scroll', place, { passive: true });
      addEventListener('resize', place);
    }
    const menu = panel.querySelector('.cdm-menu');
    menu.innerHTML = '';
    const entries = info.media.map((m, i) => ({
      label: `${m.kind === 'stream' ? (m.url.includes('.mpd') ? 'DASH' : 'HLS') + ' stream' : (m.mime || 'media')}${m.size ? ' · ' + (m.size / 1048576).toFixed(1) + ' MB' : ''}`,
      run: () => chrome.runtime.sendMessage({ type: 'cdm-download-media', index: i }, done),
    }));
    if (info.mediaHost) entries.unshift({ label: 'Best quality via yt-dlp', run: () => chrome.runtime.sendMessage({ type: 'cdm-download-media', pageVideo: true }, done) });
    entries.forEach(e => { const b = document.createElement('button'); b.textContent = e.label; b.onclick = () => { panel.classList.remove('open'); e.run(); }; menu.appendChild(b); });
    panel.querySelector('.cdm-main').onclick = () => entries[0] && entries[0].run();
    panel.querySelector('.cdm-more').style.display = entries.length > 1 ? '' : 'none';
    place();
    clearTimeout(hideTimer);
  }

  function done(res) {
    toast(res && res.ok ? 'Sent to Crazy Download Manager' : 'Crazy Download Manager: ' + ((res && res.error) || 'not running'));
  }

  function toast(text) {
    const t = document.createElement('div');
    t.className = 'cdm-toast';
    t.textContent = text;
    document.documentElement.appendChild(t);
    setTimeout(() => t.remove(), 3500);
  }

  function refresh() {
    chrome.runtime.sendMessage({ type: 'cdm-media-list' }, info => {
      if (chrome.runtime.lastError || !info || !info.enabled) return;
      showPanel(info);
    });
  }

  chrome.runtime.onMessage.addListener(msg => {
    if (msg.type === 'cdm-media') refresh();
    if (msg.type === 'cdm-toast') toast(msg.text);
  });

  // Video sites load the player late; check a few times.
  [1500, 4000, 9000].forEach(t => setTimeout(refresh, t));
  document.addEventListener('play', () => setTimeout(refresh, 500), true);
})();
