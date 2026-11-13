import { api } from './api.js';
import { addMenu } from './addmenu.js';
import { h, icon } from './dom.js';
import { baseName, breadcrumbs, dirName, errorText, formatClock, libraryQuery } from './logic.js';
import { store } from './store.js';
import { toast } from './toast.js';

const PAGE = 50;

/** Browse and search the music library. Returns {el, show, update}. */
export function createLibrary() {
  let dir = '';
  let q = '';
  const tags = new Set();
  let items = []; // dirs first (browse mode), then tracks
  let total = 0;
  let loaded = false;
  let unavailable = false;
  let ticket = 0; // ignores answers to requests that were superseded
  let allTags = [];
  let wasScanning = false;

  const search = h('input', { type: 'search', class: 'field', placeholder: 'Search title, artist or folder', 'aria-label': 'Search the library', autocomplete: 'off' });
  const tagBar = h('div', { class: 'tags', role: 'group', 'aria-label': 'Filter by tag', hidden: true });
  const crumbs = h('nav', { class: 'crumbs', 'aria-label': 'Folder' });
  const scanLine = h('p', { class: 'panel-note' });
  const rescanBtn = h('button', { type: 'button', class: 'btn ghost' }, icon('refresh'), 'Rescan');
  const list = h('ul', { class: 'results' });
  const note = h('p', { class: 'empty' });
  const more = h('button', { type: 'button', class: 'btn key wide', hidden: true }, 'Show more');

  let debounce = null;
  search.addEventListener('input', () => {
    clearTimeout(debounce);
    debounce = setTimeout(() => {
      q = search.value.trim();
      load(false);
    }, 250);
  });
  rescanBtn.addEventListener('click', async () => {
    const data = await store.act(api.rescan, 'Could not start the scan.');
    if (data) toast('Scanning the music folder.', 'ok');
  });
  more.addEventListener('click', () => load(true));

  const filtering = () => q !== '' || tags.size > 0;

  async function load(append) {
    const mine = ++ticket;
    const offset = append ? items.filter((i) => i.kind === 'track').length : 0;
    note.textContent = loaded ? '' : 'Loading the library';
    try {
      // A search covers the whole library: the folder is not shown while searching, so
      // limiting it silently to where you happened to be browsing would hide results.
      // Clearing the search returns to the folder you were in.
      const page = await api.library(libraryQuery({ dir: filtering() ? '' : dir, q, tags: [...tags], limit: PAGE, offset }));
      if (mine !== ticket) return;
      loaded = true;
      unavailable = false;
      allTags = page.tags;
      total = page.total;
      const fresh = [];
      if (!append) for (const d of page.dirs) fresh.push({ kind: 'dir', ...d });
      for (const t of page.tracks) fresh.push({ kind: 'track', ...t });
      items = append ? items.concat(fresh) : fresh;
    } catch (e) {
      if (mine !== ticket) return;
      if (e.code === 'library_unavailable') {
        unavailable = true;
        loaded = true;
      } else {
        toast(`Could not read the library. ${errorText(e)}`, 'error');
      }
    }
    draw();
  }

  function draw() {
    const root = el;
    root.classList.toggle('unavailable', unavailable);
    if (unavailable) {
      note.textContent = 'This bot has no music library. Set music_folder in its configuration to use one.';
      list.replaceChildren();
      more.hidden = true;
      crumbs.replaceChildren();
      tagBar.hidden = true;
      return;
    }

    // tags
    tagBar.hidden = allTags.length === 0;
    tagBar.replaceChildren(
      ...allTags.map((t) => {
        const on = tags.has(t);
        const b = h('button', { type: 'button', class: `chip${on ? ' on' : ''}`, 'aria-pressed': String(on) }, t);
        b.addEventListener('click', () => {
          if (tags.has(t)) tags.delete(t);
          else tags.add(t);
          load(false);
        });
        return b;
      }),
    );

    // where we are
    if (filtering()) {
      crumbs.replaceChildren(h('span', { class: 'crumb-here' }, total === 1 ? '1 result' : `${total} results`));
    } else {
      const parts = breadcrumbs(dir);
      // At the top there is nowhere to go back to, so "Library" is just the place name.
      const root = parts.length === 0 ? h('span', { class: 'crumb-here', 'aria-current': 'location' }, 'Library') : h('button', { type: 'button', class: 'crumb', onclick: () => go('') }, 'Library');
      crumbs.replaceChildren(
        root,
        ...parts.flatMap((p, i) => [
          h('span', { class: 'crumb-sep', 'aria-hidden': 'true' }, '/'),
          i === parts.length - 1 ? h('span', { class: 'crumb-here', 'aria-current': 'location' }, p.name) : h('button', { type: 'button', class: 'crumb', onclick: () => go(p.path) }, p.name),
        ]),
      );
    }

    list.replaceChildren(...items.map(rowFor));
    const tracks = items.filter((i) => i.kind === 'track').length;
    more.hidden = tracks >= total;
    note.textContent = items.length === 0 && loaded ? (filtering() ? 'Nothing matches. Try other words or tags.' : dir ? 'This folder is empty.' : 'The library is empty. Rescan to read the music folder.') : '';
  }

  function go(path) {
    dir = path;
    load(false);
  }

  function rowFor(item) {
    if (item.kind === 'dir') {
      return h('li', { class: 'result dir' },
        h('button', { type: 'button', class: 'result-main', onclick: () => go(item.path) },
          icon('folder'),
          h('span', { class: 'result-title' }, item.name),
          h('span', { class: 'result-meta' }, item.tracks === 1 ? '1 track' : `${item.tracks} tracks`),
        ),
      );
    }
    const where = filtering() ? dirName(item.path) : '';
    const sub = [item.artist, where].filter(Boolean).join(', ');
    return h('li', { class: 'result' },
      h('div', { class: 'result-main' },
        h('span', { class: 'result-text' },
          h('span', { class: 'result-title' }, item.title || baseName(item.path)),
          sub ? h('span', { class: 'result-sub' }, sub) : null,
        ),
        h('span', { class: 'result-meta' }, item.duration_ms > 0 ? formatClock(item.duration_ms) : ''),
      ),
      addMenu(() => ({ type: 'library', path: item.path })),
    );
  }

  const el = h(
    'div',
    { class: 'tab-body library' },
    h('div', { class: 'search-row' }, h('span', { class: 'search-icon', 'aria-hidden': 'true' }, icon('search')), search),
    tagBar,
    h('div', { class: 'scan-row' }, scanLine, rescanBtn),
    crumbs,
    list,
    note,
    more,
  );

  function update() {
    const scan = store.state && store.state.scan;
    if (!scan) return;
    rescanBtn.disabled = !scan.available || scan.running;
    if (scan.running) scanLine.textContent = 'Scanning the music folder';
    else if (scan.last_error) scanLine.textContent = `The last scan failed: ${scan.last_error}`;
    else if (scan.last_result) scanLine.textContent = `Last scan: ${scan.last_result}`;
    else scanLine.textContent = scan.available ? '' : 'No music library';
    if (wasScanning && !scan.running && loaded) load(false); // a scan finished: show what it found
    wasScanning = scan.running;
  }

  return {
    el,
    update,
    show() {
      if (!loaded) load(false);
    },
  };
}
