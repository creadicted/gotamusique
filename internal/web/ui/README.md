# The web interface

Plain HTML, CSS and ES modules, embedded into the binary with `go:embed` (see `../ui.go`).
There is **no build step and no Node toolchain**: edit a file, rebuild the bot.

    index.html      the page; loads app.css and js/app.js
    app.css         all styles (light and dark, one stylesheet)
    icon.svg        the tab icon
    fonts/          Archivo (variable, SIL OFL 1.1, from @fontsource-variable/archivo 5.3.0)
    js/
      app.js        starts everything, the login screen, the page layout
      api.js        the only code that talks to the server
      store.js      polls /api/v1/state, keeps the latest state, runs mutations
      logic.js      pure functions (formatting, the playhead clock, queue moves, ...)
      decode.js     the title animation (frames are pure and tested)
      dom.js        h() builds elements; text is never parsed as HTML
      player.js     the board and its controls
      queue.js      the queue
      add.js, library.js, radio.js, url.js, upload.js, addmenu.js   the Add panel
      toast.js      short messages

## Rules the tests enforce

* Nothing is built from text as markup or code: no `innerHTML`, `eval`, `insertAdjacentHTML`...
  Station names, tags and user names come from outside and are only ever set as text.
* No inline script or style, no `style` attributes: the page's Content-Security-Policy forbids them.
  Set a style from JavaScript with `el.style...`, which the policy allows.
* Nothing is loaded from the internet.
* Only the files the page needs are served (`README.md` and licence texts are not).

## Trying it

    make ui-demo              # a fake bot with made-up music at http://127.0.0.1:8181
    make ui-demo LOGIN=1      # the same, with the !web login flow

Files are embedded at build time, so restart `make ui-demo` after editing the interface.

## Tests

* `go test ./internal/web` checks how the files are served and the rules above.
* `uitest/*.test.mjs` test the pure logic with `node --test` (run by `go test` when Node is installed;
  skipped otherwise). The interface itself never needs Node.
* The layout and behaviour were checked in a real browser (headless Chromium) against `make ui-demo`:
  every control, the keyboard, both themes, a phone width, the login flow, losing the connection,
  reduced motion and hostile text. That script is not part of the repository.

## Design

Light "timetable paper" with one dark board for what is playing; amber is only used on dark
surfaces, where it has contrast. Archivo is used at two widths: condensed bold for the board and
the queue, normal width for everything else. The one animation is the title decoding when the track
changes.
