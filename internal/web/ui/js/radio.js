import { api } from './api.js';
import { addMenu } from './addmenu.js';
import { h } from './dom.js';
import { errorText } from './logic.js';
import { toast } from './toast.js';

/** Radio: the bot's own stations, a search on radio-browser.info, and a direct stream URL. */
export function createRadio() {
  const presets = h('ul', { class: 'results' });
  const presetsNote = h('p', { class: 'empty' }, 'Loading your stations');
  const found = h('ul', { class: 'results' });
  const foundNote = h('p', { class: 'empty' });
  const query = h('input', { type: 'search', class: 'field', placeholder: 'Station name', 'aria-label': 'Search for radio stations', autocomplete: 'off' });
  const tag = h('input', { type: 'search', class: 'field narrow', placeholder: 'Tag, such as jazz', 'aria-label': 'Only stations with this tag', autocomplete: 'off' });
  const url = h('input', { type: 'url', class: 'field', placeholder: 'https://example.com/stream.mp3', 'aria-label': 'Stream address', autocomplete: 'off', inputmode: 'url' });
  let loaded = false;
  let ticket = 0;

  async function loadPresets() {
    try {
      const { presets: list } = await api.radioPresets();
      presets.replaceChildren(
        ...list.map((p) =>
          h('li', { class: 'result' },
            h('div', { class: 'result-main' }, h('span', { class: 'result-text' }, h('span', { class: 'result-title' }, p.comment || p.name), p.comment ? h('span', { class: 'result-sub' }, p.name) : null)),
            addMenu(() => ({ type: 'radio', preset: p.name })),
          ),
        ),
      );
      presetsNote.textContent = list.length ? '' : 'No stations are set up. Add some under [radio] in the bot’s configuration, or search below.';
    } catch (e) {
      presetsNote.textContent = `Could not load the stations. ${errorText(e)}`;
    }
  }

  async function doSearch(ev) {
    ev.preventDefault();
    const text = query.value.trim();
    const wanted = tag.value.trim();
    if (!text && !wanted) return;
    const mine = ++ticket;
    foundNote.textContent = 'Searching';
    found.replaceChildren();
    try {
      const { stations } = await api.radioSearch(text, wanted);
      if (mine !== ticket) return;
      found.replaceChildren(
        ...stations.map((s) => {
          const detail = [s.codec, s.bitrate > 0 ? `${s.bitrate} kbit/s` : '', s.country].filter(Boolean).join(', ');
          const tags = (s.tags || '').split(',').map((t) => t.trim()).filter(Boolean).slice(0, 4).join(', ');
          return h('li', { class: 'result' },
            h('div', { class: 'result-main' }, h('span', { class: 'result-text' }, h('span', { class: 'result-title' }, s.name), detail ? h('span', { class: 'result-sub' }, detail) : null, tags ? h('span', { class: 'result-sub' }, tags) : null)),
            addMenu(() => ({ type: 'radio', uuid: s.uuid })),
          );
        }),
      );
      foundNote.textContent = stations.length ? '' : 'No stations found. Try a shorter name, or another tag.';
    } catch (e) {
      if (mine !== ticket) return;
      foundNote.textContent = '';
      toast(`The station search failed. ${errorText(e)}`, 'error');
    }
  }

  const el = h(
    'div',
    { class: 'tab-body radio' },
    h('h3', {}, 'Your stations'),
    presets,
    presetsNote,
    h('h3', {}, 'Find stations'),
    h('form', { class: 'inline-form', onsubmit: doSearch }, query, tag, h('button', { type: 'submit', class: 'btn key' }, 'Search')),
    found,
    foundNote,
    h('h3', {}, 'Stream address'),
    h('div', { class: 'inline-form' }, url, addMenu(() => ({ type: 'radio', url: url.value.trim() }))),
    h('p', { class: 'hint' }, 'A direct link to a radio stream, such as an Icecast or SHOUTcast address.'),
  );

  return {
    el,
    show() {
      if (!loaded) {
        loaded = true;
        loadPresets();
      }
    },
  };
}
