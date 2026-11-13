import { api } from './api.js';
import { decodeInto } from './decode.js';
import { h, icon, reducedMotion } from './dom.js';
import { currentEntry, formatClock, transportAction } from './logic.js';
import { store } from './store.js';

const MODES = [
  ['one-shot', 'One-shot'],
  ['repeat', 'Repeat'],
  ['random', 'Random'],
  ['autoplay', 'Autoplay'],
];

/** The now-playing board and the transport controls. Returns {el, update}. */
export function createPlayer() {
  // --- title: the one place that animates
  const titleText = h('span', { class: 'board-title-text', 'aria-hidden': 'true' }, '');
  const title = h('h1', { class: 'board-title' }, titleText);
  const status = h('p', { class: 'board-status' });
  const clock = h('p', { class: 'board-clock' });
  let shown = null; // what the title currently shows, to animate only real changes
  let cancelDecode = () => {};

  // --- position
  const scrub = h('input', { type: 'range', class: 'scrub', min: '0', max: '0', step: '1000', value: '0', 'aria-label': 'Position in the track' });
  const live = h('div', { class: 'live', hidden: true }, h('span', { class: 'live-dot', 'aria-hidden': 'true' }), 'Live stream');
  let scrubbing = false;

  // --- transport
  const playBtn = h('button', { type: 'button', class: 'btn key big', 'aria-label': 'Play' }, icon('play'));
  const nextBtn = h('button', { type: 'button', class: 'btn key', 'aria-label': 'Skip to the next entry', title: 'Skip to the next entry' }, icon('next'));
  const stopBtn = h('button', { type: 'button', class: 'btn key', 'aria-label': 'Stop and go back to the first entry', title: 'Stop and go back to the first entry' }, icon('stop'));

  // --- volume
  const muteBtn = h('button', { type: 'button', class: 'btn ghost', 'aria-label': 'Mute', 'aria-pressed': 'false' }, icon('volume'));
  const volume = h('input', { type: 'range', class: 'vol', min: '0', max: '100', step: '1', value: '0', 'aria-label': 'Volume' });
  const volText = h('span', { class: 'vol-text' }, '0');
  let volumeDragging = false;
  let volTimer = null;
  let volPending = null; // a value waiting for the next send
  let volInFlight = 0; // requests sent and not yet answered
  let volTouchedAt = -Infinity; // when the person last moved the slider
  // The slider belongs to the person until their changes have been sent, answered and
  // settled: an answer to an earlier request must not snap it back under their finger.
  const volumeIsMine = () => volumeDragging || volPending !== null || volInFlight > 0 || performance.now() - volTouchedAt < 800;

  // --- mode
  const modeInputs = MODES.map(([value, text]) => {
    const input = h('input', { type: 'radio', name: 'mode', value });
    const label = h('label', { class: 'seg' }, input, h('span', {}, text));
    return { value, input, label };
  });
  const modes = h('div', { class: 'segmented', role: 'radiogroup', 'aria-label': 'Playback mode' }, modeInputs.map((m) => m.label));

  // --- events
  playBtn.addEventListener('click', () => {
    const a = transportAction(store.state);
    if (a.kind === 'pause') store.act(api.pause, 'Could not pause.');
    else if (a.kind === 'resume') store.act(api.resume, 'Could not resume.');
    else if (a.kind === 'start') store.act(() => api.play(store.state.queue[0].id), 'Could not start the queue.');
  });
  nextBtn.addEventListener('click', () => store.act(api.next, 'Could not skip.'));
  stopBtn.addEventListener('click', () => store.act(api.stop, 'Could not stop.'));
  muteBtn.addEventListener('click', () => store.act(() => api.setMuted(!store.state.muted), 'Could not change the mute.'));

  scrub.addEventListener('input', () => {
    scrubbing = true;
    clock.textContent = `${formatClock(Number(scrub.value))} / ${formatClock(Number(scrub.max))}`;
  });
  scrub.addEventListener('change', async () => {
    await store.act(() => api.seek(Number(scrub.value)), 'Could not seek.');
    scrubbing = false;
  });

  function sendVolume() {
    volTimer = null;
    if (volPending === null) return;
    const v = volPending;
    volPending = null;
    volInFlight += 1;
    store.act(() => api.setVolume(v / 100), 'Could not set the volume.').finally(() => {
      volInFlight -= 1;
    });
    volTimer = setTimeout(sendVolume, 150); // at most one request per 150 ms while dragging
  }
  volume.addEventListener('input', () => {
    volumeDragging = true;
    volTouchedAt = performance.now();
    volText.textContent = volume.value;
    volPending = Number(volume.value);
    if (volTimer === null) sendVolume();
  });
  volume.addEventListener('change', () => {
    volumeDragging = false;
  });

  for (const m of modeInputs) {
    m.input.addEventListener('change', () => {
      if (m.input.checked) store.act(() => api.setMode(m.value), `Could not switch to ${m.value}.`);
    });
  }

  const board = h(
    'section',
    { class: 'board', 'aria-label': 'Now playing' },
    h('div', { class: 'board-head' }, title, h('div', { class: 'board-line' }, status, clock)),
    h('div', { class: 'board-scrub' }, scrub, live),
    h('div', { class: 'board-controls' },
      h('div', { class: 'transport' }, playBtn, nextBtn, stopBtn),
      h('div', { class: 'volume' }, muteBtn, volume, volText),
      modes,
    ),
  );

  function setTitle(text, key) {
    if (shown === key) return;
    const first = shown === null;
    shown = key;
    cancelDecode();
    title.setAttribute('aria-label', text);
    if (first) {
      titleText.textContent = text; // the first title just appears; only changes animate
      cancelDecode = () => {};
    } else {
      cancelDecode = decodeInto(titleText, text, { reduced: reducedMotion() });
    }
  }

  function update() {
    const s = store.state;
    if (!s) {
      setTitle('', 'none');
      return;
    }
    const cur = currentEntry(s);
    const idle = !cur;
    const key = cur ? `${cur.id}:${cur.title}` : 'idle';
    setTitle(cur ? cur.title : 'Nothing playing', key);
    board.classList.toggle('idle', idle);
    board.classList.toggle('paused', !!s.pause);

    const isLive = !!cur && s.position_ms == null && !cur.seekable && cur.duration_ms === 0 && s.playing;
    let text;
    if (idle) text = s.queue.length ? 'Press play to start the queue.' : 'The queue is empty. Add a station or a track.';
    else if (s.pause === 'auto') text = 'Paused because the channel is empty.';
    else if (s.pause) text = 'Paused';
    else if (s.playing) text = isLive ? 'On air' : 'Playing';
    else text = 'Starting';
    status.textContent = text;

    // transport
    const a = transportAction(s);
    const wanted = a.kind === 'pause' ? 'pause' : 'play';
    if (playBtn.dataset.icon !== wanted) {
      playBtn.dataset.icon = wanted;
      playBtn.replaceChildren(icon(wanted));
    }
    playBtn.setAttribute('aria-label', a.label);
    playBtn.title = a.label;
    playBtn.disabled = a.kind === 'none';
    nextBtn.disabled = idle;
    stopBtn.disabled = idle && !s.queue.length;

    // position
    const dur = cur ? cur.duration_ms : 0;
    const canSeek = !!cur && cur.seekable && dur > 0;
    scrub.hidden = !canSeek;
    live.hidden = !isLive;
    if (canSeek) scrub.max = String(dur);
    scrub.disabled = !canSeek;

    // volume
    muteBtn.replaceChildren(icon(s.muted || s.volume === 0 ? 'mute' : 'volume'));
    muteBtn.setAttribute('aria-pressed', String(s.muted));
    muteBtn.setAttribute('aria-label', s.muted ? 'Unmute' : 'Mute');
    board.classList.toggle('muted', s.muted);
    if (!volumeIsMine()) {
      volume.value = String(Math.round(s.volume * 100));
      volText.textContent = volume.value;
    }

    // mode
    for (const m of modeInputs) {
      m.input.checked = s.mode === m.value;
      const blocked = m.value === 'autoplay' && !s.scan.available;
      m.input.disabled = blocked;
      m.label.title = blocked ? 'Autoplay needs a music library' : '';
    }
    tick();
  }

  /** Runs several times a second: advances the clock and the scrubber between polls. */
  function tick() {
    const s = store.state;
    const cur = s && currentEntry(s);
    if (!cur) {
      clock.textContent = '';
      return;
    }
    const pos = store.position();
    if (pos === null) {
      clock.textContent = cur.duration_ms > 0 ? formatClock(cur.duration_ms) : '';
      return;
    }
    if (scrubbing) return;
    scrub.value = String(Math.min(pos, Number(scrub.max) || pos));
    clock.textContent = cur.duration_ms > 0 ? `${formatClock(pos)} / ${formatClock(cur.duration_ms)}` : formatClock(pos);
  }

  return { el: board, update, tick };
}
