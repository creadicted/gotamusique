import { h } from './dom.js';

// Short messages at the bottom of the screen. The container is an aria-live region,
// so screen readers announce them without moving focus.

const MAX = 3;

export function toast(message, kind = 'info') {
  const host = document.getElementById('toasts');
  if (!host) return;
  const el = h('p', { class: `toast ${kind}` }, message);
  host.append(el);
  while (host.children.length > MAX) host.firstElementChild.remove();
  setTimeout(() => el.remove(), kind === 'error' ? 7000 : 3500);
}
