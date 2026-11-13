// Where the interface is served from. Behind a reverse proxy it can live under a path
// ("/music"), so every address the page uses is relative to this file's own location:
// this file is at <base>/ui/js/base.js. It is "" when the bot is served from the root.
export const BASE = new URL('../..', import.meta.url).pathname.replace(/\/$/, '');
