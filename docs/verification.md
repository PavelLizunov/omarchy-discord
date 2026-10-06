# Verification scope

The lifecycle, source-freshness, QR event-order and default text-contrast fixes are accompanied by permanent Go and Node regressions. The UI usability fixes are covered by `tests/visual/tst_usability.qml` with inert account/device operations.

## Checked candidate

The candidate retains the existing text-only native design and Escape navigation. It bounds staged attachments, exposes Send/Search/Help and selected-message actions, shows members as an overlay below the wide-layout threshold, shortens contextual hints, and confirms account logout with Cancel initially focused.

Observed checks on the candidate:

- Node: 45 tests passed, exit 0.
- Qt offscreen: 30 pass events including initialization/cleanup, exit 0. Seven usability cases cover attachment bounds/focus scrolling, click send/edit, search/help/draft preservation, compact/wide members, logout confirmation, guarded message actions and Tab traversal.
- Nine final native consumer renders were inspected: compact attachments, compact members, logout, chat, own-message actions, edit, help, emoji picker and light chat. Software/offscreen, Qt 6.11.2, explicit locale en_US, DPR1, constrained 640×420 or normal 1040×680, strict diagnostics empty.
- Ten staged attachments at 640×420 leave 157.6 logical units of timeline height, instead of the reproduced 8 units. The attachment viewport is bounded and scrolls to the focused last chip.
- Manifest validation, shell syntax, no-remote-media assertions and diff whitespace checks passed.
- QML lint exited 0 with warnings; this is not warning-free acceptance.
- Unchanged backend/runtime files matched the prior checked hashes. Their existing ordinary/race suites passed with 268 pass and 1 skip each; build/vet and isolated script/socket smoke passed. These checks are reused for unchanged sources, not claimed rerun for a UI-only change.

## Limits

This is direct coordinator review, not independent signoff. Inert tests do not establish real phone QR login, Discord delivery/edit/delete/reactions/upload, desktop clipboard or microphone/playback/calls. Pulse smoke remains opt-in. The standalone Qt runner's Quickshell I/O-plugin import gap remains recorded separately from the working installed backend lifecycle.

The render fixture loads the actual ClientView and local theme adapter with inert models. It does not run the production FloatingWindow's compositor/clipboard/link routes or certify all user themes, locales, accessibility consumers or desktop keyboard layouts. The short hint row is not a substitute for the full Help reference.

Publication of source is separate from applying UI changes to an already running shared shell. A reload unloads other plugins too; preserve their active work and Omacord drafts before applying an update. Passing tests and GitHub publication do not certify Marketplace readiness.
