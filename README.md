# Omacord

A text-only Discord client for Omarchy Quattro, with a native panel, channel switcher, mention indicator and voice controls. Plugin ID: `quickshell.discord`.

The plugin runs with your user permissions inside the shared Omarchy shell. The backend connects to Discord, stores a sign-in token through Secret Service, and uses a user systemd service. This is an unofficial client; use it at your own risk.

## First beta: 0.1.0

This is the first Omacord beta checkpoint, published as GitHub tag `v0.1.0` and marked as a prerelease. It is a development baseline, not a stable-release certification. See [CHANGELOG.md](CHANGELOG.md) for included changes and known limitations.

The release includes a complete plugin archive with the tested Linux x86_64 backend, plus the backend binary and SHA-256 checksums as separate assets. The binary alone is not the plugin; extract the plugin archive and use the installer below. Voice audio and recovery still need live acceptance. Video, participant streams and remote image previews are not supported.

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

- Search finds servers, text channels, voice rooms and direct messages, not full message history. Ctrl+K opens the same switcher; category buttons and result labels identify what is being searched. Tab and Shift+Tab move between categories and the search input; Return or Space selects a category, and arrow keys move through results while the input is focused.
- The toolbar uses icon buttons with descriptive tooltips and accessible names. Compact switches to a 520×560 window with persistent voice-room controls within the channel-content area whenever a call is active or has failed. Full opens a wider 1040×680 floating window instead of restoring a potentially smaller tile. Switching sizes preserves the visible conversation and does not change the voice connection. Drafts and the conversation stay in the same client. Server-name hover/cursor tooltips are disabled.
- Compact and narrow windows start with servers and the selected server's channel tree side by side. Clicking a server only lists its text and voice channels; it never opens the first or remembered chat. Select a channel explicitly to open its conversation, then use Channels or Escape to return. Reopening the compact client starts at navigation unless an explicit channel link was requested. Existing drafts are retained.
- Direct Messages and Archived servers are icon navigation links above a separated Servers section, not server tiles. Find server filters server names. Visible icon buttons select All, Unread, Mentions, Voice or Archive. Mentions filters servers with a nonzero mention counter, not only personal tags. Channel buttons select All, Text, Voice or Unread. Tab reaches the visible filters; Return or Space applies them. Channel navigation and activation use the filtered list. The slider icon exposes additional search and sorting; sort by Discord order, name, mentions or approximate online count. Online counts load only on request, are snapshots rather than chat activity, and failed/missing counts remain No data. Each refresh is a serial sweep capped at 200 servers or 2 minutes; remaining servers have no data.
- Right-click a server, or press the Menu key or Shift+F10 from its rail cursor, for Server settings, Mark all as read, Archive/Unarchive server and Leave server. Archiving moves inactive servers out of the main list into an Archived section in the rail and filter dropdown. Unarchiving restores the server. Settings currently expose account-wide mute/unmute, not administration or roles. Leaving requires an explicit confirmation; server owners must transfer ownership first. Mark all as read acknowledges visible cached server channels at their last known message; errors may leave a partially acknowledged server. Account actions depend on Discord and are not exercised by inert tests.
- The Options button opens an account card with shortcuts reference and guarded Log out, keeping destructive and secondary actions away from the primary toolbar.
- Voice channels also support text chat: selecting a voice channel in the list or search opens its messages without connecting to voice. Join voice above the messages connects only when explicitly pressed. Clicking Chat or the room title in the call bar opens its text channel beside the call controls without changing the call.
- Navigation follows servers, the selected server's channels, then channel content. In Full, the voice room's people and controls occupy the left side of that content area, not the server or channel list. In narrow browsing, the server rail keeps its readable width and the retained call sits below the selected server's channel tree. In narrow chat, people and controls remain to the left of the messages. They never overlay navigation. Compact suppresses the optional server-wide member column during a call, not the voice panel. The room list follows voice membership events and remains visible without a text channel or when another server is selected. Missing membership data is marked unavailable.
- A failed voice session retains its room panel with a Disconnected status and an explicit Reconnect button. Room members are not labeled as an active call while disconnected. An invalid voice session (4006) triggers at most one fresh-session recovery per explicit join, preserving the room and mute/deafen flags. A repeat failure stops at Disconnected; Reconnect starts a new user-requested attempt. Leaving cancels stale recovery. The exact gateway error remains in the reconnect tooltip. Error 4006 means the voice session is no longer valid, not that Discord login expired.
- The optional server Members list appears beside the conversation when there is room. It is hidden while browsing compact server/channel navigation and returns with the conversation. The people icon shows or hides it; narrow active calls reserve that space for the persistent left voice panel. Narrow lists wrap names and omit activity subtitles. Hover or keyboard focus reveals full member details. Escape returns focus to the composer.
- Read acknowledgements wait for Discord before updating local read state. Failed acknowledgements surface an error rather than reporting local success. Older local acknowledgement requests do not move the read boundary backwards; new messages can still make a channel unread.
- Failed actions show a bounded, scrollable error message with a Dismiss error button. Dismissing only clears the message: it does not retry the action, leave a call or discard a draft. A subsequent failure shows its message again.
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
