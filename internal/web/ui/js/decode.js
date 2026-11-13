// The one piece of motion in the interface: when the track changes, the title on the
// board decodes into place, character by character, like a departures board updating.

const GLYPHS = 'abcdefghijklmnopqrstuvwxyz0123456789';

/**
 * The frames of the effect, ending on `to` itself. Characters are revealed left to
 * right; the rest show random glyphs. Spaces stay spaces so the shape of the title
 * is readable early, and every frame has the length of `to`.
 */
export function decodeFrames(to, rand = Math.random, steps = 14) {
  const n = to.length;
  const frames = [];
  for (let s = 1; s <= steps; s++) {
    const revealed = Math.floor((n * s) / steps);
    let out = '';
    for (let i = 0; i < n; i++) {
      const c = to[i];
      out += i < revealed || c === ' ' ? c : GLYPHS[Math.floor(rand() * GLYPHS.length)];
    }
    frames.push(out);
  }
  if (frames.length === 0 || frames[frames.length - 1] !== to) frames.push(to);
  return frames;
}

/**
 * Show `text` in `el`, decoding into it unless motion is reduced. Returns a function
 * that cancels a running animation (used when the text changes again meanwhile).
 */
export function decodeInto(el, text, { reduced = false, interval = 32 } = {}) {
  if (reduced || text.length === 0) {
    el.textContent = text;
    return () => {};
  }
  const frames = decodeFrames(text);
  let i = 0;
  const timer = setInterval(() => {
    el.textContent = frames[i++];
    if (i >= frames.length) clearInterval(timer);
  }, interval);
  return () => {
    clearInterval(timer);
    el.textContent = text;
  };
}
