import { findCredentials } from './auth.js';

export class Client {
  constructor(creds, timeout = 10000) {
    this.creds = creds;
    this.baseURL = `https://${creds.workspace}.slack.com/api/`;
    this.timeout = timeout;
  }

  static async create(workspace, timeout) {
    const creds = await findCredentials(workspace);
    return new Client(creds, timeout);
  }

  async post(method, params = {}) {
    params.token = this.creds.token;
    const body = new URLSearchParams(params).toString();

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const resp = await fetch(this.baseURL + method, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Cookie': `d=${this.creds.cookie}`,
        },
        body,
        signal: controller.signal,
      });
      clearTimeout(timer);

      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(`HTTP ${resp.status}: ${text}`);
      }

      const data = await resp.json();
      if (!data.ok) throw new Error(`slack API error: ${data.error}`);
      return data;
    } catch (e) {
      clearTimeout(timer);
      if (e.name === 'AbortError') throw new Error('network error: request timed out');
      throw e;
    }
  }

  async authTest() {
    return this.post('auth.test');
  }

  async conversationsList(types, limit = 200) {
    const params = { limit: String(limit), exclude_archived: 'true' };
    if (types) params.types = types;
    return this.post('conversations.list', params);
  }

  async conversationsHistory(channelID, limit = 20) {
    return this.post('conversations.history', { channel: channelID, limit: String(limit) });
  }

  async chatPostMessage(channelID, text) {
    return this.post('chat.postMessage', { channel: channelID, text });
  }

  async searchMessages(query, limit = 20) {
    return this.post('search.messages', { query, count: String(limit) });
  }

  async usersList(limit = 100) {
    return this.post('users.list', { limit: String(limit) });
  }

  async usersInfo(userID) {
    return this.post('users.info', { user: userID });
  }

  async teamInfo() {
    return this.post('team.info');
  }

  async usersGetPresence(userID) {
    return this.post('users.getPresence', { user: userID });
  }

  async resolveChannelID(nameOrID) {
    if (/^[CGD]/.test(nameOrID)) return nameOrID;
    const name = nameOrID.replace(/^#/, '');
    const resp = await this.conversationsList('public_channel,private_channel', 1000);
    const ch = resp.channels.find(c => c.name === name);
    if (!ch) throw new Error(`channel "${name}" not found`);
    return ch.id;
  }

  async resolveUserID(nameOrID) {
    if (/^[UW]/.test(nameOrID)) return nameOrID;
    const name = nameOrID.replace(/^@/, '');
    const resp = await this.usersList(1000);
    const user = resp.members.find(u => u.name === name || u.profile?.display_name === name);
    if (!user) throw new Error(`user "${name}" not found`);
    return user.id;
  }

  async openDM(userID) {
    const resp = await this.post('conversations.open', { users: userID });
    return resp.channel.id;
  }
}
