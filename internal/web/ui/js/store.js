import { api, ApiError } from './api.js';
import { positionNow, sentence } from './logic.js';
import { toast } from './toast.js';

// Polls the state, keeps the latest copy and lets views subscribe. Status is one of
//   loading  nothing received yet
//   ready    state is current
//   login    the server wants a login (401)
//   offline  the bot did not answer
// Mutations return the new state, which is applied at once; the poll corrects the rest.

const FAST = 1000; // visible tab
const SLOW = 5000; // hidden tab
const LOGIN = 3000; // waiting for someone to log in

class Store {
  constructor() {
    this.status = 'loading';
    this.state = null;
    this.version = null;
    this.receivedAt = 0;
    this.failures = 0;
    this.listeners = new Set();
    this.timer = null;
    this.polling = false;
  }

  subscribe(fn) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  notify() {
    for (const fn of this.listeners) fn(this);
  }

  /** Playhead position in ms, advanced locally while the track plays. */
  position() {
    return positionNow(this.state, this.receivedAt, performance.now());
  }

  setStatus(status) {
    if (this.status === status) return false;
    this.status = status;
    return true;
  }

  apply(state) {
    this.setStatus('ready');
    if (this.state && state.version === this.version) return;
    this.state = state;
    this.version = state.version;
    this.receivedAt = performance.now();
    this.notify();
  }

  start() {
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) this.refresh();
    });
    this.refresh();
  }

  /** Poll right now, then keep going on the usual schedule. */
  refresh() {
    clearTimeout(this.timer);
    return this.poll();
  }

  async poll() {
    if (this.polling) return;
    this.polling = true;
    try {
      const state = await api.state(this.version);
      this.failures = 0;
      if (state) this.apply(state);
      else if (this.setStatus('ready')) this.notify();
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        this.failures = 0;
        this.state = null;
        this.version = null;
        if (this.setStatus('login')) this.notify();
      } else {
        this.failures += 1;
        if (this.failures >= 2 && this.setStatus('offline')) this.notify();
      }
    } finally {
      this.polling = false;
      const wait = this.status === 'login' ? LOGIN : this.status === 'offline' ? Math.min(10000, FAST * 2 ** Math.min(this.failures, 4)) : document.hidden ? SLOW : FAST;
      this.timer = setTimeout(() => this.poll(), wait);
    }
  }

  /**
   * Run a mutation. `fn` returns the API promise. On success the returned state is
   * applied. A queue entry that is already gone (404) is not an error to show: another
   * person got there first, so refresh and move on. Returns the response or null.
   */
  async act(fn, failure = '') {
    try {
      const data = await fn();
      if (data && data.version !== undefined && Array.isArray(data.queue)) this.apply(data);
      else this.refresh();
      return data;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404 && e.code === 'not_found') {
        this.refresh();
        return null;
      }
      if (e instanceof ApiError && e.status === 401) {
        this.refresh();
        return null;
      }
      // The server's message is already written for people; `failure` only adds context
      // where the message alone would not say what failed (an empty `failure` adds none).
      toast([failure, sentence(e.message)].filter(Boolean).join(' '), 'error');
      this.refresh();
      return null;
    }
  }
}

export const store = new Store();
