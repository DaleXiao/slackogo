#!/usr/bin/env node

import { Command } from 'commander';
import chalk from 'chalk';
import { Client } from '../lib/client.js';
import { Printer } from '../lib/output.js';
import {
  loadCredentials, addOrUpdateCredentials, importFromBrowser,
} from '../lib/auth.js';

const VERSION = '0.1.0';

// Exit codes
const EXIT = { OK: 0, ERROR: 1, USAGE: 2, AUTH: 3, NETWORK: 4 };

let globalOpts = {};
let printer;

function getFormat() {
  if (globalOpts.json) return 'json';
  if (globalOpts.plain) return 'plain';
  return 'human';
}

async function getClient() {
  return Client.create(globalOpts.workspace, globalOpts.timeout ? parseInt(globalOpts.timeout) : 10000);
}

function formatTS(ts) {
  const sec = parseInt(ts?.split('.')[0]);
  if (isNaN(sec)) return ts || '';
  return new Date(sec * 1000).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
}

function die(err) {
  printer.error(`Error: ${err.message}`);
  const msg = err.message;
  if (msg.includes('auth error') || msg.includes('no credentials')) process.exit(EXIT.AUTH);
  if (msg.includes('network error')) process.exit(EXIT.NETWORK);
  process.exit(EXIT.ERROR);
}

const program = new Command()
  .name('slacko')
  .description('Slack CLI tool using browser cookies')
  .version(VERSION)
  .option('--json', 'Output as JSON')
  .option('--plain', 'Output as plain tab-separated text')
  .option('--no-color', 'Disable color output')
  .option('-w, --workspace <name>', 'Select workspace')
  .option('--timeout <ms>', 'Request timeout', '10000')
  .option('-q, --quiet', 'Suppress non-essential output')
  .option('-v, --verbose', 'Verbose output')
  .option('-d, --debug', 'Debug output')
  .hook('preAction', (thisCmd) => {
    globalOpts = program.opts();
    if (globalOpts.noColor) chalk.level = 0;
    printer = new Printer(getFormat());
  });

// === Auth ===
const auth = program.command('auth').description('Manage authentication');

auth.command('import')
  .description('Import cookies from browser')
  .option('--browser <name>', 'Browser (chrome,edge,brave,firefox,safari)', 'chrome')
  .option('--browser-profile <name>', 'Browser profile name')
  .action(async (opts) => {
    try {
      printer.human(`Importing cookies from ${opts.browser}...`);
      const cookie = importFromBrowser(opts.browser, opts.browserProfile);
      printer.success('Cookie imported successfully');
      printer.human(`Cookie value (first 20 chars): ${cookie.slice(0, 20)}...`);
      printer.human(`\nYou still need to provide the xoxc- token.`);
      printer.human(`Use: slacko auth manual --token <TOKEN> --cookie '${cookie}' WORKSPACE`);
    } catch (e) { die(e); }
  });

auth.command('manual')
  .description('Manually set token and cookie')
  .requiredOption('--token <token>', 'xoxc- token')
  .requiredOption('--cookie <cookie>', 'd cookie value')
  .argument('<workspace>', 'Workspace name')
  .action(async (workspace, opts) => {
    try {
      if (!opts.token.startsWith('xoxc-')) throw new Error("token must start with 'xoxc-'");
      await addOrUpdateCredentials({ token: opts.token, cookie: opts.cookie, workspace });
      console.log(`✓ Credentials saved for workspace "${workspace}"`);
    } catch (e) { die(e); }
  });

auth.command('status')
  .description('Check authentication status')
  .action(async () => {
    try {
      const creds = await loadCredentials();
      if (creds.length === 0) {
        printer.human('No credentials configured.');
        printer.human("Run 'slacko auth manual' or 'slacko auth import' to get started.");
        return;
      }
      const entries = creds.map(c => ({
        workspace: c.workspace,
        token: c.token.slice(0, 15) + '...',
        has_cookie: !!c.cookie,
      }));
      printer.auto(entries, entries.map(e => [e.workspace, e.token, String(e.has_cookie)]), () => {
        printer.header('Configured Workspaces');
        for (const e of entries) {
          printer.human(`  ${e.workspace}: token=${e.token} cookie=${e.has_cookie}`);
        }
      });
    } catch (e) { die(e); }
  });

// === Workspace ===
const ws = program.command('workspace').description('Workspace operations');

ws.command('list')
  .description('List workspaces')
  .action(async () => {
    try {
      const client = await getClient();
      const resp = await client.teamInfo();
      const t = resp.team;
      printer.auto(
        { id: t.id, name: t.name, domain: t.domain },
        [[t.id, t.name, t.domain]],
        () => { printer.header('Workspace'); printer.human(`  ${t.name} (${t.id}) — ${t.domain}.slack.com`); }
      );
    } catch (e) { die(e); }
  });

// === Channel ===
const channel = program.command('channel').description('Channel operations');

channel.command('list')
  .description('List channels')
  .action(async () => {
    try {
      const client = await getClient();
      const resp = await client.conversationsList('public_channel,private_channel', 200);
      const chs = resp.channels;
      printer.auto(chs, chs.map(c => [c.id, c.name, String(c.num_members), c.topic?.value || '']), () => {
        printer.header(`Channels (${chs.length})`);
        for (const c of chs) {
          const pre = c.is_private ? '🔒' : '#';
          printer.human(`  ${pre}${c.name.padEnd(20)}  ${String(c.num_members).padStart(3)} members  ${c.topic?.value || ''}`);
        }
      });
    } catch (e) { die(e); }
  });

channel.command('read')
  .description('Read channel messages')
  .argument('<channel>', 'Channel name or ID')
  .option('--limit <n>', 'Number of messages', '20')
  .action(async (ch, opts) => {
    try {
      const client = await getClient();
      const chID = await client.resolveChannelID(ch);
      const resp = await client.conversationsHistory(chID, parseInt(opts.limit));
      const msgs = resp.messages;
      printer.auto(msgs, msgs.map(m => [m.ts, m.user || '', m.text]), () => {
        printer.header(`Messages in ${ch} (latest ${msgs.length})`);
        for (let i = msgs.length - 1; i >= 0; i--) {
          const m = msgs[i];
          printer.human(`  [${formatTS(m.ts)}] ${chalk.cyan(m.user || 'bot')}: ${m.text}`);
        }
      });
    } catch (e) { die(e); }
  });

channel.command('send')
  .description('Send a message to a channel')
  .argument('<channel>', 'Channel name or ID')
  .argument('<message>', 'Message text')
  .action(async (ch, message) => {
    try {
      const client = await getClient();
      const chID = await client.resolveChannelID(ch);
      await client.chatPostMessage(chID, message);
      if (!globalOpts.quiet) printer.success(`Message sent to ${ch}`);
    } catch (e) { die(e); }
  });

// === DM ===
const dm = program.command('dm').description('Direct message operations');

dm.command('list')
  .description('List DM conversations')
  .action(async () => {
    try {
      const client = await getClient();
      const resp = await client.conversationsList('im', 200);
      const chs = resp.channels;
      printer.auto(chs, chs.map(c => [c.id, c.user]), () => {
        printer.header(`DM Conversations (${chs.length})`);
        for (const c of chs) printer.human(`  ${c.id} → ${c.user}`);
      });
    } catch (e) { die(e); }
  });

dm.command('read')
  .description('Read DM messages')
  .argument('<user>', 'Username or user ID')
  .option('--limit <n>', 'Number of messages', '20')
  .action(async (user, opts) => {
    try {
      const client = await getClient();
      const userID = await client.resolveUserID(user);
      const dmID = await client.openDM(userID);
      const resp = await client.conversationsHistory(dmID, parseInt(opts.limit));
      const msgs = resp.messages;
      printer.auto(msgs, msgs.map(m => [m.ts, m.user || '', m.text]), () => {
        printer.header(`DM with ${user} (latest ${msgs.length})`);
        for (let i = msgs.length - 1; i >= 0; i--) {
          const m = msgs[i];
          printer.human(`  [${formatTS(m.ts)}] ${chalk.cyan(m.user || '')}: ${m.text}`);
        }
      });
    } catch (e) { die(e); }
  });

dm.command('send')
  .description('Send a DM')
  .argument('<user>', 'Username or user ID')
  .argument('<message>', 'Message text')
  .action(async (user, message) => {
    try {
      const client = await getClient();
      const userID = await client.resolveUserID(user);
      const dmID = await client.openDM(userID);
      await client.chatPostMessage(dmID, message);
      if (!globalOpts.quiet) printer.success(`DM sent to ${user}`);
    } catch (e) { die(e); }
  });

// === Search ===
program.command('search')
  .description('Search messages')
  .argument('<query>', 'Search query')
  .option('--limit <n>', 'Number of results', '20')
  .action(async (query, opts) => {
    try {
      const client = await getClient();
      const resp = await client.searchMessages(query, parseInt(opts.limit));
      const matches = resp.messages.matches;
      printer.auto(resp.messages, matches.map(m => [m.ts, m.channel?.name, m.username, m.text]), () => {
        printer.header(`Search results for "${query}" (${resp.messages.total} total)`);
        for (const m of matches) {
          printer.human(`  [#${chalk.yellow(m.channel?.name)}] ${chalk.cyan(m.username)}: ${m.text}`);
        }
      });
    } catch (e) { die(e); }
  });

// === Status ===
program.command('status')
  .description('Show current status')
  .action(async () => {
    try {
      const client = await getClient();
      const authResp = await client.authTest();
      const presResp = await client.usersGetPresence(authResp.user_id);
      const info = {
        user: authResp.user, user_id: authResp.user_id,
        team: authResp.team, team_id: authResp.team_id,
        presence: presResp.presence,
      };
      printer.auto(info, [[info.user, info.user_id, info.team, info.presence]], () => {
        printer.header('Status');
        printer.human(`  User:      ${info.user} (${info.user_id})`);
        printer.human(`  Team:      ${info.team} (${info.team_id})`);
        const pres = info.presence === 'active' ? chalk.green(info.presence) : chalk.yellow(info.presence);
        printer.human(`  Presence:  ${pres}`);
      });
    } catch (e) { die(e); }
  });

// === User ===
const user = program.command('user').description('User operations');

user.command('list')
  .description('List users')
  .option('--limit <n>', 'Number of users', '100')
  .action(async (opts) => {
    try {
      const client = await getClient();
      const resp = await client.usersList(parseInt(opts.limit));
      const members = resp.members;
      printer.auto(members, members.map(u => [u.id, u.name, u.real_name, `bot=${u.is_bot}`]), () => {
        printer.header(`Users (${members.length})`);
        for (const u of members) {
          if (u.deleted) continue;
          const bot = u.is_bot ? ' 🤖' : '';
          printer.human(`  ${u.name.padEnd(20)} ${(u.real_name || '').padEnd(25)} ${u.profile?.title || ''}${bot}`);
        }
      });
    } catch (e) { die(e); }
  });

user.command('info')
  .description('Show user info')
  .argument('<user>', 'Username or user ID')
  .action(async (usr) => {
    try {
      const client = await getClient();
      const userID = await client.resolveUserID(usr);
      const resp = await client.usersInfo(userID);
      const u = resp.user;
      printer.auto(u, [[u.id, u.name, u.real_name, u.profile?.email || '', u.profile?.title || '']], () => {
        printer.header('User Info');
        printer.human(`  ID:      ${u.id}`);
        printer.human(`  Name:    ${u.name}`);
        printer.human(`  Real:    ${u.real_name}`);
        if (u.profile?.title) printer.human(`  Title:   ${u.profile.title}`);
        if (u.profile?.email) printer.human(`  Email:   ${u.profile.email}`);
        if (u.profile?.status_text) printer.human(`  Status:  ${u.profile.status_emoji || ''} ${u.profile.status_text}`);
      });
    } catch (e) { die(e); }
  });

program.parse();
