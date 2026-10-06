# Omacord

A text-only Discord client for Omarchy Quattro, with a native panel, channel switcher, mention indicator and voice controls. Plugin ID: `quickshell.discord`.

The plugin runs with your user permissions inside the shared Omarchy shell. The backend connects to Discord, stores a sign-in token through Secret Service, and uses a user systemd service. This is an unofficial client; use it at your own risk.

## Requirements and installation

Requires Omarchy Quattro, Quickshell, Qt Quick Controls, systemd, `secret-tool`, `libopus`, `rsync` and `jq`. Clipboard image staging uses `wl-paste`. The bundled backend is for x86_64; other architectures or changed backend sources require Go and build dependencies. Source builds may download Go modules. Never start a second Quickshell instance to run the plugin.

From this checkout:

```sh
bash scripts/install-local.sh --section right
```

This validates the plugin, installs the backend and user unit, copies source into `~/.config/omarchy/plugins/quickshell.discord`, rescans plugins and enables the widget if needed. It can restart an existing backend. The installer mirrors the destination with deletion, so back up local installed-copy changes before using it. The unit is not enabled at login by setup; the plugin manages its lifecycle.

For updates, rerun the installer after preserving drafts and local changes. A shell reload may be necessary when cached QML remains loaded; preserve unsent work in other plugins before a shell restart.

## Use

Click the bar icon to open the client. Log in by scanning the QR code with Discord on your phone, or paste a user token into the login field. Do not share the token.

- Search finds channels and direct messages, not full message history. Ctrl+K opens the same switcher.
- Help opens the complete shortcut reference. Ctrl+/ also opens it.
- Select a message, then use Reply, React or Copy below the conversation. Own messages also offer Edit and Delete. Delete requires a second activation within 3 seconds; Escape cancels it.
- Send submits a draft; Enter sends and Shift+Enter inserts a newline. Ctrl+V stages clipboard images or pastes text. Attachments have a bounded scroll area; Tab and arrows reach staged files. There is no file-browser attachment picker.
- Members appears beside the conversation in wide windows and over it in compact windows. Hover or keyboard focus reveals full member details. Close members or Escape dismisses the compact pane.
- Log out asks for confirmation before ending the session and attempting to remove its saved token. Cancel is initially focused. Close only hides the panel.
- Escape walks back through focus zones; it does not close a ready panel from the server rail.

Attachments are links, not image previews. Bar settings control connection lifetime, notifications, mention count and window persistence. Real QR login, delivery, clipboard and audio depend on the account, network and desktop; inert tests do not certify them.

## Checks and troubleshooting

```sh
node --test tests/*.test.cjs components/harness/*.test.js
node tests/no-media.cjs
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software \
  /usr/lib/qt6/bin/qmltestrunner -input tests/visual -import .
cd backend
go test -tags nolibopusfile ./...
go test -race -tags nolibopusfile ./...
go vet -tags nolibopusfile ./...
```

Offscreen fixtures use inert services and load the actual UI components. They do not test microphone/playback, phone QR approval, live account operations, compositor placement or screen-reader behavior. `TestPulseSmoke` is opt-in. Backend socket tests require loopback sockets in the test environment. A standalone Qt runner may not load Quickshell's native I/O plugin even when the shared shell does.

Inspect the user service without sending account commands:

```sh
systemctl --user status omarchy-discord.service
journalctl --user -u omarchy-discord.service -n 50
```

The bundled backend is selected only when its production-source fingerprint matches `backend/dist/x86_64/source.sha256`; otherwise the build script compiles from source. This stamp is a freshness guard, not a signature. Keep the binary and stamp synchronized after backend edits.

## Removal

Preserve drafts, then disable the plugin before removing its runtime:

```sh
omarchy plugin disable quickshell.discord
bash scripts/remove-runtime.sh
```

Runtime removal requires a successful backend stop, removes the user unit/executable and moves matching configuration into a dated backup. It does not remove the plugin source copy. `--purge` additionally deletes plugin configuration/cache and attempts to clear matching keyring entries; use it only when you intend those losses and verify keyring removal independently.

## License

MIT. See [LICENSE](LICENSE).
