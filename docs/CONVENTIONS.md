# Engineering Conventions — omarchy-discord

Contract for all implementation work on this plugin. Every API name below was verified
against the installed Omarchy Quattro shell source (`/usr/share/omarchy/shell/`), the
quickshell.spotify v1.0.2 plugin, arikawa v3 (b430932b), ningen v3 (5a08d3a7), discordo
(f8803785), and dissent (6ff6182). Where the PLAN assumed something that does not exist,
see **DIVERGENCES FROM PLAN** at the end. PLAN.md remains the authority on scope;
this document is the authority on mechanics.

---

## 1. Plugin layout & manifest

Plugin id: `quickshell.discord` (dotted third-party ids are valid; `omarchy.*` is a
reserved namespace). Installed checkout lives at
`~/.config/omarchy/plugins/quickshell.discord/`.

```
omarchy-discord/
├── manifest.json
├── Service.qml                     # socket client, state store, notifications
├── BarWidget.qml                   # per-monitor badge, popup, IpcHandler
├── Panel.qml                       # full client (FloatingWindow), lazy-loaded
├── QuickSwitch.qml                 # Ctrl+K overlay
├── components/                     # MessageRow, Timeline, Composer, ChannelList, GuildRail, Markdown.js
├── backend/                        # Go module (see §5)
│   └── dist/x86_64/omarchy-discord-backend   # committed prebuilt (real file, never a symlink)
├── systemd/omarchy-discord.service # static user unit (no [Install] section)
├── scripts/                        # build-backend.sh, setup.sh, install-local.sh,
│                                   # backend-runtime.sh, keyring-store.sh, remove-runtime.sh
└── docs/                           # PLAN.md, CONVENTIONS.md, BACKEND_PROTOCOL.md, TECHNICAL.md
```

### Manifest — fields the shell actually reads

Validation is `PluginRegistry.qml validateManifest` plus `omarchy plugin validate`:

| Field | Rule |
|---|---|
| `schemaVersion` | **JSON number `1`** — the string `"1"` is rejected |
| `id`, `name`, `version` | required |
| `kinds` | non-empty array; ours: `["service", "bar-widget", "panel"]` |
| `entryPoints` | object; kind→key mapping is fixed: `service`→`service`, `bar-widget`→`barWidget`, `panel`→`panel`. Relative paths only, no leading `/`, no `..`. A declared kind without its entry point fails validation |
| `barWidget.defaultSection` | one of `left`/`center`/`right` |
| `barWidget.{displayName, description, category, allowMultiple, defaults, schema}` | copied into BarWidgetRegistry metadata |
| `keepLoaded` (top-level, optional) | keeps the panel mounted between summons. Spotify omits it; we omit it too — authoritative state lives in Service.qml, the panel is cheap to recreate |
| `author`, `license` | informational |

**Schema entries are objects**, not strings. Booleans are modeled as `enum`
`"On"`/`"Off"`. Shapes verified in shipped manifests:

```json
{ "key": "notifications", "type": "enum", "label": "Notifications",
  "options": ["All", "Mentions and DMs", "Off"], "defaultValue": "Mentions and DMs",
  "description": "…" }
{ "key": "mediaCacheMB", "type": "integer", "label": "Media cache size (MB)",
  "min": 64, "max": 4096, "step": 64, "defaultValue": 512, "description": "…" }
```

A parallel `barWidget.defaults` map duplicates each schema `defaultValue` (and may carry
non-schema keys). Do **not** include `activation` (no consumer in shell source) or
`barWidget.aliases` (read by nothing).

### Enablement & settings home

Enabling inserts ONE entry `{"id": "quickshell.discord", ...settings...}` into
`shell.json` `bar.layout.<section>[]`; that single inline entry enables all three kinds
and is the plugin's only shell-provided persistence. Anything bigger goes in
`$XDG_CACHE_HOME/omarchy-discord/` or `$XDG_STATE_HOME` — never the plugin tree.

### Hot reload — hard rule

The shell runs `inotifywait -m -r -e close_write,create,delete,move` over
`~/.config/omarchy/plugins`; **any write inside the plugin dir reloads the whole plugin
system** (150 ms debounce; dotfiles and `.git/` exempt). Therefore:

- Go builds go to `$XDG_CACHE_HOME/omarchy-discord/target` — never the tree.
- The installed runtime binary lives at `~/.local/lib/omarchy-discord/` — never the tree.
- No logs, sockets, caches, or staged files in the tree at runtime.
- `omarchy plugin validate` refuses symlinks anywhere in the plugin folder — the shipped
  `dist/` prebuilt must be a real file.

### Install flow

`omarchy plugin add <repo> --enable`. The installer runs no hooks; the enabled Service
installs the bundled backend binary + unit on first load (spotify
`installBundledBackendIfNeeded` pattern). `scripts/install-local.sh` for dev: validate →
`setup.sh` → symlink checkout into plugins dir (refusing to replace any existing path) →
`omarchy-shell shell rescanPlugins` → poll `omarchy plugin list --json` → enable.

---

## 2. QML conventions

### Imports allowed

`QtQuick`, `QtQuick.Controls`, `QtQuick.Layouts`, `QtQuick.Effects`, `Quickshell`,
`Quickshell.Io`, `Quickshell.Wayland`, `qs.Commons`, `qs.Ui`, and plugin-local
`import "Foo.js" as Foo`. The `qs.` prefix resolves from third-party plugin dirs
(spotify proves it). **Never import shell services as singletons** — `PluginRegistry`
and `BarWidgetRegistry` are instances handed in by property injection.

### Property injection (conditional — declare or it silently stays null)

The host sets a property only if the root object declares it (`"shell" in item`):

- Service root: `property var shell; property var manifest; property var pluginRegistry;`
  (also available: `omarchyPath`, `barWidgetRegistry`). Root is
  `Item { visible: false; width: 0; height: 0; ... }`.
- Panel root: same five **plus** `property var service` (the live Service instance).
- Bar widget: injected `bar`, `moduleName`, `settings` — extend `qs.Ui BarWidget`
  which declares them.

Plugin dir at runtime: `manifest.__sourceDir`. Plugin id:
`manifest && manifest.id ? String(manifest.id) : "quickshell.discord"`.

### Theme tokens (exact names — no raw colors anywhere)

From `qs.Commons` singletons:

- `Color.foreground / background / accent / urgent / muted` — `Color.urgent` is the
  mention-badge color.
- Per-surface: `Color.popups.{background,text,border}` (panel/popup surfaces),
  `Color.bar.{background,text,active}`, `Color.tooltip.*`, `Color.menu.*`.
- Inside bar widgets prefer `bar.foreground` / `bar.barForeground` (wallpaper-derived)
  over `Color.foreground`; panels use `Color.foreground` directly.
- `Style.font.family` — fontconfig `monospace` alias; **this IS the mono font**, there
  is no separate mono token. Sizes: `Style.font.{caption,bodySmall,body,subtitle,title,heading,display,...}`.
- `Style.space(px)` for every dimension (spacing-scale aware); semantic tokens under
  `Style.spacing.*`; `Style.cornerRadius`, `Style.gapsOut`, `Style.bar.*`.
- Interactive states: `Style.controlFill(focused, hot, fg, accent)`,
  `Style.controlBorder(...)`, `Style.controlBorderWidth(focused, hot)`; prebound
  `Style.normalFill/hoverFill/selectedFill/...`; per-role `Style.normalFillFor(fg, accent, urgent)` etc.
- Borders: `Border.flat(color, width)`, `Border.surfaceSpec("popups", "border",
  Color.popups.border, 2)`, `Border.controlSpec(state, fg, accent, urgent)` with
  `qs.Ui BorderSurface`/`BorderOverlay`.
- `Util.alpha(color, a)` for alpha composition.

Light themes flow through the same tokens automatically — never branch on light/dark.

### Settings access

- **Bar widget** reads injected `settings` via `setting(name, fallback)`
  (from `qs.Ui BarWidget`); re-injected live on shell.json change.
- **Service is NOT injected with settings** — it self-serves by scanning
  `shell.shellConfig.bar.layout.{left,center,right}[]` then `plugins[]` for its entry
  (spotify `configuredEntry()`), re-synced via
  `Connections { target: root.shell; function onShellConfigChanged() {...} }`.
  Defaults merge from `manifest.barWidget.defaults`. The widget additionally pushes its
  injected settings into the service on change (`onSettingsChanged`).
- **Write**: normalize (enums coerced to canonical option strings, integers clamped to
  schema min/max), merge over a copy of current settings, then
  `shell.updateEntryInline(pluginId, merged)` — it replaces the whole entry, so unknown
  keys are dropped unless carried forward. Small opaque state (session state, switcher
  history) may persist as JSON-string keys on the same entry (spotify caps at 16000 chars).

### Notifications

**There is no QML notify API.** Fire freedesktop notifications: `notify-send` via a
`Process`/`Quickshell.execDetached`, or D-Bus `org.freedesktop.Notifications` from the
Go backend. The shell's `omarchy.notifications` server renders them themed and applies
DND (our app name will be correctly silenced by DND — bypass exists only for
`omarchy-action` / critical+`notify-send`). The plugin-level *All / Mentions and DMs /
Off* filter gates in Service.qml **before** sending. Never put message content into argv
logs; icon = cached avatar path from the media cache.

### Bar widget contract

- Root extends `qs.Ui BarWidget { moduleName: "quickshell.discord" }`; must set
  `implicitWidth`/`implicitHeight` (zero collapses the slot).
- **Instantiated once per monitor.** The socket client lives in Service.qml only;
  widgets mirror via
  `readonly property var discord: bar && bar.shell ? bar.shell.serviceFor("quickshell.discord") : null`
  and null-guard every handler. Use `broadcast(method)` when an IPC-triggered action
  must reach every instance.
- Visual: `qs.Ui WidgetButton` (props `bar, text, foreground, activeColor, active,
  dimmed, tooltipText, ...`; signals `pressed(int button)`, `wheelMoved(int delta)`).
  Vector/glyph mark in `bar.foreground`, dimmed when disconnected, mention count in
  `Color.urgent`.
- Click split (spotify precedent): left = open panel, middle = configured quick action.
  Opening the full panel from the widget: `bar.shell.hide(pluginId)` then
  `Qt.callLater(() => bar.shell.summon(pluginId, JSON.stringify(payload)))` — the
  hide/summon split across event-loop turns is required on Wayland.
- IpcHandler for Hyprland binds goes in **BarWidget.qml or Service.qml** (always
  loaded), never in Panel.qml (doesn't exist until first summon):
  `IpcHandler { target: root.moduleName + ".panel"; function toggle(): void {...} }`
  → `omarchy-shell quickshell.discord.panel toggle`. The zero-code fallback bind is
  `omarchy-shell shell toggle quickshell.discord`.

### Panel contract

- Root is a plain `Item` declaring `shell`, `manifest`, `service`,
  `property bool opened: false`, plus:
  - `function open(payloadJson)` — **payload is a JSON string; `JSON.parse` it.**
  - `function close()` — called by the host on hide.
  Without all three, `isPluginOpen`/toggle break.
- The surface is a Quickshell `FloatingWindow` (normal Hyprland-managed window with
  `title`, `minimumSize`, `visible: root.opened`); a `closingFromHost` flag
  distinguishes host-driven close from the user closing the window, which must route
  back through `shell.hide(pluginId)`.
- `open()` ends with `Qt.callLater(() => focusScope.forceActiveFocus())` — keyboard
  focus after map. Bar popups / the quick switcher instead use `qs.Ui KeyboardPanel`
  (layer-shell `WlrLayer.Overlay`, primes `WlrKeyboardFocus.Exclusive` ~75 ms then
  OnDemand — the only way a keyboard-summoned surface gets keys without a click), with
  `focusTarget` set to an inner key-handling Item (`qs.Ui PanelKeyCatcher` fits).
- Without `keepLoaded` the panel item is **destroyed on hide** — all authoritative
  state (open channel, timeline cache mirror, composer drafts) lives in Service.qml.
- Every UI surface calls `service.setUiVisible(key, bool)` on open/close/destruction
  with a unique key; `uiVisible` (refcount over surfaces) gates polling/refresh work.

### Service lifecycle

Services load synchronously at shell startup while the plugin is enabled — **there is
no on-demand mode**; idle-disconnect is a backend/service feature, not a manifest one.
Any write in the plugin dir (or disable) destroys and recreates the Service instance,
so the backend daemon must survive frontend restarts (systemd unit + reconnecting
socket client). Defer startup work behind a 0-interval Timer from
`Component.onCompleted` so shell/manifest injection lands first. Surface errors as
`property string lastError` / `statusMessage` + `signal operationFailed(string)`, all
strings passed through the redact helper (§7).

### Socket client (copy spotify BackendClient.qml wholesale)

- Path: `(Quickshell.env("XDG_RUNTIME_DIR") || "/tmp") + "/omarchy-discord/backend.sock"`.
- Quickshell `Socket` **cannot reconnect in place** after a failed connect — wrap in
  `Component` + `Loader` and toggle `active` each retry. Backoff:
  `Math.min(1500, 180 + attempt * 120)`, cap attempt at 12, reset on connect.
- `SplitParser { splitMarker: "\n" }`; on connect send `hello`.
- Correlation: `nextId` int counter, `pending` map id→callback — **replace the map
  wholesale (copy → assign) for QML reactivity**, never mutate in place.
- `socket.write(JSON.stringify(payload) + "\n"); socket.flush()`.
- Tolerate responses whose id matches no pending request (malformed requests are
  answered with id 0).
- Single on/off input `property bool wanted`; when false: stop timer, deactivate
  loader, fail all pending callbacks. All error strings redacted before callbacks.

### Misc QML rules

- List rendering: `ListView { clip: true; reuseItems: true; cacheBuffer:
  Style.space(150); boundsBehavior: Flickable.StopAtBounds }`, heterogeneous rows via a
  `Loader` delegate switching on row kind, model = plain JS array + `model: rows.length`.
- Heavy/pure logic in a plugin-local `.js` library (spotify `Api.js` pattern), out of
  bindings.
- Render-thread animations (`XAnimator`) for continuous motion; no JS timers driving
  animation on the shared shell main thread.
- `renderType: Text.NativeRendering` for bar text; Nerd-Font glyphs as icon text.
- Process command arrays that depend on `pluginDir` are assigned at call time inside
  functions, not bound declaratively (`pluginDir` arrives after children construct;
  a declarative binding briefly runs `/scripts/...` → exit 127).

---

## 3. Keyboard & focus conventions

Three focus zones — **sidebar**, **timeline**, **composer** — each with a roving
cursor. The implementation patterns to copy are spotify's, verbatim:

- **Roving cursor** (spotify BarWidget.qml mini-player): a
  `readonly property var <zone>Actions` computes the ordered list of currently
  available item ids from state; `property string <zone>Cursor` +
  `bool <zone>CursorActive`; `ensureCursor/setCursor/moveCursor(delta)` with
  wrap-around modulo; one `handleKey(event)` function attached via `Keys.onPressed`
  with `Keys.priority: Keys.BeforeItem`; `on<Zone>ActionsChanged: ensureCursor()`
  keeps the cursor valid as availability changes. Cursor visual state uses the
  hover-cursor state tokens (`Style.hoverFillFor` / `Border.controlSpec("hover-cursor", ...)`).
- **Shortcuts** (spotify Panel.qml): `Shortcut { sequence: ...; enabled:
  !root.shortcutsBlocked && ...; }` children of a full-window
  `FocusScope { focus: true }`. `shortcutsBlocked` ORs all modal popup `.opened`s;
  single-key shortcuts additionally require `!textInputFocused()`. Modal popups take
  `focus: true` and return it with
  `onClosed: Qt.callLater(() => focusScope.forceActiveFocus())`.
- **Esc ladder**: one `Keys.onEscapePressed` on the FocusScope walks, in order:
  dismiss transient popup → clear search text → collapse search → leave zone
  (composer → timeline, marking read) → two-stage close (first Esc arms — close
  affordance turns `Color.urgent` — second Esc within the timeout closes; any
  navigation disarms).
- **Focus on open** is an acceptance criterion: `open()` →
  `Qt.callLater(forceActiveFocus)` for the FloatingWindow panel; KeyboardPanel's
  Exclusive→OnDemand prime for the quick switcher and bar popup. Opening a channel
  focuses the composer.
- Keymap per PLAN.md (Ctrl+K quick switcher, Alt+h/l zone moves, j/k roving, `R E Y O`
  message actions, `↑` edit-last in empty composer, Ctrl+/ cheatsheet).

---

## 4. Backend conventions (lifecycle, packaging, keyring)

### Build & install locations

| What | Where | Never |
|---|---|---|
| Build output | `$XDG_CACHE_HOME/omarchy-discord/target` | the plugin tree (hot reload kills builds) |
| Installed binary | `~/.local/lib/omarchy-discord/omarchy-discord-backend` (dir 700, bin 755; override `OMARCHY_DISCORD_RUNTIME_DIR`) | the plugin tree |
| Shipped prebuilt | `backend/dist/x86_64/omarchy-discord-backend` (gitignore-negated, real file, selected by `uname -m`) | a symlink |
| Socket | `$XDG_RUNTIME_DIR/omarchy-discord/backend.sock` (0600; parent dir created; stale file unlinked on bind; removed on shutdown) | — |
| Media cache | `$XDG_CACHE_HOME/omarchy-discord/media/` (LRU capped by `mediaCacheMB`) | — |
| Staged uploads | `$XDG_RUNTIME_DIR/omarchy-discord/staged/` | — |

`build-backend.sh`: prebuilt-wins (if `dist/$(uname -m)/` binary is executable, just
`install` it); else require the Go toolchain and build with output outside the tree,
then `install`. Backend gitignore mirrors spotify's dist-negation block.

### systemd static user unit

`systemd/omarchy-discord.service` — **no `[Install]` section** (that is what makes it
static / never enabled at login; do not add one). Shape:

```ini
[Unit]
Description=Omarchy Discord plugin backend
After=graphical-session.target

[Service]
Type=simple
ExecStart=%h/.local/lib/omarchy-discord/omarchy-discord-backend
Restart=on-failure
RestartSec=5
TimeoutStopSec=5
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
```

Installed by `setup.sh` (`install -m 644` into `~/.config/systemd/user/`, then
`systemctl --user daemon-reload`; warn — don't fix — if the user manually enabled it).
Started/stopped on demand by QML through one shim script,
`scripts/backend-runtime.sh check|unit|status|start|stop` — the **only** place QML
touches systemctl; `stop` stops unconditionally with `|| true`. The daemon is never a
child of the shell.

Unlike spotify, the default is connected-while-enabled: the Service starts the unit on
load and does not idle it out unless the `stayConnected` setting says so (idle
disconnect is implemented in Service.qml exactly like spotify's `idleShutdownMinutes`
timer, gated on `uiVisible` and `lastActivityAt`).

Config file (if one is needed): `~/.config/omarchy-discord/` mode 600, dir 700,
preserved across setup reruns unless `--force-config` (timestamped `.bak` first).

### Keyring — secret-tool over stdin, verbatim pattern

The token travels over **stdin, never argv** (argv is visible in `ps`), never a file.
`scripts/keyring-store.sh` (adapted from spotify's, whole-file pattern):

```sh
#!/bin/sh
set -eu
IFS= read -r user_token
[ -z "$user_token" ] && exit 3
printf '%s' "$user_token" | secret-tool store \
  --label='Omarchy Discord user token' \
  service quickshell-discord \
  kind user-token
```

- Lookup: `["secret-tool", "lookup", "service", "quickshell-discord", "kind", "user-token"]`.
- Clear: `secret-tool clear service quickshell-discord kind user-token` — removes
  **one** entry per invocation; purge scripts loop (cap 20 iterations).
- QML write side: `Process { stdinEnabled: true; onStarted: { write(token + "\n");
  token = "" } }` — clear the property immediately after write.
- Processes that may see the token consume stdout/stderr with discarding `SplitParser`
  handlers — never `StdioCollector` — so secrets can't land in the journal.
- The Go backend reads the token from the keyring itself (libsecret/secret-tool child
  with the same attributes) or receives it via the socket `login` command; either way
  it holds it only in memory.

### Teardown

`scripts/remove-runtime.sh [--purge]`: stop unit, remove unit file + installed binary,
`daemon-reload`; config dir moved to `.bak.<timestamp>` (deleted only with `--purge`
after re-asserting the literal path); `--purge` also loops keyring clear and removes
the cache dir.

---

## 5. Go conventions

- Module in `backend/`; layout `backend/cmd/omarchy-discord-backend/main.go` +
  `backend/internal/{socket,protocol,session,readstate,media}/`.
- Pin `github.com/diamondburned/arikawa/v3` and `github.com/diamondburned/ningen/v3`
  to a **mutually consistent pair** (ningen's capability constants must exist in the
  pinned arikawa) — the dissent pin (arikawa v3.6.0 + 2025-07 ningen) is the proven
  combination; upgrading either means upgrading both. Never depend on the `ayn2op`
  forks — everything we need was verified upstream except QR (which we port as our own
  code, §6).
- Quality gate, run before every commit touching `backend/`:
  `gofmt -l` (empty output) · `go vet ./...` · `go test ./...` · `go build ./...`
  — all with the build/test cache outside the plugin tree when the checkout is the
  live plugin dir (`GOCACHE=$XDG_CACHE_HOME/omarchy-discord/gocache`, `-o` into
  `$XDG_CACHE_HOME/omarchy-discord/target`).
- **Golden tests for the protocol**: every request/response/event shape in
  BACKEND_PROTOCOL.md has a golden JSON fixture round-tripped through the real encoder
  and decoder; changing wire output requires regenerating goldens in the same commit,
  which makes accidental breakage reviewable.
- **Recorded-payload tests for the gateway**: handlers are tested against captured raw
  gateway JSON (READY with `read_state.entries` object form, `private_channels` with
  `recipient_ids` only, MESSAGE_CREATE with nonce, GUILD_MEMBER_LIST_UPDATE ops, …) —
  scrubbed of real ids/tokens before committing. arikawa's
  `ws.EnableRawEvents` dump mechanism (dissent's `DISSENT_DEBUG_DUMP_ALL_EVENTS_PLEASE`
  pattern) is the capture tool.
- One-shot subcommands on the binary, spotify-style: `check` (validate environment,
  print JSON summary, exit) is worth shipping from Phase 0.
- All socket writes go through a single writer goroutine; every handler (gateway
  events, `read.UpdateEvent`, media results) sends into it via channels — no direct
  writes from event goroutines (see §6 ordering gotchas).
- CLI flags: `--socket-path` override (default computed from `XDG_RUNTIME_DIR`).
- Distinct exit codes for distinct setup failures (spotify convention), documented in
  the scripts that interpret them.
- Snowflakes are `discord.Snowflake` internally, **strings on the wire** (JSON
  round-trips through QML would lose 64-bit precision).

---

## 6. ningen / arikawa usage conventions

### Session boot

- Construct with `ningen.New(token)` or `ningen.NewWithIdentifier(id)` — **there is no
  `ningen.Connect` or `ningen.FromToken`**. `ningen.FromState(state.New(token))` is
  the dissent path (skips capabilities; ningen handles both READY shapes).
- Before constructing anything, override identity globally (dissent pattern):
  `api.UserAgent = "..."` and `gateway.DefaultIdentity = gateway.IdentifyProperties{
  gateway.IdentifyOS: runtime.GOOS, gateway.IdentifyBrowser: ..., gateway.IdentifyDevice: "Arikawa"}`.
  Phase 0 ships dissent-parity fingerprinting; discordo's full browser spoof
  (X-Super-Properties, TLS profile) is a later opt-in, never the fork.
- **User accounts: `IdentifyCommand.Intents` stays nil.** Capabilities replace intents.
- `n.Open(ctx)` blocks until READY is processed and all ningen sub-states are
  populated. **`n.Connect(ctx)` statically dispatches to `session.Connect` →
  `session.Open`, bypassing ningen's ready-wait** — either drive your own retry loop
  around `n.Open` + `DisconnectedEvent`, or use `Connect` and gate readiness on
  `ningen.ConnectedEvent`, never on `Connect` returning.
- Reconnect/resume is arikawa's job. Our job: `ningen.ConnectedEvent` (fires on Ready
  AND Resumed) → lifecycle `ready`; `ningen.DisconnectedEvent` → if `IsLoggedOut()`
  (close codes 4004/4010-4014) → `reauth_needed`, else brief `connecting` with a ~3 s
  grace before surfacing it (dissent's anti-flash rule).
- `gateway.ReadyEventKeepRaw` is forced true by ningen's `init()` — required by
  `hackReady` and read-state parsing; never disable it. Expect a connect-time memory
  spike (one retained raw READY).
- **Register all our handlers on ningen's handler** (`n.AddHandler(...)` resolves to
  it) — it re-dispatches after sub-states update, so caches are consistent inside
  handlers. Registering on `state.State`'s handler races the caches.
- `Offline()` gives cache-only reads (pre-cancelled context — REST fails instantly);
  use `Online()` deliberately for REST. Forgetting `Online()` on a copied state
  silently breaks all REST.

### Read state & acks

- Per-channel: `n.ReadState.ReadState(chID)` → `gateway.ReadState{LastMessageID,
  MentionCount, ...}`; derived: `n.ChannelIsUnread(chID, opts)` (Read/Unread/Mentioned),
  `n.GuildIsUnread(...)`, `n.ChannelCountUnreads(...)`.
- Bar badge number: `n.ReadState.TotalMentionCount()`.
- The `read_state_changed` socket event is driven 1:1 by `read.UpdateEvent`
  (`{gateway.ReadState; GuildID; Unread bool}`) — **it fires on fresh goroutines with
  no ordering guarantee**; funnel into the socket writer goroutine.
- Ack: `n.ReadState.MarkRead(chID, msgID)` — it dedupes, zeroes MentionCount, and
  sends the REST ack (`POST /channels/{ch}/messages/{msg}/ack`, empty `&api.Ack{}`)
  only when the message is cached and not self-authored — so ack after the channel is
  open and the message cached. Ack on focus-at-bottom (dissent), not on a timer.
- Mute/suppression is folded into `n.MessageMentions(msg) MessageMentionFlags`
  (`MessageMentions` | `MessageNotifies`) — the notification decision function.
  Notify on `Has(MessageNotifies)` (stricter than dissent, which arguably has a bug
  notifying on Mentions alone), skip when `n.Status() == discord.DoNotDisturbStatus`,
  and suppress the currently-open channel. Known limit: ningen does **not** implement
  role mentions (`@role` won't notify; explicit TODO in ningen).

### Structure & subscriptions

- Guild/channel lists: `n.Channels(guildID, allowedTypes)` (permission-filtered,
  empty categories removed) and `n.PrivateChannels()` (sorted by LastMessageID desc) —
  use these, not raw Cabinet reads. Allowed-types list: copy dissent's
  `AllowedChannelTypes`.
- `open_channel` on a guild channel calls `n.MemberState.Subscribe(guildID)`
  (idempotent; sends `GuildSubscribeCommand{Typing: true, Threads: true,
  Activities: true}`, Op 14). **Guild TypingStart is silent without it.** DMs need
  nothing.
- Member list: `n.MemberState.RequestMemberList(guildID, chID, chunk)` +
  `GuildMemberListUpdateEvent` ops; single misses via
  `n.MemberState.RequestMember(guildID, uID)` (batched Op 8); search via
  `SearchMember`. **Never REST-fetch guild member lists as a user account — instant
  email unverification** (warning in ningen source). Don't rely on
  `GuildMembersChunk` arriving (it often doesn't for user accounts).
- Presence: `n.SetStatus(status, custom, activities...)` (gateway + settings PATCH so
  it persists); readback `n.Status()`; others via `n.PresenceStore.Presence(guildID,
  uID)` (guild 0 = global/friends).

### Messages

- Initial ~50: `state.Messages(chID, limit)` (cache-aware; default store caps at
  100/channel). Paging past cache: `client.MessagesBefore(chID, beforeID, limit)`
  directly — don't pollute the cache with deep history.
- Send: `SendMessageComplex(chID, api.SendMessageData{Content, Nonce, Reference:
  &discord.MessageReference{MessageID: replyTo}, AllowedMentions, Files})`. **Always
  set a Nonce** — the gateway echoes it on `MessageCreateEvent`, which is the only
  dedup for optimistic sends; skipping it renders duplicates.
- Edit: `EditText(chID, msgID, content)`. Delete: `DeleteMessage(chID, msgID, "")` —
  remove rows on the gateway `MessageDeleteEvent`, not locally.
- Reactions: `React/Unreact(chID, msgID, discord.APIEmoji)`; **strip trailing U+FE0F
  variation selectors first** (dissent `SanitizeEmoji`) or the REST call 400s.
- Typing out: `client.Typing(chID)`, self-throttled to one call per 10 s per channel
  (discordo). Typing in: `TypingStartEvent`, typer expiry 10 s, cleared by a
  MessageCreate from the same author.
- Uploads: `sendpart.File{Name, Reader}`; size cap via
  `state.DetermineUploadSize(guildID)` (an **arikawa state.State** method, not
  ningen). No native progress hook — wrap the Reader in a counting reader to emit
  `upload_progress`.
- Author display: `state.AuthorDisplayName(ev)` (also arikawa state, not ningen).
- Grouping rule for the timeline: same author (userID + tag) within 10 minutes.
- **Virgin-DM guard** (dissent): if a DM's history fetch returns 0 messages, refuse to
  open it ("send a message via the official client first") — opening virgin DMs from
  third-party clients is a spam-flag risk.
- Markdown: QML renders it (`Markdown.js` per PLAN). If the backend ever pre-parses
  with ningen's `discordmd`, wrap every parse in `recover()` — it can panic on hostile
  input (dissent issue #275). The QML renderer must likewise never throw on arbitrary
  content.

### QR remote auth (ported, not imported)

No library exists — port discordo's ~300-line flow as our own `internal/` package:
dial `wss://remote-auth-gateway.discord.gg/?v=2` (browser UA + Origin) → `hello`
(heartbeat every `heartbeat_interval`) → send `init` with base64 SPKI of a fresh
RSA-2048 key → `nonce_proof` (RSA-OAEP-SHA256 decrypt, reply base64url) →
`pending_remote_init` gives the fingerprint; QR content is
`https://discord.com/ra/<fingerprint>` → `pending_ticket` (decrypt user payload
`id:discriminator:avatarHash:username`) → `pending_login` gives the ticket → close WS →
`api.Client.ExchangeRemoteAuthTicket(ticket)` (upstream arikawa) with headers
`Referer: https://discord.com/login`, `X-Fingerprint`, browser UA → base64-decode +
RSA-OAEP-decrypt `encrypted_token` → user token. `timeout_ms` from hello (~2 min)
bounds the QR's life; reconnect to regenerate.

---

## 7. Security

- **The token never touches disk, argv, logs, journal, or the QML layer.** Keyring
  in/out over stdin (§4); in-memory only in the backend; the socket protocol never
  carries it except inbound on `login`, and `login` request lines are exempt from any
  logging.
- Redaction rule: every error string crossing a boundary (backend → socket state,
  QML → UI, anything → journal) passes a redact function before it moves. QML side:
  spotify's `Api.redact` regexes (authorization bearer, `token=`-style query params,
  `"token": "..."` JSON fields — extend the field list with `ticket`,
  `encrypted_token`). Backend side: an equivalent Go `Redact(string) string` applied
  at the socket-write and log-write choke points; state `error` fields are
  pre-redacted before serialization (spotify precedent).
- The socket is owner-only (0600) in an owner-only runtime dir — the auth boundary is
  filesystem permissions; no auth inside the protocol.
- REST restricted to `https://discord.com/api/v9` (arikawa's `api.Endpoint` default —
  don't override) plus the QR gateway `remote-auth-gateway.discord.gg`. Media fetches
  restricted to Discord CDN hosts only: `cdn.discordapp.com`,
  `media.discordapp.net` — the media cache refuses other hosts. QML never fetches from
  the network; it renders local cache paths handed over by the backend.
- Never `notify-send` full message bodies when content could be sensitive is a
  non-goal (notifications carry preview text by design), but notification argv must
  not include tokens/urls beyond the preview and cached icon path.
- systemd hardening block on the unit (§4) is mandatory, not decorative.

---

## DIVERGENCES FROM PLAN

Each of these was assumed by PLAN.md and disproved by recon. The reality below wins.

1. **"Fire through the shell's notification service from Service.qml" — no such QML
   API exists.** The shell's `omarchy.notifications` plugin is a freedesktop
   notification *server*. Notify via `notify-send` (Process/`Quickshell.execDetached`)
   or D-Bus `org.freedesktop.Notifications` from the backend; the shell renders it
   themed and applies DND.
2. **The PLAN's manifest example would be rejected by the shell.** It omits required
   `schemaVersion: 1` (JSON number), `name`, `version`; `"activation": "on-demand"`
   has zero consumers (services always run while enabled — idle disconnect is a
   backend/service feature); `barWidget.schema` entries must be objects
   `{key, type, label, options|min|max|step, defaultValue, description}` with booleans
   as `"On"/"Off"` enums, plus a parallel `defaults` map; `barWidget.aliases` is read
   by nothing.
3. **`omarchy shell -q quickshell.discord.panel toggle` only works if we register that
   IpcHandler ourselves in an always-loaded file** (Service.qml or BarWidget.qml — an
   IpcHandler in on-demand Panel.qml doesn't exist until first summon). Zero-code
   alternative: `omarchy shell -q shell toggle quickshell.discord`.
4. **`ningen.Connect` / `ningen.FromToken` do not exist.** Constructors are
   `ningen.New(token)`, `NewWithIdentifier(id)`, `FromState(s)`; connect via
   `n.Open(ctx)` (ready-waiting) or `session.Connect` + `ConnectedEvent` gating (§6).
5. **The Spotify protocol has no granular events** — its only event is `state_changed`
   with the complete state. Our richer event set (`message_create` etc.) is a
   deliberate departure; we copy the envelope/framing/error discipline, not the
   event model. See BACKEND_PROTOCOL.md.
6. **`shell.summon` delivers the payload as a JSON string** to `open(payloadJson)` —
   parse it; and panels missing `close()`/`opened` break `isPluginOpen`/toggle.
7. **No separate mono-font theme token** — `Style.font.family` is the system monospace
   alias; code blocks just use it.
8. **Settings are injected into the bar widget only**; Service self-serves from
   `shell.shellConfig`, and `updateEntryInline` replaces the whole inline entry
   (merge before writing).
9. **Upload progress has no native hook in arikawa** — counting-reader wrapper is the
   only route to `upload_progress`.
10. **No QR remote-auth library exists to import** — only
    `ExchangeRemoteAuthTicket` is upstream; the WS protocol is ported from discordo
    (~300 lines including RSA-OAEP), as our own code against upstream arikawa.
