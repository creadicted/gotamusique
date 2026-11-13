import { h } from './dom.js';
import { createLibrary } from './library.js';
import { createRadio } from './radio.js';
import { createUpload } from './upload.js';
import { createUrl } from './url.js';

const TABS = [
  ['library', 'Library', createLibrary],
  ['radio', 'Radio', createRadio],
  ['url', 'Web address', createUrl],
  ['upload', 'Upload', createUpload],
];

/** The Add panel: four sources in tabs. Panels are built once and kept, so a search survives switching tabs. */
export function createAdd() {
  const views = TABS.map(([id, label, make]) => ({ id, label, view: make(), button: null, panel: null }));
  let current = 'library';

  const tablist = h('div', { class: 'tabs', role: 'tablist', 'aria-label': 'Where to add from' });
  const body = h('div', { class: 'tab-panels' });

  for (const v of views) {
    v.button = h('button', { type: 'button', class: 'tab', role: 'tab', id: `tab-${v.id}`, 'aria-controls': `panel-${v.id}` }, v.label);
    v.button.addEventListener('click', () => select(v.id, true));
    v.button.addEventListener('keydown', (e) => {
      const i = views.indexOf(v);
      const to = e.key === 'ArrowRight' ? views[(i + 1) % views.length] : e.key === 'ArrowLeft' ? views[(i + views.length - 1) % views.length] : null;
      if (to) {
        e.preventDefault();
        select(to.id, true);
      }
    });
    v.panel = h('div', { class: 'tab-panel', role: 'tabpanel', id: `panel-${v.id}`, 'aria-labelledby': `tab-${v.id}` }, v.view.el);
    tablist.append(v.button);
    body.append(v.panel);
  }

  function select(id, focus) {
    current = id;
    for (const v of views) {
      const on = v.id === id;
      v.button.setAttribute('aria-selected', String(on));
      v.button.tabIndex = on ? 0 : -1;
      v.panel.hidden = !on;
      if (on) {
        v.view.show();
        if (focus) v.button.focus();
      }
    }
  }

  const el = h('section', { class: 'panel add-panel', 'aria-labelledby': 'add-heading' }, h('div', { class: 'panel-head' }, h('h2', { id: 'add-heading' }, 'Add')), tablist, body);
  select(current, false);

  return {
    el,
    update() {
      for (const v of views) if (v.view.update) v.view.update();
    },
  };
}
