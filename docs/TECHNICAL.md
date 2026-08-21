# Technical notes

Implementation, development and deep-troubleshooting detail kept out of the
user-facing README. `docs/CONVENTIONS.md` is the mechanics contract (every API name
verified against the installed shell), `docs/BACKEND_PROTOCOL.md` the socket
protocol, `docs/PLAN.md` the scope.

## Architecture

Omarchy Discord runs as a plugin inside Omarchy's existing `omarchy-shell`
Quickshell process. It provides a shared service, a bar widget (one per monitor) and
a lazy-loaded panel, plus a service-owned quick-switcher overlay. There is no web
view, no second shell process and no resident helper beyond the backend.

The Discord session lives in `omarchy-discord-backend`, a static Go binary built on
arikawa (gateway + REST) and ningen (the client-behaviour layer: read state, mention
counts, guild subscriptions, member lists). A static systemd user unit supervises
it; the unit has no `[Install]` section, so it is never enabled at login — the
enabled plugin starts it and, with `stayConnected` On, restarts it within 5 s if it
dies. The backend is the single source of truth; QML holds a mirror fed by socket
events and never authoritative state.

```
BarWidget.qml (badge · presence dot)    Panel.qml (full client, FloatingWindow)    QuickSwitch.qml (Ctrl+K overlay)
                            ▲ bind to Service state / call Service functions ▲
Service.qml — BackendClient (socket) · state mirror · media cache mirror · notifications · member subscription
                            ▲ $XDG_RUNTIME_DIR/omarchy-discord/backend.sock — JSON lines, protocol v1 ▲
omarchy-discord-backend — arikawa gateway + REST · ningen read state / member lists · media cache · QR auth
                            ▲ wss gateway + REST (official-client identify) ▲
Discord
```

The socket is owner-only (0600) in an owner-only runtime directory; the filesystem
is the auth boundary. Framing is one UTF-8 JSON object per line, caller-chosen
integer ids, a versioned envelope, stable machine-readable error codes and a full
state snapshot on connect (`state_changed`, then `guilds_synced`). Snowflakes are
strings on the wire: a 64-bit id does not survive a JavaScript double.

## File map

| File | Role |
|---|---|
| `manifest.json` | Plugin id `quickshell.discord`, kinds `service` / `bar-widget` / `panel`, settings schema |
| `Service.qml` | Socket client owner, state mirror (structure, channels, threads, messages, read state, typers, member list, server emoji), media cache mirror, notification fan-out, quick switcher loader, IPC handlers (`quickshell.discord.panel`, `.switcher`) |
| `BackendClient.qml` | Reconnecting `Socket` behind a `Loader` (Quickshell sockets cannot reconnect in place), request correlation, redaction |
| `DaemonManager.qml` | `scripts/backend-runtime.sh` shim: install check, unit start/stop/status |
| `BarWidget.qml` | Per-monitor mark + mention badge; left click panel, middle click configured action |
| `Panel.qml` | Login / status screens, header, rail + channel list (threads), timeline column, composer, member pane, footer hints; every zone's keyboard routing |
| `QuickSwitch.qml` | Full-screen overlay (`PanelWindow`, layer `Overlay`) with the Exclusive→OnDemand keyboard prime |
| `components/Timeline.qml` | Virtualized message list with a diffed `ListModel`, history paging, cursor, spoilers, ack-on-bottom |
| `components/MessageRow.qml` | One message: avatar, header, reply line, rich text, attachments, embeds, reactions |
| `components/Composer.qml` | Text input zone: send / reply / edit / paste / chips, key interception on the TextArea |
| `components/AttachmentChip.qml` | Staged upload chip with progress |
| `components/MemberList.qml` | Member pane: group headers, member rows, presence dots, its own roving cursor |
| `components/EmojiPicker.qml` | Modal picker: reactions → frequent → server emoji per guild → catalogue |
| `components/Cheatsheet.qml` | Modal `Ctrl+/` overlay rendered from `Keymap.js` |
| `Api.js` | Pure helpers: redaction, id comparison, channel filters, thread counts, member rows, theme contrast helpers |
| `Markdown.js` | Discord markdown → Qt rich text (never throws on hostile input) |
| `Emoji.js` | Picker model, catalogue parsing, frequent-emoji persistence |
| `Keymap.js` | The one key table: footer states and cheatsheet sections both render from it |
| `backend/` | Go module: `cmd/omarchy-discord-backend`, `internal/{socket,protocol,session,media,remoteauth,keyring,redact}` |
| `systemd/omarchy-discord.service` | Static user unit with the hardening block |
| `scripts/` | `setup.sh`, `build-backend.sh`, `install-local.sh`, `backend-runtime.sh`, `keyring-store.sh`, `remove-runtime.sh` |
| `components/harness/` | Offscreen Timeline harness and the Markdown node tests |

## State flow

Everything the user sees is a binding on `Service.qml` properties, replaced
wholesale (copy → assign) so QML reactivity fires; nothing authoritative lives in
the panel, which is destroyed on hide.

- **Session**: `state_changed` → `backendState`; `lifecycle` derives `ready`,
  `loggedOut`, `showStructure` (a gateway reconnect is hidden behind a 3 s grace).
  Stale generations are dropped (`acceptGeneration`).
- **Structure**: `guilds_synced` replaces `guilds` / `dms`; `list_channels` fills
  `channelsByGuild[guildId]` lazily per selected guild. `list_channels` carries every
  active thread the cache knows as `type: "thread"` rows; the sidebar hides them
  (`Api.visibleChannels`) and reduces them to per-parent counts; `t` fetches
  `list_threads` into `threadsByParent[parentId]`. `channel_update` never triggers a
  request directly: guilds and loaded thread parents are marked dirty and a 300 ms
  timer issues one `list_channels` / `list_threads` each — THREAD_LIST_SYNC arrives
  as one `create` per thread (749 on one real guild) and the backend drops a client
  that cannot keep up with the response stream.
- **Read state**: `read_state_changed` patches the channel row (and its guild, and
  any mirrored thread row); the bar badge is `total_mention_count` from the state,
  never a QML reduction.
- **Messages**: `open_channel` registers the channel in `openChannels` and stores
  `channelData[id] = {channel, messages, hasMore, loading, oldestId, unreadMarkerId}`.
  Events merge by id (`message_create` may precede the open response); history
  prepends; a rolling window caps the loaded array at 500 + 100 slack while the
  timeline is pinned to the bottom. Optimistic rows (`pending-N`) are re-keyed by
  the send response's nonce. One channel is open at a time; the previous is closed.
- **Member list**: `membersWanted` (the `m` toggle) + the panel being registered
  visible + `currentChannelId` decide the one subscribed channel (`syncMembers`);
  `member_list_update` replaces `memberList`, `presence_update` patches rows. A 15 s
  timer flips `membersTimedOut` when no list arrives.
- **Server emoji**: `list_emoji` on `ready` and on every `guilds_synced` →
  `serverEmoji` (guild-grouped); the picker only builds its grid while shown.
- **Settings**: self-served from `shell.shellConfig` (the plugin's inline entry in
  `bar.layout.*`), normalized, re-read on every `shellConfigChanged`; writes go
  through `shell.updateEntryInline` with a merge so unknown keys survive. Small
  opaque state (`frequentEmoji`) rides on the same entry as a JSON string.
- **Media**: see "Media pipeline".

## Lifecycle

1. The shell loads `Service.qml` synchronously while the plugin is enabled
   (services have no on-demand mode). Startup work is deferred behind a 0 ms timer
   so `shell` / `manifest` injection lands first.
2. `DaemonManager.checkRequirements()` runs `scripts/backend-runtime.sh check`; a
   missing runtime triggers `setup.sh`, which installs the bundled binary and unit
   (`$XDG_CACHE_HOME/omarchy-discord/target` for any build output — never the tree,
   which is watched by the shell's hot reload).
3. `keepAliveTimer` starts the unit whenever the socket is down and either
   `stayConnected` is On or a UI surface is visible; `idleTimer` stops it 15 min after
   the last surface closed when `stayConnected` is Off.
4. `BackendClient` connects, sends `hello`, receives the snapshot; on `ready` the
   service refreshes the structure, loads server emoji, re-opens the channels it
   had open (open sets and member subscriptions are per socket connection) and
   re-issues in-flight media requests.
5. The panel is summoned by the shell (`shell.summon(id, payloadJson)` → `open()`),
   registers `setUiVisible("full-panel", true)`, restores the cursors from the
   service and takes keyboard focus after map. `close()` / destruction unregisters,
   which also drops the member subscription.
6. Any write inside the plugin directory (or disabling it) destroys and recreates
   the service; the backend unit and its session survive, and the reconnecting
   client picks the snapshot back up.

## Keyboard and zone model

Four zones — sidebar (rail / channels columns), timeline, composer, and members
while the pane is shown — each with an id-keyed roving cursor so a resync keeps the
row. A full-window `FocusScope` with `Keys.priority: BeforeItem` routes keys:
global chords first (`Ctrl+K`, `Ctrl+/` everywhere; `/`, `?`, `m`, `r`, `t` outside
text inputs), then the zone's own handler. The Timeline, Composer and MemberList
handle their keys on their own focus item and let the rest bubble; the composer
intercepts on the TextArea itself (`Ctrl+K` would otherwise delete to end of line,
`Alt+m` would type an "m") and emits `switcherRequested` / `cheatsheetRequested` /
`membersRequested`. Modal overlays (cheatsheet, picker) own the keyboard entirely
while shown and hand focus back to the last zone on close.

`Keymap.js` is the one key table: `FOOTER` states are lists of entry ids the panel
footer renders for the current zone/state, `sections()` groups the same entries by
zone for the cheatsheet, and `missingFooterIds()` must stay empty (harness
assertion). The README's keyboard tables are generated from it.

Esc ladder: dismiss overlay → composer chip cursor → edit/reply mode → composer →
timeline (marking read) → channel list → rail → close the panel. Tab cycles rail →
channels → timeline → composer (→ its chips) → member list → Members → Log out →
Close; stops that cannot take focus are skipped.

## Media pipeline

QML never touches the network. `Service.mediaPath(url, size)` returns a cached local
path or `""`; a miss only records the want and defers one `fetch_media` per
`url|size` key to the next event-loop turn (`Qt.callLater`), so no socket write
happens inside a binding (that produced binding-loop warnings on every media row).
The backend downloads from Discord CDN hosts only (`cdn.discordapp.com`,
`media.discordapp.net`; `Api.isCdnUrl` filters before any request) into
`$XDG_CACHE_HOME/omarchy-discord/media/`, an LRU capped by `mediaCacheMB`
(`set_config` on every connect and on change), and answers with a cache hit or a
later `media_ready`. Sizes: avatars and guild icons 64, emoji 32 (PNG even for
animated — rich text cannot animate), attachments and embeds original. A path the
LRU evicted fails in the `Image`; `mediaError(path)` drops the key and re-requests
once per connection. Spoiler images are never fetched until revealed.

Uploads go the other way: `wl-paste` writes the clipboard image into
`$XDG_RUNTIME_DIR/omarchy-discord/staged/` (0700), the chip shows it, `upload`
streams it with `upload_progress` events routed by request id and filename, and
the staged file is removed once sent or when the chip is removed.

## Notification rules

`message_create` events carry `notify: true` when ningen says the message would
notify under Discord's own settings (mute, suppression; role mentions are not
implemented upstream). `Service.maybeNotify` then applies, in order: plugin mode Off
→ skip; own message → skip; Discord status DND → skip; mode "Mentions and DMs" and
neither `mentions_self` nor a DM → skip; the channel is being viewed (panel open,
window active, same channel) → skip. Delivery is `notify-send --app-name="Omarchy
Discord" --urgency=normal [--icon=<cached avatar>] -- <summary> <body>` through
`Quickshell.execDetached`; the shell's notification server renders it themed and
applies do-not-disturb. Summary "Author in #channel" ("Author" for DMs), body the
plain-text first 200 characters, entity-escaped, plus a paperclip for attachments.
One notification per channel per 3 s; arrivals inside the window are held and
flushed as one "(+N more)" when the window ends, and the timer is never re-armed by
later arrivals so continuous traffic cannot postpone the flush.

## Security model

- The user token never touches disk, argv, logs, the journal or the QML layer. It
  enters through `login` over the socket (the QML clears its copy immediately) or
  the QR flow inside the backend, goes to GNOME Keyring via `secret-tool` over
  stdin (`scripts/keyring-store.sh`), and is held in backend memory only.
- Every error string crossing a boundary passes a redaction function (`Api.redact`
  in QML, `Redact` in Go: bearer headers, `token=` query params, `"token"` /
  `"ticket"` / `"encrypted_token"` JSON fields).
- REST is restricted to `https://discord.com/api/v9` plus the remote-auth gateway;
  media to the two CDN hosts. Guild member lists are never fetched over REST (an
  instant email-unverification for user accounts); they come from the gateway
  exactly as the official client requests them.
- The socket is 0600 in an owner-only directory; there is no auth inside the
  protocol. The systemd unit carries `NoNewPrivileges`, `PrivateTmp`,
  `ProtectSystem=strict`, `UMask=0077`.
- Opening a 1:1 DM with no history is refused (`empty_dm_refused`) — a known
  spam-flag trigger for third-party clients.

## Development workflow

### Harnesses

All UI verification is offscreen (`QT_QPA_PLATFORM=offscreen`) against the installed
shell's `qs.Commons` / `qs.Ui`, driven by QtTest key events and `Panel.dispatchKey`;
nothing injects keys into the live Wayland session.

- `components/harness/run.sh` — the Timeline harness: a scratch config root of
  symlinks (nothing is written into the plugin tree), `qs -p` it, drive it over IPC
  (`qs -p "$ROOT" ipc call harness key j`).
- `node components/harness/markdown.test.js` — Markdown.js unit tests (escaping,
  mentions, spoilers, never-throws on garbage).
- The full-panel harnesses used for each phase live outside the tree (a Python mock
  backend speaking protocol v1 with a control socket, a `PanelWindow` shim for the
  switcher, one `shell.qml` per scenario that checks state and grabs screenshots).
  The Phase 3 set covers threads, forums, the channel_update burst, the member pane
  (subscribe / unsubscribe / presence / zone navigation / 15 s fallback), server
  emoji, the theme pass (every view rendered with each bundled light theme's tokens
  by calling `Color.loadColors` / `Color.loadShell` with a shell.toml generated from
  `default/themed/shell.toml.tpl`), and a read-only run against the real backend
  through a proxy that refuses every write command.

### Quality gate

Before a commit touching the frontend:

1. `qmllint` (Qt 6: `/usr/lib/qt6/bin/qmllint -I <dir with qs -> /usr/share/omarchy/shell> *.qml components/*.qml`)
   — no new warnings beyond the known false positives (`Style.*` / `Color.*`
   members "not found on type QObject": the shell's singletons are untyped to the
   linter).
2. `omarchy plugin validate .` (exit 0, silent).
3. `node components/harness/markdown.test.js`.
4. The offscreen harnesses above, with screenshots inspected.

Before a commit touching `backend/`: `gofmt -l`, `go vet ./...`, `go test ./...`,
`go build ./...` with `GOCACHE` and `-o` outside the tree (golden fixtures for every
wire shape; recorded gateway payloads for handlers).

### Install, rebuild, remove

```sh
scripts/install-local.sh            # validate → setup.sh → copy into plugins dir → rescan → enable
scripts/setup.sh --reinstall-backend
scripts/remove-runtime.sh [--purge]
```

`scripts/backend-runtime.sh check|unit|status|start|stop` is the only place QML
touches systemctl. `omarchy-discord-backend check` prints the environment summary;
`omarchy-discord-backend login` reads a token from stdin.

### Theme QA

Run the view set with a theme's tokens (scratch harness), then inspect: invisible
text, muted-on-muted, tints that vanish on light backgrounds, hardcoded colours,
borders that disappear. Findings so far: `Color.muted` is unusable as text on most
themes (1.5–2.5 contrast), hence `Api.secondaryColor`; the spoiler cover must be
opaque (`Api.blend`); everything else follows the tokens. Keep `grep -n '"#[0-9a-fA-F]'`
over the QML empty.

## Upstream projects

arikawa and ningen (diamondburned) provide the protocol layer; dissent is the
reference implementation for session and member-list semantics; discordo's QR
remote-auth flow is ported into `internal/remoteauth` against upstream arikawa's
`ExchangeRemoteAuthTicket`; thisisgm/omarchy-discord supplied the bar-mark
geometry. Pin arikawa and ningen as a mutually consistent pair (the dissent pin is
the proven combination).
