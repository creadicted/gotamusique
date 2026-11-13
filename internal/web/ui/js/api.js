// The only place that talks to the server. Every call is same-origin, so the session
// cookie from the !web login travels with it.

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

async function call(method, path, body) {
  const init = { method, credentials: 'same-origin', cache: 'no-store', headers: {} };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  let resp;
  try {
    resp = await fetch(path, init);
  } catch (e) {
    throw new ApiError(0, 'network', 'Cannot reach the bot.');
  }
  const text = await resp.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    /* not JSON: a proxy error page, say */
  }
  if (!resp.ok) {
    throw new ApiError(resp.status, (data && data.code) || `http_${resp.status}`, (data && data.error) || `The bot answered ${resp.status}.`);
  }
  return data;
}

const enc = encodeURIComponent;

export const api = {
  // state: 304 means "unchanged since `since`"
  async state(since) {
    let resp;
    try {
      resp = await fetch(`/api/v1/state${since ? `?since=${enc(since)}` : ''}`, { credentials: 'same-origin', cache: 'no-store' });
    } catch (e) {
      throw new ApiError(0, 'network', 'Cannot reach the bot.');
    }
    if (resp.status === 304) return null;
    if (resp.status === 401) throw new ApiError(401, 'unauthenticated', 'Log in with the link from !web.');
    if (!resp.ok) throw new ApiError(resp.status, `http_${resp.status}`, `The bot answered ${resp.status}.`);
    return resp.json();
  },
  logout: () => call('POST', '/api/v1/auth/logout'),

  pause: () => call('POST', '/api/v1/playback/pause'),
  resume: () => call('POST', '/api/v1/playback/resume'),
  stop: () => call('POST', '/api/v1/playback/stop'),
  next: () => call('POST', '/api/v1/playback/next'),
  seek: (positionMs) => call('POST', '/api/v1/playback/seek', { position_ms: Math.round(positionMs) }),
  setVolume: (volume) => call('PUT', '/api/v1/volume', { volume }),
  setMuted: (muted) => call('PUT', '/api/v1/volume', { muted }),
  setMode: (mode) => call('PUT', '/api/v1/mode', { mode }),

  add: (source, position) => call('POST', '/api/v1/queue', { ...source, position }),
  clear: () => call('DELETE', '/api/v1/queue'),
  shuffle: () => call('POST', '/api/v1/queue/shuffle'),
  remove: (id) => call('DELETE', `/api/v1/queue/${id}`),
  play: (id) => call('POST', `/api/v1/queue/${id}/play`),
  move: (id, after) => call('POST', `/api/v1/queue/${id}/move`, { after }),

  radioPresets: () => call('GET', '/api/v1/radio/presets'),
  radioSearch: (q, tag = '', limit = 20) => call('GET', `/api/v1/radio/search?${q ? `q=${enc(q)}&` : ''}${tag ? `tag=${enc(tag)}&` : ''}limit=${limit}`),
  library: (query) => call('GET', `/api/v1/library${query}`),
  rescan: () => call('POST', '/api/v1/library/rescan'),
};

/** Upload one file with progress. XMLHttpRequest, because fetch cannot report upload progress. */
export function uploadFile(file, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/v1/library/upload');
    xhr.withCredentials = true;
    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total);
    });
    xhr.addEventListener('error', () => reject(new ApiError(0, 'network', 'The upload was interrupted.')));
    xhr.addEventListener('abort', () => reject(new ApiError(0, 'aborted', 'The upload was cancelled.')));
    xhr.addEventListener('load', () => {
      let data = null;
      try {
        data = JSON.parse(xhr.responseText);
      } catch {
        /* not JSON */
      }
      if (xhr.status >= 200 && xhr.status < 300 && data && data.track) resolve(data.track);
      else reject(new ApiError(xhr.status, (data && data.code) || `http_${xhr.status}`, (data && data.error) || `The bot answered ${xhr.status}.`));
    });
    const form = new FormData();
    form.append('file', file, file.name);
    xhr.send(form);
  });
}
