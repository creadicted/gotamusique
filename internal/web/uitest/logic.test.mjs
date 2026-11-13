import test from 'node:test';
import assert from 'node:assert/strict';
import {
  formatClock, positionNow, currentEntry, transportAction, moveRequest, queueSummary,
  breadcrumbs, baseName, dirName, errorText, libraryQuery, sentence,
} from '../ui/js/logic.js';
import { decodeFrames } from '../ui/js/decode.js';

const q = (...durations) => durations.map((d, i) => ({ id: (i + 1) * 10, title: `t${i}`, duration_ms: d, seekable: d > 0 }));
const state = (over = {}) => ({ queue: q(60000, 90000, 0), index: 0, playing: true, pause: '', position_ms: 10000, ...over });

test('formatClock', () => {
  assert.equal(formatClock(0), '0:00');
  assert.equal(formatClock(999), '0:00');
  assert.equal(formatClock(1000), '0:01');
  assert.equal(formatClock(84000), '1:24');
  assert.equal(formatClock(3599000), '59:59');
  assert.equal(formatClock(3600000), '1:00:00');
  assert.equal(formatClock(3725000), '1:02:05');
  for (const bad of [-5, NaN, undefined, null, Infinity]) assert.equal(formatClock(bad), '0:00', String(bad));
});

test('positionNow advances while playing and holds while paused', () => {
  const s = state();
  assert.equal(positionNow(s, 1000, 1000), 10000);
  assert.equal(positionNow(s, 1000, 3500), 12500);
  assert.equal(positionNow(state({ playing: false, pause: 'manual' }), 1000, 9000), 10000, 'paused: no advance');
});

test('positionNow is clamped, never negative, and null without a position', () => {
  assert.equal(positionNow(state({ position_ms: 59000 }), 0, 5000), 60000, 'clamped to the duration');
  assert.equal(positionNow(state(), 5000, 1000), 10000, 'a clock that went backwards does not rewind');
  assert.equal(positionNow(state({ position_ms: null }), 0, 5000), null);
  assert.equal(positionNow(null, 0, 0), null);
  const live = state({ index: 2, position_ms: 4000 }); // duration 0: no clamp
  assert.equal(positionNow(live, 0, 6000), 10000);
});

test('currentEntry', () => {
  assert.equal(currentEntry(state()).id, 10);
  assert.equal(currentEntry(state({ index: 3 })), null, 'past the end is idle');
  assert.equal(currentEntry(state({ queue: [] })), null);
  assert.equal(currentEntry(null), null);
});

test('transportAction picks what the main button does', () => {
  assert.equal(transportAction(state()).kind, 'pause');
  assert.equal(transportAction(state({ playing: false, pause: 'manual' })).kind, 'resume');
  assert.equal(transportAction(state({ playing: false, pause: 'auto' })).kind, 'resume');
  assert.equal(transportAction(state({ index: 3, playing: false })).kind, 'start', 'idle with entries starts the queue');
  assert.equal(transportAction(state({ queue: [], index: 0, playing: false })).kind, 'none');
  assert.equal(transportAction(null).kind, 'none');
});

test('moveRequest', () => {
  const queue = q(1, 1, 1, 1); // ids 10 20 30 40
  assert.deepEqual(moveRequest(queue, 1, -1), { id: 20, after: 0 }, 'second to the front');
  assert.deepEqual(moveRequest(queue, 3, -1), { id: 40, after: 20 }, 'up goes behind the entry two above');
  assert.deepEqual(moveRequest(queue, 0, 1), { id: 10, after: 20 }, 'down goes behind the next one');
  assert.deepEqual(moveRequest(queue, 2, 1), { id: 30, after: 40 });
  assert.equal(moveRequest(queue, 0, -1), null, 'the first cannot go up');
  assert.equal(moveRequest(queue, 3, 1), null, 'the last cannot go down');
  assert.equal(moveRequest(queue, 9, 1), null);
  assert.equal(moveRequest(queue, -1, 1), null);
  assert.equal(moveRequest([], 0, 1), null);
});

test('queueSummary', () => {
  assert.equal(queueSummary([]), 'Empty');
  assert.equal(queueSummary(q(60000)), '1 entry, 1:00');
  assert.equal(queueSummary(q(60000, 90000)), '2 entries, 2:30');
  assert.equal(queueSummary(q(60000, 0)), '2 entries, 1:00 and 1 without a length');
  assert.equal(queueSummary(q(60000, 0, 0)), '3 entries, 1:00 and 2 without a length');
  assert.equal(queueSummary(q(0, 0)), '2 entries');
});

test('library paths', () => {
  assert.deepEqual(breadcrumbs(''), []);
  assert.deepEqual(breadcrumbs('rock'), [{ name: 'rock', path: 'rock' }]);
  assert.deepEqual(breadcrumbs('rock/metal/old'), [
    { name: 'rock', path: 'rock' }, { name: 'metal', path: 'rock/metal' }, { name: 'old', path: 'rock/metal/old' },
  ]);
  assert.deepEqual(breadcrumbs('/rock//metal/'), [{ name: 'rock', path: 'rock' }, { name: 'metal', path: 'rock/metal' }]);
  assert.equal(baseName('rock/metal/song.mp3'), 'song.mp3');
  assert.equal(baseName('song.mp3'), 'song.mp3');
  assert.equal(dirName('rock/metal/song.mp3'), 'rock/metal');
  assert.equal(dirName('song.mp3'), '');
});

test('libraryQuery encodes and skips empty values', () => {
  assert.equal(libraryQuery({}), '');
  assert.equal(libraryQuery({ dir: 'rock/metal', limit: 50 }), '?dir=rock%2Fmetal&limit=50');
  assert.equal(libraryQuery({ q: 'dark & stormy', tags: ['a b', 'c'], offset: 50 }), '?q=dark+%26+stormy&tag=a+b&tag=c&offset=50');
  assert.equal(libraryQuery({ offset: 0 }), '', 'offset 0 is the default');
});

test('errorText prefers the server sentence', () => {
  assert.equal(errorText({ message: 'The stream is unreachable.' }, 'x'), 'The stream is unreachable.');
  assert.equal(errorText({ message: '' }, 'fallback'), 'fallback');
  assert.equal(errorText(null, 'fallback'), 'fallback');
  assert.equal(errorText(undefined), 'Something went wrong.');
});

test('decodeFrames ends on the exact text and keeps its shape', () => {
  let seed = 7;
  const rand = () => ((seed = (seed * 16807) % 2147483647) / 2147483647);
  for (const text of ['[Radio] Jazz Yeah !', 'Ümläut – 音楽', 'x', '']) {
    const frames = decodeFrames(text, rand);
    assert.equal(frames[frames.length - 1], text, `last frame of ${JSON.stringify(text)}`);
    for (const f of frames) assert.equal(f.length, text.length, 'every frame has the length of the text');
  }
  const f = decodeFrames('ab cd', rand);
  for (const frame of f) assert.equal(frame[2], ' ', 'spaces stay spaces');
  assert.notEqual(f[0], 'ab cd', 'the first frame is still scrambled');
});

test('decodeFrames reveals left to right', () => {
  const rand = () => 0.5; // a fixed glyph
  const text = 'abcdefghijklmnopqrstuvwxyz';
  const frames = decodeFrames(text, rand, 13);
  let lastRevealed = -1;
  for (const f of frames) {
    let revealed = 0;
    while (revealed < f.length && f[revealed] === text[revealed]) revealed++;
    assert.ok(revealed >= lastRevealed, 'the revealed prefix never shrinks');
    lastRevealed = revealed;
  }
  assert.equal(lastRevealed, text.length);
});

test('sentence capitalises and ends the server text', () => {
  assert.equal(sentence('cannot queue that: yt-dlp failed'), 'Cannot queue that: yt-dlp failed.');
  assert.equal(sentence('too many requests in progress, try again in a moment'), 'Too many requests in progress, try again in a moment.');
  assert.equal(sentence('The stream is unreachable.'), 'The stream is unreachable.');
  assert.equal(sentence('Is it playing?'), 'Is it playing?');
  assert.equal(sentence('  spaced  '), 'Spaced.');
  assert.equal(sentence(''), '');
  assert.equal(sentence(undefined), '');
  assert.equal(sentence(null), '');
});
