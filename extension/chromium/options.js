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
  $('result').className = '';
  const r = await chrome.runtime.sendMessage({ type: 'cdm-test' }).catch(e => ({ ok: false, error: e.message }));
  $('result').textContent = r && r.ok ? '✔ Connected to Crazy Download Manager' : '✖ ' + ((r && r.error) || 'no answer');
  $('result').className = r && r.ok ? 'ok' : 'err';
};

// Firefox treats MV3 host permissions as optional and doesn't grant them at
// install time. Without them the extension can't see downloads or media.
const ALL = { origins: ['<all_urls>'] };
async function checkAccess() {
  const ok = await chrome.permissions.contains(ALL).catch(() => true);
  $('access').hidden = ok;
}
$('grant').onclick = async () => { await chrome.permissions.request(ALL).catch(() => false); checkAccess(); };
checkAccess();
