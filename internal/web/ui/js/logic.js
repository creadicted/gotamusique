// Pure functions: no DOM, no network. They are unit tested with `node --test`
// (internal/web/uitest), which is why they live apart from the views.

/** Format milliseconds as m:ss, or h:mm:ss from one hour up. Negative or NaN is 0:00. */
export function formatClock(ms) {
  const total = Math.max(0, Math.floor((Number.isFinite(ms) ? ms : 0) / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  const ss = String(s).padStart(2, '0');
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}

/**
 * Where the playhead is now. The server sends position_ms only when something else
 * changed, so while the track plays we advance it by the time since it arrived.
 * Returns null when there is no meaningful position (live stream, nothing playing).
 */
export function positionNow(state, receivedAt, now) {
  if (!state || state.position_ms == null) return null;
  let pos = state.position_ms;
  if (state.playing) pos += Math.max(0, now - receivedAt);
  const cur = state.queue[state.index];
  if (cur && cur.duration_ms > 0) pos = Math.min(pos, cur.duration_ms);
  return Math.max(0, pos);
}

/** The entry that is current, or null when idle or empty. */
export function currentEntry(state) {
  return state && state.index >= 0 && state.index < state.queue.length ? state.queue[state.index] : null;
}

/**
 * What the main transport button does right now.
 *   pause:  something is playing
 *   resume: paused
 *   start:  idle with entries in the queue (jump to the first one)
 *   none:   nothing to play
 */
export function transportAction(state) {
  if (!state) return { kind: 'none', label: 'Play' };
  if (state.pause) return { kind: 'resume', label: 'Resume' };
  if (currentEntry(state) && state.playing) return { kind: 'pause', label: 'Pause' };
  if (currentEntry(state)) return { kind: 'pause', label: 'Pause' }; // loading the current entry
  if (state.queue.length > 0) return { kind: 'start', label: 'Play' };
  return { kind: 'none', label: 'Play' };
}

/**
 * The request that moves queue[index] one place up (dir -1) or down (dir +1):
 * {id, after} for POST /queue/{id}/move, or null at the edge. `after` is the entry
 * to place it behind, 0 for the front.
 */
export function moveRequest(queue, index, dir) {
  if (index < 0 || index >= queue.length) return null;
  if (dir < 0) {
    if (index === 0) return null;
    return { id: queue[index].id, after: index >= 2 ? queue[index - 2].id : 0 };
  }
  if (index >= queue.length - 1) return null;
  return { id: queue[index].id, after: queue[index + 1].id };
}

/** "3 tracks, 12:30" style summary; unknown lengths are left out of the total. */
export function queueSummary(queue) {
  const count = queue.length;
  if (count === 0) return 'Empty';
  const known = queue.reduce((sum, e) => sum + (e.duration_ms > 0 ? e.duration_ms : 0), 0);
  const unknown = queue.filter((e) => !(e.duration_ms > 0)).length;
  const noun = count === 1 ? 'entry' : 'entries';
  let text = `${count} ${noun}`;
  if (known > 0) text += `, ${formatClock(known)}`;
  if (unknown > 0 && known > 0) text += ` and ${unknown} without a length`;
  return text;
}

/** Breadcrumb parts for a library folder: "rock/metal" -> rock, rock/metal. */
export function breadcrumbs(dir) {
  const parts = (dir || '').split('/').filter(Boolean);
  return parts.map((name, i) => ({ name, path: parts.slice(0, i + 1).join('/') }));
}

/** The file name part of a library path. */
export function baseName(path) {
  const i = path.lastIndexOf('/');
  return i < 0 ? path : path.slice(i + 1);
}

/** The folder part of a library path ("" for the root). */
export function dirName(path) {
  const i = path.lastIndexOf('/');
  return i < 0 ? '' : path.slice(0, i);
}

/** Text for an API failure. The server's own sentence is already written for people. */
export function errorText(err, fallback) {
  if (err && typeof err.message === 'string' && err.message) return err.message;
  return fallback || 'Something went wrong.';
}

/** Make a server message read as a sentence: first letter upper case, ends with a full stop. */
export function sentence(text) {
  const t = String(text || '').trim();
  if (!t) return '';
  const cap = t[0].toUpperCase() + t.slice(1);
  return /[.!?]$/.test(cap) ? cap : `${cap}.`;
}

/** Where a new entry goes, as the API's `position` value. */
export const POSITIONS = Object.freeze({ end: 'end', next: 'next', now: 'now' });

/** Query string for GET /library from a plain object, skipping empty values. */
export function libraryQuery({ dir, q, tags, limit, offset }) {
  const p = new URLSearchParams();
  if (dir) p.set('dir', dir);
  if (q) p.set('q', q);
  for (const t of tags || []) p.append('tag', t);
  if (limit) p.set('limit', String(limit));
  if (offset) p.set('offset', String(offset));
  const s = p.toString();
  return s ? `?${s}` : '';
}
