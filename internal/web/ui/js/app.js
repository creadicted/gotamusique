import { createAdd } from './add.js';
import { api } from './api.js';
import { h, icon, render } from './dom.js';
import { currentEntry } from './logic.js';
import { createPlayer } from './player.js';
import { createQueue } from './queue.js';
import { store } from './store.js';

const app = document.getElementById('app');
let shell = null; // the main screen, built once and then updated in place
let mounted = null; // which screen is on the page: 'main', 'login' or 'boot'

function buildShell() {
  const player = createPlayer();
  const queue = createQueue();
  const add = createAdd();
  const who = h('span', { class: 'who' });
  const banner = h('p', { class: 'banner', role: 'alert', hidden: true }, 'Cannot reach the bot. Trying again');

  const logout = h('button', { type: 'button', class: 'btn ghost', 'aria-label': 'Log out', title: 'Log out' }, icon('logout'), h('span', { class: 'label' }, 'Log out'));
  logout.addEventListener('click', async () => {
    try {
      await api.logout();
    } catch {
      /* logging out never fails in a way that matters */
    }
    store.refresh();
  });

  const top = h('header', { class: 'top' }, h('p', { class: 'brand' }, 'gotamusique'), h('span', { class: 'top-gap' }), who, h('a', { class: 'btn ghost', href: '/api/docs', title: 'Interactive API documentation' }, 'API'), logout);

  // On a narrow screen only one of Queue and Add shows at a time.
  const main = h('main', { class: 'grid', dataset: { view: 'queue' } });
  const viewTabs = h('div', { class: 'viewtabs', role: 'group', 'aria-label': 'Show' });
  const viewButtons = ['queue', 'add'].map((v) => {
    const b = h('button', { type: 'button', class: 'viewtab', 'aria-pressed': String(v === 'queue') }, v === 'queue' ? 'Queue' : 'Add');
    b.addEventListener('click', () => {
      main.dataset.view = v;
      for (const [i, other] of viewButtons.entries()) other.setAttribute('aria-pressed', String((i === 0 ? 'queue' : 'add') === v));
    });
    viewTabs.append(b);
    return b;
  });
  main.append(player.el, viewTabs, queue.el, add.el);

  return { nodes: [banner, top, main], banner, who, player, queue, add };
}

function loginScreen() {
  return h(
    'main',
    { class: 'login' },
    h('h1', {}, 'Log in with the bot'),
    h('ol', { class: 'steps' },
      h('li', {}, 'In the Mumble channel the bot is in, type ', h('code', {}, '!web'), '.'),
      h('li', {}, 'The bot sends you a private message with a link. Open it in this browser.'),
      h('li', {}, 'You land back here, logged in.'),
    ),
    h('p', { class: 'hint' }, 'The link works once and expires after five minutes. If the bot does not answer, check that the web interface is switched on in its configuration.'),
  );
}

function paint() {
  if (store.status === 'login') {
    if (mounted !== 'login') {
      render(app, loginScreen());
      mounted = 'login';
      document.title = 'Log in - gotamusique';
    }
    return;
  }
  if (!store.state) {
    if (mounted === null) {
      render(app, h('p', { class: 'boot' }, store.status === 'offline' ? 'Cannot reach the bot. Trying again' : 'Loading'));
      mounted = 'boot';
    }
    return;
  }
  if (mounted !== 'main') {
    if (!shell) shell = buildShell();
    render(app, ...shell.nodes);
    mounted = 'main';
  }
  const s = store.state;
  shell.banner.hidden = store.status !== 'offline';
  shell.who.textContent = s.me.name;
  shell.player.update();
  shell.queue.update();
  shell.add.update();
  const cur = currentEntry(s);
  document.title = cur ? `${cur.title} - gotamusique` : 'gotamusique';
}

store.subscribe(paint);
setInterval(() => {
  if (mounted === 'main') shell.player.tick();
}, 250);

// An open "Add" menu closes when you click elsewhere or press Escape.
document.addEventListener('click', (e) => {
  for (const d of document.querySelectorAll('details.menu[open]')) if (!d.contains(e.target)) d.open = false;
});
document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  for (const d of document.querySelectorAll('details.menu[open]')) {
    d.open = false;
    d.querySelector('summary')?.focus();
  }
});

store.start();
