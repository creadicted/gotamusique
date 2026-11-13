// A tiny DOM builder. Text always goes in as a text node, never as HTML, so a radio
// station called "<img src=x onerror=...>" is shown, not run. There is no innerHTML
// anywhere in the interface, and no inline style: the page's policy forbids both.

const SVG_NS = 'http://www.w3.org/2000/svg';

/** h('button', {class: 'x', onclick: fn, disabled: true}, 'Label', childElement) */
export function h(tag, props, ...children) {
  const el = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') el.className = value;
    else if (key === 'dataset') Object.assign(el.dataset, value);
    else if (key.startsWith('on') && typeof value === 'function') el.addEventListener(key.slice(2), value);
    else if (value === true) el.setAttribute(key, '');
    else el.setAttribute(key, String(value));
  }
  append(el, children);
  return el;
}

export function append(el, children) {
  for (const child of children.flat(Infinity)) {
    if (child === null || child === undefined || child === false) continue;
    el.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return el;
}

/** Replace everything inside `el`. */
export function render(el, ...children) {
  el.replaceChildren();
  return append(el, children);
}

// 24x24 line icons, as path data. Built with createElementNS, never parsed from text.
const ICONS = {
  play: 'M8 5v14l11-7z',
  pause: 'M7 5h3.5v14H7zM13.5 5H17v14h-3.5z',
  next: 'M6 5v14l9-7zM17 5h2.5v14H17z',
  stop: 'M6 6h12v12H6z',
  plus: 'M12 5v14M5 12h14',
  close: 'M6 6l12 12M18 6L6 18',
  up: 'M6 14l6-6 6 6',
  down: 'M6 10l6 6 6-6',
  more: 'M5 10.5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zM12 10.5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3zM19 10.5a1.5 1.5 0 1 1 0 3 1.5 1.5 0 0 1 0-3z',
  search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-4-4',
  folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
  volume: 'M4 9v6h4l5 4V5L8 9zM16.5 8.5a5 5 0 0 1 0 7M19 6a8.5 8.5 0 0 1 0 12',
  mute: 'M4 9v6h4l5 4V5L8 9zM17 9l5 6M22 9l-5 6',
  shuffle: 'M4 7h3c4 0 5 10 9 10h4M4 17h3c1.6 0 2.6-1.6 3.6-3.4M13.4 10.4C14.4 8.6 15.4 7 17 7h3M17 4l3 3-3 3M17 14l3 3-3 3',
  trash: 'M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12',
  upload: 'M12 16V5M7 9l5-5 5 5M5 19h14',
  refresh: 'M20 12a8 8 0 1 1-2.5-5.8M20 4v5h-5',
  logout: 'M10 5H6a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h4M14 8l4 4-4 4M18 12H9',
};

export function icon(name) {
  const svg = document.createElementNS(SVG_NS, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('class', 'icon');
  svg.setAttribute('aria-hidden', 'true');
  const path = document.createElementNS(SVG_NS, 'path');
  path.setAttribute('d', ICONS[name] || '');
  const filled = name === 'play' || name === 'pause' || name === 'next' || name === 'stop' || name === 'more';
  path.setAttribute('class', filled ? 'icon-fill' : 'icon-line');
  svg.append(path);
  return svg;
}

/** An icon-only button with an accessible name. */
export function iconButton(name, label, onclick, extraClass = '') {
  return h('button', { type: 'button', class: `ib ${extraClass}`.trim(), 'aria-label': label, title: label, onclick }, icon(name));
}

/** True when the visitor asked the system for less motion. */
export function reducedMotion() {
  return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}
