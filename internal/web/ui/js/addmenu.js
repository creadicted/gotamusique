import { api } from './api.js';
import { h } from './dom.js';
import { store } from './store.js';
import { toast } from './toast.js';
import { POSITIONS } from './logic.js';

/**
 * The Add control used by every source: a main button that queues at the end, and a
 * small menu with "Play next" and "Play now". `getSource` returns the API source
 * object ({type, path | preset | url | uuid}) when the button is pressed.
 */
export function addMenu(getSource, { label = 'Add' } = {}) {
  const main = h('button', { type: 'button', class: 'btn key' }, label);
  const menu = h('details', { class: 'menu' });
  const summary = h('summary', { class: 'btn key menu-toggle', 'aria-label': 'More ways to add', title: 'More ways to add' }, h('span', { class: 'caret', 'aria-hidden': 'true' }));
  const nextBtn = h('button', { type: 'button', class: 'menu-item' }, 'Play next');
  const nowBtn = h('button', { type: 'button', class: 'menu-item' }, 'Play now');
  menu.append(summary, h('div', { class: 'menu-list' }, nextBtn, nowBtn));

  let busy = false;
  async function add(position) {
    if (busy) return;
    busy = true;
    menu.open = false;
    main.disabled = true;
    main.textContent = 'Adding';
    // No prefix: the server's reason ("cannot queue that: ...", "too many requests ...") says it.
    const data = await store.act(() => api.add(getSource(), position));
    busy = false;
    main.disabled = false;
    if (data && data.added) {
      main.textContent = 'Added';
      toast(`${position === POSITIONS.now ? 'Playing' : position === POSITIONS.next ? 'Playing next' : 'Added'}: ${data.added.title}`, 'ok');
      setTimeout(() => {
        if (!busy) main.textContent = label;
      }, 1400);
    } else {
      main.textContent = label;
    }
  }
  main.addEventListener('click', () => add(POSITIONS.end));
  nextBtn.addEventListener('click', () => add(POSITIONS.next));
  nowBtn.addEventListener('click', () => add(POSITIONS.now));

  return h('span', { class: 'addmenu' }, main, menu);
}
