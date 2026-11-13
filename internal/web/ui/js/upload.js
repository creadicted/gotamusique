import { uploadFile } from './api.js';
import { addMenu } from './addmenu.js';
import { h, icon } from './dom.js';
import { errorText } from './logic.js';

/** Upload audio files into the bot's library. Files go up one at a time. */
export function createUpload() {
  const input = h('input', { type: 'file', class: 'file-input', accept: 'audio/*,.mp3,.flac,.ogg,.oga,.opus,.m4a,.aac,.wav,.wma,.ape,.aiff', multiple: true, 'aria-label': 'Choose audio files' });
  const zone = h('label', { class: 'drop' }, icon('upload'), h('span', { class: 'drop-title' }, 'Choose files or drop them here'), h('span', { class: 'hint' }, 'MP3, FLAC, OGG, OPUS, M4A, AAC, WAV and similar'), input);
  const list = h('ul', { class: 'uploads' });
  const queueOfFiles = [];
  let running = false;

  function take(files) {
    for (const f of files) {
      const bar = h('progress', { max: '1', value: '0', 'aria-label': `Uploading ${f.name}` });
      const state = h('span', { class: 'result-meta' }, 'Waiting');
      const li = h('li', { class: 'result upload' }, h('div', { class: 'result-main' }, h('span', { class: 'result-title' }, f.name), state), bar);
      list.prepend(li);
      queueOfFiles.push({ f, li, bar, state });
    }
    run();
  }

  async function run() {
    if (running) return;
    running = true;
    while (queueOfFiles.length) {
      const { f, li, bar, state } = queueOfFiles.shift();
      state.textContent = 'Uploading';
      try {
        const track = await uploadFile(f, (p) => {
          bar.value = p;
        });
        bar.remove();
        state.textContent = 'Uploaded';
        li.append(addMenu(() => ({ type: 'library', path: track.path })));
      } catch (e) {
        bar.remove();
        li.classList.add('failed');
        state.textContent = errorText(e, 'Upload failed.');
      }
    }
    running = false;
  }

  input.addEventListener('change', () => {
    take([...input.files]);
    input.value = '';
  });
  zone.addEventListener('dragover', (e) => {
    e.preventDefault();
    zone.classList.add('over');
  });
  zone.addEventListener('dragleave', () => zone.classList.remove('over'));
  zone.addEventListener('drop', (e) => {
    e.preventDefault();
    zone.classList.remove('over');
    take([...e.dataTransfer.files]);
  });

  const el = h('div', { class: 'tab-body upload' }, zone, list, h('p', { class: 'hint' }, 'Files are saved on the bot under uploads, in a folder with your name, and appear in the library straight away.'));
  return { el, show() {} };
}
