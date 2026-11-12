// The download page (/descarca/): reads /downloads/downloads.json (written
// by the release step next to the installers), turns on the buttons for the
// files that exist, and puts the visitor's own system first.
//
// downloads.json:
// { "version": "0.1.0",
//   "files": { "windows": "SADE_0.1.0_x64-setup.exe", "macos-arm": "...dmg",
//              "macos-intel": "...dmg", "linux-appimage": "...AppImage",
//              "linux-deb": "...deb" },
//   "sizes": { "windows": 4718592, ... } }

const LABEL = {
  windows: 'Descarcă pentru Windows',
  'macos-arm': 'Descarcă pentru Mac (Apple Silicon)',
  'macos-intel': 'Descarcă pentru Mac (Intel)',
  'linux-appimage': 'Descarcă pentru Linux',
  'linux-deb': 'Descarcă pentru Linux'
};

function detect() {
  const ua = navigator.userAgent;
  const plat = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || '';
  if (/iPhone|iPad|iPod/.test(ua)) return 'ios';
  if (/Android/.test(ua)) return 'android';
  if (/Win/.test(plat) || /Windows/.test(ua)) return 'windows';
  if (/Mac/.test(plat) || /Mac OS X/.test(ua)) return 'macos';
  if (/Linux/.test(plat) || /Linux/.test(ua)) return 'linux';
  return '';
}

function size(bytes) {
  if (!bytes) return '';
  const mb = bytes / (1024 * 1024);
  return mb >= 10 ? `${Math.round(mb)} MB` : `${mb.toFixed(1).replace('.', ',')} MB`;
}

async function main() {
  const os = detect();
  const row = os && document.querySelector(`.plat[data-os="${os}"]`);
  if (row) {
    row.classList.add('is-you');
    row.parentElement.prepend(row);
  }

  let manifest = null;
  try {
    const res = await fetch('/downloads/downloads.json', { cache: 'no-cache' });
    if (res.ok) manifest = await res.json();
  } catch {
    // no releases yet
  }
  const files = (manifest && manifest.files) || {};
  const sizes = (manifest && manifest.sizes) || {};

  for (const a of document.querySelectorAll('[data-file]')) {
    const f = files[a.dataset.file];
    if (f) {
      a.href = `/downloads/${encodeURIComponent(f)}`;
      a.setAttribute('download', '');
      a.removeAttribute('aria-disabled');
    } else {
      a.textContent = `${a.textContent} (în curând)`;
    }
  }

  // The hero button: this computer's file, or the closest honest action.
  const primary = document.getElementById('dl-primary');
  const meta = document.getElementById('dl-meta');
  const key = os === 'windows' ? 'windows' : os === 'macos' ? 'macos-arm' : os === 'linux' ? 'linux-appimage' : '';
  if (key && files[key]) {
    primary.href = `/downloads/${encodeURIComponent(files[key])}`;
    primary.setAttribute('download', '');
    primary.textContent = LABEL[key];
    meta.textContent = [manifest.version && `Versiunea ${manifest.version}`, size(sizes[key])].filter(Boolean).join(' · ');
  } else if (os === 'ios' || os === 'android') {
    primary.href = '/login';
    primary.textContent = 'Folosește SADE în browser';
    meta.textContent = 'Aplicația pentru telefon vine în curând.';
  } else if (key) {
    primary.href = '/login';
    primary.textContent = 'Folosește SADE în browser';
    meta.textContent = 'Versiunea pentru acest sistem vine în curând.';
  }
}

main();
