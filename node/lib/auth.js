import { readFile, writeFile, mkdir } from 'fs/promises';
import { existsSync } from 'fs';
import { homedir } from 'os';
import { join } from 'path';
import { execSync } from 'child_process';

const CONFIG_DIR = join(homedir(), '.config', 'slacko');
const CREDS_FILE = join(CONFIG_DIR, 'credentials.json');

async function ensureDir() {
  if (!existsSync(CONFIG_DIR)) {
    await mkdir(CONFIG_DIR, { recursive: true, mode: 0o700 });
  }
}

export async function loadCredentials() {
  try {
    const data = await readFile(CREDS_FILE, 'utf8');
    return JSON.parse(data);
  } catch (e) {
    if (e.code === 'ENOENT') return [];
    throw new Error(`invalid credentials file: ${e.message}`);
  }
}

export async function saveCredentials(creds) {
  await ensureDir();
  await writeFile(CREDS_FILE, JSON.stringify(creds, null, 2) + '\n', { mode: 0o600 });
}

export async function addOrUpdateCredentials(cred) {
  const creds = await loadCredentials();
  const idx = creds.findIndex(c => c.workspace === cred.workspace);
  if (idx >= 0) {
    creds[idx] = cred;
  } else {
    creds.push(cred);
  }
  await saveCredentials(creds);
}

export async function findCredentials(workspace) {
  const creds = await loadCredentials();
  if (creds.length === 0) {
    throw new Error("no credentials configured. Run 'slacko auth manual' or 'slacko auth import'");
  }
  if (!workspace) return creds[0];
  const found = creds.find(c => c.workspace === workspace);
  if (!found) throw new Error(`no credentials for workspace "${workspace}"`);
  return found;
}

const SUPPORTED_BROWSERS = ['chrome', 'edge', 'brave', 'firefox', 'safari'];

export function importFromBrowser(browser, browserProfile) {
  browser = browser.toLowerCase();
  if (!SUPPORTED_BROWSERS.includes(browser)) {
    throw new Error(`unsupported browser "${browser}". Supported: ${SUPPORTED_BROWSERS.join(', ')}`);
  }

  // Try sweetcookie
  try {
    const args = ['get', '--browser', browser, '--domain', '.slack.com', '--name', 'd'];
    if (browserProfile) args.push('--browser-profile', browserProfile);
    const cookie = execSync(`sweetcookie ${args.join(' ')}`, { encoding: 'utf8' }).trim();
    if (cookie) return cookie;
  } catch {}

  const names = { chrome: 'Chrome', edge: 'Edge', brave: 'Brave', firefox: 'Firefox', safari: 'Safari' };
  throw new Error(`cookie import requires sweetcookie CLI.

Install:  npm i -g sweetcookie  (or)  go install github.com/steipete/sweetcookie/cmd/sweetcookie@latest

Or extract manually:
1. Open ${names[browser]} → app.slack.com → F12 → Application → Cookies
2. Find the 'd' cookie for .slack.com domain
3. Copy its value
4. Run: slacko auth manual --token xoxc-YOUR-TOKEN --cookie "YOUR-D-COOKIE" WORKSPACE`);
}
