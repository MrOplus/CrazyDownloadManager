const $ = id => document.getElementById(id);
const DEFAULTS = { port: 47100, token: '', enabled: true, videoPanel: true, minMediaKB: 300 };

chrome.storage.local.get(Object.keys(DEFAULTS)).then(v => {
  const c = { ...DEFAULTS, ...v };
  $('token').value = c.token;
  $('port').value = c.port;
  $('enabled').checked = c.enabled;
  $('videoPanel').checked = c.videoPanel;
  $('minMediaKB').value = c.minMediaKB;
});

async function save() {
  await chrome.storage.local.set({
    token: $('token').value.trim(),
    port: parseInt($('port').value, 10) || 47100,
    enabled: $('enabled').checked,
    videoPanel: $('videoPanel').checked,
    minMediaKB: parseInt($('minMediaKB').value, 10) || 0,
  });
}

$('save').onclick = async () => { await save(); $('result').textContent = 'Saved'; };
$('test').onclick = async () => {
  await save();
  $('result').textContent = 'Testing…';
  // give the background worker a moment to reload its settings
  setTimeout(async () => {
    const r = await chrome.runtime.sendMessage({ type: 'cdm-test' });
    $('result').textContent = r.ok ? '✔ Connected to Crazy Download Manager' : '✖ ' + r.error;
    $('result').className = r.ok ? 'ok' : 'err';
  }, 300);
};
