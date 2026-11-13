import { api } from './api.js';
import { h, icon, iconButton } from './dom.js';
import { currentEntry, formatClock, moveRequest, queueSummary } from './logic.js';
import { store } from './store.js';

/** The queue as a list of entries in play order. Returns {el, update}. */
export function createQueue() {
  const summary = h('p', { class: 'panel-note' });
  const shuffleBtn = h('button', { type: 'button', class: 'btn ghost' }, icon('shuffle'), 'Shuffle');
  const clearBtn = h('button', { type: 'button', class: 'btn ghost danger' }, icon('trash'), h('span', {}, 'Clear'));
  const list = h('ol', { class: 'queue' });
  const empty = h('p', { class: 'empty' }, 'Nothing queued. Pick something in Add.');
  const expanded = new Set(); // ids of entries whose actions are open

  shuffleBtn.addEventListener('click', () => store.act(api.shuffle, 'Could not shuffle.'));
  let confirmTimer = null;
  clearBtn.addEventListener('click', () => {
    if (confirmTimer === null) {
      clearBtn.lastChild.textContent = 'Clear everything?';
      clearBtn.classList.add('armed');
      confirmTimer = setTimeout(disarm, 3000);
      return;
    }
    disarm();
    store.act(api.clear, 'Could not clear the queue.');
  });
  function disarm() {
    clearTimeout(confirmTimer);
    confirmTimer = null;
    clearBtn.lastChild.textContent = 'Clear';
    clearBtn.classList.remove('armed');
  }

  const el = h(
    'section',
    { class: 'panel queue-panel', 'aria-labelledby': 'queue-heading' },
    h('div', { class: 'panel-head' },
      h('div', {}, h('h2', { id: 'queue-heading' }, 'Queue'), summary),
      h('div', { class: 'panel-actions' }, shuffleBtn, clearBtn),
    ),
    list,
    empty,
  );

  function row(entry, i, s, cur) {
    const isNow = cur && cur.id === entry.id;
    const done = !!cur && i < s.index;
    const open = expanded.has(entry.id);
    const dur = entry.duration_ms > 0 ? formatClock(entry.duration_ms) : '–';

    const more = h('button', { type: 'button', class: 'ib', 'aria-label': `Actions for ${entry.title}`, 'aria-expanded': String(open), title: 'Move or remove', dataset: { key: `more-${entry.id}` } }, icon('more'));
    more.addEventListener('click', () => {
      if (expanded.has(entry.id)) expanded.delete(entry.id);
      else expanded.add(entry.id);
      update();
    });

    const up = moveRequest(s.queue, i, -1);
    const down = moveRequest(s.queue, i, 1);
    const actions = h('div', { class: 'row-actions' },
      h('button', { type: 'button', class: 'btn ghost', disabled: !up, dataset: { key: `up-${entry.id}` }, onclick: () => up && store.act(() => api.move(up.id, up.after), 'Could not move it.') }, icon('up'), 'Move up'),
      h('button', { type: 'button', class: 'btn ghost', disabled: !down, dataset: { key: `down-${entry.id}` }, onclick: () => down && store.act(() => api.move(down.id, down.after), 'Could not move it.') }, icon('down'), 'Move down'),
      h('button', { type: 'button', class: 'btn ghost danger', dataset: { key: `rm-${entry.id}` }, onclick: () => { expanded.delete(entry.id); store.act(() => api.remove(entry.id), 'Could not remove it.'); } }, icon('close'), 'Remove'),
    );

    return h('li', { class: `entry${isNow ? ' now' : ''}${done ? ' done' : ''}`, 'aria-current': isNow ? 'true' : null },
      h('div', { class: 'entry-main' },
        h('span', { class: 'entry-no' }, String(i + 1)),
        h('span', { class: 'entry-title' }, entry.title),
        h('span', { class: 'entry-dur' }, dur),
        iconButton('play', `Play ${entry.title}`, () => store.act(() => api.play(entry.id), 'Could not play it.'), 'row-play'),
        more,
      ),
      open ? actions : null,
    );
  }

  function update() {
    const s = store.state;
    if (!s) return;
    // Rebuilding the list would drop keyboard focus; remember it and put it back.
    const focusKey = el.contains(document.activeElement) && document.activeElement.dataset ? document.activeElement.dataset.key : null;
    const cur = currentEntry(s);
    const ids = new Set(s.queue.map((e) => e.id));
    for (const id of expanded) if (!ids.has(id)) expanded.delete(id);

    summary.textContent = queueSummary(s.queue);
    shuffleBtn.disabled = s.queue.length < 2;
    clearBtn.disabled = s.queue.length === 0;
    empty.hidden = s.queue.length > 0;
    list.replaceChildren(...s.queue.map((e, i) => row(e, i, s, cur)));

    if (focusKey) {
      const again = el.querySelector(`[data-key="${CSS.escape(focusKey)}"]`);
      if (again && !again.disabled) again.focus();
    }
  }

  return { el, update };
}
