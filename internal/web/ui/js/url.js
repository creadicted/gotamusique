import { addMenu } from './addmenu.js';
import { h } from './dom.js';

/** Add a single track from a web address, resolved by yt-dlp. */
export function createUrl() {
  const url = h('input', { type: 'url', class: 'field', placeholder: 'https://', 'aria-label': 'Track address', autocomplete: 'off', inputmode: 'url' });
  const el = h(
    'div',
    { class: 'tab-body url' },
    h('h3', {}, 'Track from a web address'),
    h('div', { class: 'inline-form' }, url, addMenu(() => ({ type: 'url', url: url.value.trim() }))),
    h('p', { class: 'hint' }, 'One track per address; playlists are not accepted. This works for many sites, such as SoundCloud and Bandcamp. YouTube often refuses servers, so it may fail there.'),
  );
  return { el, show() {} };
}
