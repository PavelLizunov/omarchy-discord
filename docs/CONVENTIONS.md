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
│                                   # backend-runtime.sh, remove-runtime.sh
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

`omarchy plugin add <repo> --enable`. The installer runs no hooks, and neither does
`omarchy plugin update` (a `git merge --ff-only` in place), so the enabled Service
owns both install and upgrade: `DaemonManager` runs `scripts/backend-runtime.sh sync`
once per Service load, which reinstalls and `try-restart`s only when the shipped
backend differs from the installed one (stamp file + mtime check — see §4 and
`docs/TECHNICAL.md`, "Runtime install and upgrade"). The spotify
`installBundledBackendIfNeeded` pattern is the first-load half of this.

`scripts/install-local.sh` for dev: validate → `setup.sh` → `systemctl --user
try-restart` → **rsync a copy** of the checkout into the plugins dir (`-a --delete`,
excluding `.git`; never a symlink — `omarchy plugin validate` refuses symlinks, and a
symlinked checkout would hot-reload the shell on every in-tree edit), refusing an
existing path that is a symlink, a non-directory, or a directory without a
`manifest.json` → `omarchy-shell shell rescanPlugins` → poll `omarchy plugin list
--json` → enable. It copies rather than links, so **re-run it after every source
change**.

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

**Secondary text is `Api.secondaryColor(Color.muted, Color.foreground, Color.background)`**,
exposed as `muted` on every surface root (and `Service.secondaryColor` for the
markdown context) — never `Color.muted` directly. The shell itself never paints text
with `muted`, and on several bundled themes (rose-pine, catppuccin-latte,
flexoki-light, tokyo-night, everforest, …) it sits at a 1.5–2.5 contrast ratio against
the background. The helper keeps the theme's `muted` when it clears 3:1 and otherwise
uses the foreground at 60 % alpha (the shell's own placeholder construction). It is
a legibility guard, not a light/dark branch. Rich-text covers that are both ink and
background (the spoiler) take the opaque `Api.blend` of it, since a translucent
colour would show the text through. Status dots: online = `Color.accent`, idle =
`muted`, dnd = `Color.urgent`, offline = `Util.alpha(Color.foreground, 0.35)`.

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

What ships (Service.qml `maybeNotify` / `notificationArgs`), per `message_create`
with `notify: true`:

```
notify-send --app-name=Omarchy Discord --urgency=normal [--icon=<cached avatar path>] -- <summary> <body>
```

- `summary`: `"<author> in #<channel_name>"`; DMs whose `channel_name` is the author:
  `"<author>"`; group DMs: `"<author> in <channel_name>"`. `<`/`>` stripped (the card
  renders the summary as auto-text).
- `body`: `Markdown.plainText(content)` → `Api.redact` → cut to 200 chars (`…`) → HTML
  entities escaped (the shell's card renders the body as `Text.StyledText`) → ` 📎`
  appended when the message has attachments → ` (+N more)` when it flushes a held burst.
- `--icon` only when the author's avatar (`avatar_url`, size 64) is **already** in
  `mediaPaths`; the lookup itself queues the fetch so the next notification has it.
  Never a URL, never an icon-theme name.
- No actions (`--action` support varies across servers), no hints, never critical.
- The argv builder is `property var notifyCommand(args)` (default prepends
  `notify-send`) so a harness can record instead of execute; `fireNotification` runs it
  through `Quickshell.execDetached`.

Skip rules, in order (`notifySkipReason` returns the name, `""` means notify):
`not-notify` (event's `notify` false) · `not-ready` (lifecycle ≠ ready) · `mode-off` ·
`own` (author is `selfId`) · `dnd` (`state.presence === "dnd"`) · `mode-mentions`
(mode is *Mentions and DMs* and neither `message.mentions_self` nor `guild_id == null`)
· `viewing` (`panelActive` — Panel.qml publishes `Window.active` — and the full panel
surface is visible and `channel_id === currentChannelId`). Rate limit: one
notification per channel per 3 s (`notifyWindowMs`); arrivals inside the window are
held (`notifyHeld`, last message wins, count accumulates) and flushed by one timer as a
single notification with `(+N more)`. The timer is armed for the earliest held
channel's window end and never re-armed by later arrivals, so continuous traffic
cannot postpone a flush; `flushNotifications` re-arms for whatever is still inside
its window.

### Media (fetch_media mirror)

`Service.mediaPath(url, size)` returns the cached local path or `""`. It is safe inside
bindings because a miss does no I/O: it records the key in `mediaWanted` (in place,
nothing binds to it) and `Qt.callLater(flushMediaRequests)` issues the `fetch_media`
on the next event-loop turn (`mediaPending` marks keys in flight). A socket write from
inside a binding evaluation produced "Binding loop detected" warnings on every
media-bearing row; the deferred flush is what keeps that at zero. `flushMediaRequests`
also runs on every connect, re-issuing wanted keys whose fetch died with the socket.
Resolution writes `mediaPaths` wholesale (a cache-hit response or `media_ready`). `media_ready` is keyed by `url` only: one outstanding size
adopts the path directly, several outstanding sizes are re-requested and each key
resolves only from its own size-specific `cached: true` response (the finished one is
now a cache hit). Failures go to `mediaFailed` so a binding never loops. A cached path
the backend's LRU has since evicted fails in the `Image`: every consumer reports
`Image.Error` through `Service.mediaError(path)` (rows via `ctx.mediaError`), which
drops the key from `mediaPaths` and re-requests it — once per key (`mediaRetried`). All three maps reset per socket connection. `markdownCtx` carries `mediaPath`, `emojiPath(id)`
(`https://cdn.discordapp.com/emojis/<id>.png` at size 32, PNG even for animated —
rich text cannot animate), `emojiSize` (`font.body × 1.4`), `imagePreviews`, and reads
`mediaPaths`, so rows re-render when media lands without touching the service.
Sizes: avatars and guild icons 64, emoji 32, attachments/embeds original (`size` 0 =
omitted). Off-CDN URLs (embed images of other sites) are filtered with
`Api.isCdnUrl` before any request. `set_config {media_cache_mb}` is sent on every
connect and whenever `mediaCacheMB` changes. Spoiler images are never loaded until
revealed (`Timeline.revealed`, per message id, reset on channel change).

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

- Path: `$XDG_RUNTIME_DIR/omarchy-discord/backend.sock`, the single location the
  backend binds (its own fallback is `/run/user/<uid>`; never `/tmp`). QML cannot
  learn the uid, so when `XDG_RUNTIME_DIR` is unset the client leaves the path
  empty, never connects, and surfaces a clear `lastError`.
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
- **Accepted deviation — the message timeline** (`components/Timeline.qml`): an int
  model resets the ListView on every change, so the timeline keeps a small `ListModel`
  of message ids and diffs each new `messages` array against it (history prepends
  become `insert(0, …)`, new messages `append(…)`; anything else resets with a
  captured/restored scroll anchor). Its rows use `reuseItems: false` because a reused
  variable-height delegate re-lays out in polish and shifts the visible rows. Feeding
  it: pass a **new** array whose unchanged elements are the **same object references**
  (Service.qml does `concat`/`slice`, never deep copies).
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

Three focus zones — **sidebar**, **timeline**, **composer** — plus a fourth,
**members**, that exists only while the member pane is shown; each with a roving
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
- **Esc ladder**: one `Keys.onPressed` on the FocusScope (`Panel.handleKey`) walks,
  in order: dismiss transient popup → composer chip cursor → composer edit / reply
  mode → leave the composer (marking read) → leave the timeline (marking read) →
  channel list → server rail. **The rail is the end of the ladder**: Esc there is
  consumed and does nothing — it must not close the panel. Closing is the Close
  button, closing the window, the bar widget, or the `quickshell.discord.panel` IPC.
  Esc on the login screen and the backend-down screen still closes, since neither
  has a zone to fall back to.
- **Focus on open** is an acceptance criterion: `open()` →
  `Qt.callLater(forceActiveFocus)` for the FloatingWindow panel; KeyboardPanel's
  Exclusive→OnDemand prime for the quick switcher and bar popup. Opening a channel
  focuses the composer.
- Keymap per PLAN.md (Ctrl+K quick switcher, Alt+h/l zone moves, j/k roving, `R E Y O`
  message actions, `↑` edit-last in empty composer, Ctrl+/ cheatsheet). `Enter` on a
  timeline row reveals its covered spoiler images first, then activates; `O` opens an
  attachment from its cached local path when the cache has it.
- **Timeline text selection**: the message body is a `TextEdit { readOnly: true;
  selectByMouse: true; activeFocusOnPress: false; persistentSelection: true }`, not a
  `Text`. `activeFocusOnPress: false` is the whole focus mechanism — the drag selects
  while the Timeline FocusScope keeps `activeFocus`, so the roving cursor and every
  zone key survive; `persistentSelection` keeps the band painted with the keyboard
  elsewhere. Because the TextEdit covers the row's fill `MouseArea`, the row hover
  reads a `HoverHandler` and the click comes from a `TapHandler` on the text (a drag
  produces no tap, so selecting never moves the cursor). Selection is per message and
  cannot span rows; `Timeline.selectionOwner` holds the single owner and is cleared on
  a new selection, a plain click (Qt's own behaviour), delegate destruction, channel
  change and leaving the zone. `Ctrl+C` copies it — normalising Qt's U+2028 / U+2029
  separators to `\n`, since `selectedText` never returns a newline — and `Esc` peels
  it before leaving the zone (after an armed delete, before `escapeRequested`). `Y`
  still copies the whole cursor message. Hovering a link offers a "Copy link" chip
  (top-right of the row, behind a `Loader` so an unhovered row pays nothing); the
  offer is sticky behind a 600 ms timer because the chip covers the text it is
  offered for, and a direct binding to `hoveredLink` would oscillate. `L` is the
  keyboard equivalent on the cursor row.
- **Long tokens**: `Text.Wrap` is Qt's `WrapAtWordBoundaryOrAnywhere` and already
  breaks a 200-character URL mid-token — do not "fix" it to `WrapAnywhere`, which
  breaks ordinary prose mid-word. The one surface that overflowed was the fenced code
  block: Qt gives `<pre>` `white-space:pre`, which refuses to wrap at all, so
  `Markdown.blockCodeStyle` adds `white-space:pre-wrap` (block fences only — inline
  `<code>` is a span under the same wrapMode and already wraps).
- **Threads** (`t`): the flat channel list never shows thread rows —
  `list_channels` carries every active thread the cache knows (hundreds on a busy
  guild), so `Api.visibleChannels` hides `type: "thread"` and `Panel.threadCounts`
  reduces them to a per-parent count for the muted "⌥ N threads" affordance. `t` on
  a text / announcement / forum row (or on the open channel from the timeline)
  toggles `Panel.expandedThreads[parentId]` and calls `Service.listThreads(parent)`;
  `Panel.channelRows` splices `Service.threadsFor(parent)` — the `list_threads`
  mirror, or until it answers the thread rows from the channel list sorted newest
  first — beneath the parent as indented rows. `Enter` on a thread opens it like any
  channel (header: "#parent › thread"); `Enter` on a forum only expands it (a forum
  is not openable). Expansion state is panel-local (reset with the guild). Thread
  rows take `read_state_changed` like channels (`applyReadState` patches
  `threadsByParent` too).
- **channel_update bursts**: THREAD_LIST_SYNC arrives as one `create` per active
  thread (749 on one real guild, usually before the `open_channel` response) and the
  backend drops a client that falls behind. `Service.noteChannelUpdate` therefore only
  marks the guild / thread parent dirty and `structureFlushTimer` (300 ms) issues one
  `list_channels` per guild and one `list_threads` per *loaded* parent — never one
  request per event.
- **Member pane** (`components/MemberList.qml`): toggled with `m` outside text inputs,
  `Alt+m` everywhere (the composer claims it on its TextArea and emits
  `membersRequested`, else the TextArea would type an "m"), and the Members button
  on the channel-title row. The toggle (`Service.membersWanted`) lives in the service so a re-summoned
  panel keeps it; `Service.syncMembers()` subscribes the current channel while the
  pane is wanted **and** the full panel is registered visible, and unsubscribes on
  channel change, toggle off, panel close / destruction (`setUiVisible("full-panel",
  false)`); subscriptions are per socket connection, so a disconnect clears
  `membersChannelId` and the next `ready` re-subscribes. `member_list_update` is a
  full replacement (`memberList`), `presence_update` patches every row of that user
  in a copy. "Loading members…" until the first list; `membersTimer` (15 s) flips
  `membersTimedOut` → "No member list for this channel". Rows come from
  `Api.memberRows` (every group as a header with Discord's total count, members
  beneath in wire order). Keys: `j/k` (headers skipped), `g/G`, `Y` copies
  `@username`, `Esc` / `Alt+h` back to the composer; `Alt+l` from the composer enters
  it; the Tab cycle is rail → channels → timeline → composer → members → Members
  button → Log out → Close. When the pane disappears while it is the zone, the zone
  falls back to the composer.
- **One key table.** `Keymap.js` holds every binding once (`ENTRIES`, grouped by
  `ZONES`); the panel footer renders `Keymap.footer(state, …)` over `FOOTER` id lists
  and the cheatsheet renders `Keymap.sections()`, so hints and cheatsheet cannot drift.
  Adding a key means adding an entry (and, for a footer hint, its id to a state);
  `missingFooterIds()` must stay empty (harness assertion). Global chords go through
  `Panel.handleGlobalKey` before any zone: `Ctrl+K` / `Ctrl+/` everywhere (the composer
  claims them on its TextArea — `Ctrl+K` would otherwise delete to end of line — and
  emits `switcherRequested` / `cheatsheetRequested`), `/` and `?` only outside text
  inputs. While a modal overlay is shown (`Panel.overlayShown`) `handleKey` ignores
  everything; the overlay accepts every key itself.
- **Guild entry**: entering a guild from the rail (Enter / `l` / `→`, a guild tile
  click, or Tab / `Alt+l` into the channel column — `j`/`k` cursor movement never
  opens anything) opens that guild's last-visited channel, falling back to a channel
  named `general` (case-insensitive) then to the first openable channel in list
  order; the DM pseudo-guild (`"dms"`) restores only what was remembered, because a
  DM inbox has no default and auto-opening an untouched DM trips the virgin-DM guard
  (§6). The map is `Service.lastChannels` — a recency-ordered `[{ g, c }]` persisted
  as a JSON string under `lastChannels` on the shell.json entry via `persistOpaque`,
  capped by `Api.LAST_CHANNEL_CAP` — recorded from the `open_channel` response (the
  only place the guild id is authoritative) and restored through
  `Service.enterGuild` / `resolveGuildEntry`, with the single `pendingGuildEntry`
  slot as the stale guard for a list that has not landed yet.
- **Login screen** (Panel.qml `loginStops`): its own Tab cycle — Scan QR (default
  focus) → token field → Log in → Close; in the QR view Cancel/Try again → Close. All
  stops are `focusable` qs.Ui Buttons / the TextField with `activeFocusOnTab: false`
  so Qt's chain never competes. `Esc` cancels a running QR flow (`cancel_qr_login`) —
  code in hand or not — dismisses a finished one, else closes the panel. `showLogin`
  includes `qr_pending`; the QR view is gated on `qr_pending || qrBusy || qr !== null`
  so it never flashes the choices between a response and its events. A socket
  reconnect during `qr_pending` nulls `qr` and waits for the backend's replayed
  `qr_code` ("Reconnecting to QR login…"); when none arrives within `qrReplayMs`
  (3 s) `qrMissing` flips and the action button becomes Try again
  (`restartQrLogin`: cancel, then start).

### Quick switcher (`QuickSwitch.qml`, service-owned)

- Lives in Service.qml behind a `Loader` (created on first use) so the IpcHandler
  `quickshell.discord.switcher` `toggle|open|close` works from any app; the panel's
  `Ctrl+K` / `/` call `service.openSwitcher()`. The surface is a full-screen
  `PanelWindow` (`WlrLayer.Overlay`, scrim, centered ~560 px card — the shell's own
  emoji-overlay shape; `qs.Ui KeyboardPanel` is the bar-anchored variant and needs a
  bar the Service does not have) with KeyboardPanel's focus prime replicated:
  `WlrKeyboardFocus.Exclusive` until 75 ms after the surface maps, then `OnDemand`.
  The window goes on the panel's screen when the panel is open, else the default.
- Keys are intercepted on the search `TextField` itself (`Keys.priority: BeforeItem`):
  `↑/↓`, `Tab/Shift+Tab`, `Ctrl+j/k`, `Ctrl+n/p` move (wrapping), `PgUp/PgDn` by 8,
  `Enter` activates, `Esc` clears the query then closes; everything else is text.
  Typing debounces 80 ms into `service.quickSwitch(query, cb)`, which drops stale
  responses by sequence number and never touches the panel footer (`lastError`).
- `Enter` → `close()` then `service.openPanel({channel_id})` (hide/summon split across
  turns; the same payload path as the bar's middle click). Logged out: a single
  "Log in to Discord" row → `openPanel({})`. Mouse: hover moves the cursor, click
  activates, a click on the scrim closes. The switcher registers
  `setUiVisible("quick-switch")` so it counts for idle-disconnect.

### Cheatsheet (`components/Cheatsheet.qml`)

- A modal `FocusScope` overlay inside the panel window (scrim + centered card), shown by
  `Ctrl+/` anywhere and `?` outside text inputs; `Esc`, `Ctrl+/`, `?`, `Enter` close it,
  `j/k`, arrows, `PgUp/PgDn`, `g/G` scroll; every other key is swallowed. On close the
  panel runs `focusZone()` (deferred) so the keyboard returns to the last zone.

### Emoji picker (`components/EmojiPicker.qml`)

- Same overlay shape, opened by `E` on a timeline row (`Panel.openPicker`): the search
  field owns the keyboard; arrows / `Ctrl+h/j/k/l` / `Tab` move, `Enter` picks, `Esc`
  clears the filter then closes. The model is `Emoji.sections(reactions, frequent,
  server, catalog, query, limit, currentGuildId)` → `[{id, title, cells}]` in the
  order **reactions** ("Toggle", `me` marks ours) → **frequent** → **server** (one
  section per guild, id `server:<guild_id>`, the selected guild first) → **all**;
  `Emoji.move` navigates the flattened grid (rows within a section, crossing into the
  neighbour at the edge, keeping the column).
- Server emoji come from `list_emoji` (loaded on `ready` and again on
  `guilds_synced`; `Service.serverEmoji` is the guilds array), each cell an `<img>`
  through the media cache (`emojiPath`, size 32, PNG even for animated); `Enter`
  reacts with `name:id`. Two guards, both learned on a real account with ~1,500
  custom emoji: the picker's `sections` are `[]` while hidden (its Repeater is not
  virtualized — every cell would be instantiated, and every image requested, the
  moment `list_emoji` lands), and each guild section is capped (`Emoji.SERVER_CAP`
  48 without a filter, 200 with one). Typing `:name:` shortcodes in the composer is
  out of scope.
- `picked(emoji)` is wire form (unicode or `name:id`). `Service.toggleReaction` sends
  `unreact` when the loaded message already carries our reaction, else `react` and
  bumps `frequentEmoji` (`Emoji.bumpFrequent`, cap 16, persisted as a JSON string under
  the `frequentEmoji` key on the shell.json entry via `persistOpaque`, read back in
  `syncSettings`). Reaction chips in `MessageRow` emit `reactionClicked` → Timeline
  `reactionToggled` → the same function. State comes back through `message_update`.
- The unicode catalogue is the shell's `$OMARCHY_PATH/shell/plugins/emojis/emojis.json`
  read through a `FileView` on first use (`ensureEmojiCatalog`); `Emoji.FALLBACK` when
  missing. `Emoji.filter` ranks canonical-shortcode matches above whole-word, prefix,
  then substring matches.

### Composer zone (`components/Composer.qml`)

- The zone is a `FocusScope` whose keyboard owner is either the `TextArea` or a
  zero-size `chipFocus` Item (chip cursor ≥ 0). Intercept keys on the **TextArea
  itself** with `Keys.priority: Keys.BeforeItem`: key events go to the focused item
  first, so a handler on an ancestor would only see what the TextArea left unaccepted.
  Anything the composer does not accept bubbles to the panel `FocusScope`, which
  treats a focused composer like a text input: only `Alt+↑/↓` (channel stepping) is
  panel-level; single-letter shortcuts (`r` reload…) never fire while it has focus.
- **Esc semantics** (innermost first): chip cursor → back to the input; edit mode →
  cancel (draft restored); reply mode → cancel; else `leave()` → the panel marks the
  channel read and focuses the timeline. The timeline's own Esc consumes the key while
  a delete is armed (disarms) and otherwise goes to the sidebar.
- **Enter** sends (`Shift+Enter` newlines); with staged chips it uploads instead;
  in edit mode it saves. Empty text never sends. Both send and upload clear the input
  and reply mode synchronously on Enter — the completion callback never touches the
  composer, which may be showing another channel by then; the chips alone show upload
  progress. A failed upload restores its text like a failed send (`draftRestored`);
  while the composer is in edit mode a restore merges into `savedDraft` instead of
  the input. `↑` edits the newest own,
  non-pending message only when the input is empty; the typed-but-unsent draft is
  stashed and restored on cancel.
- **Chip cursor**: `Tab`/`Shift+Tab` enter the chips from the input (forwards from
  the first, backwards from the last when arriving via the panel's Tab cycle),
  `←` at the input's start / `→` at its end also enter them, `←/→/h/l` move,
  `x`/Delete/Backspace remove the focused chip **and its file**, `Tab` past the last
  chip continues the panel cycle. Chips are never editable while their upload runs.
- **State placement**: drafts (`Service.drafts`, mutated in place — nothing binds to
  it) and staged attachments (`Service.staged`, replaced wholesale — chips bind to it)
  live in Service so the panel can be destroyed between summons. The composer keeps
  only modes (`editingId`, `replyToId`) and the chip cursor; both reset on channel
  change.
- **Optimistic rows** are ordinary message objects with `id: "pending-<n>"` and
  `pending: true`; `Api.compareRows` sorts them after every real snowflake. The send
  response's `nonce` maps to the pending row; the `message_create` echo with that
  nonce (and our own author id) replaces it in place. An echo that beats the response
  is appended normally and the response then just drops the pending row (matched by
  `message_id`). Failure removes the row, writes the text back to the draft and emits
  `draftRestored(channelId)`.
- **Clipboard pipeline** runs in Service (`stageClipboardImage`): `wl-paste
  --list-types` → best `image/*` (png > jpeg > webp > gif > any non-svg) → `sh -c
  'umask 077; mkdir -p …; exec "$0" "$@" > "$OD_OUT"'` with `wl-paste --type <mime>`
  → `stat -c %s`. Both wl-paste argv arrays come from the injectable
  `clipboardCommand(args)` so a harness can substitute a script. No image type means
  the TextArea's own `paste()` runs. Staged files:
  `$XDG_RUNTIME_DIR/omarchy-discord/staged/paste-<yyyyMMdd-HHmmss>-<n>.<ext>`.
- `upload_progress` events are routed by `upload_id` (the request id Service used)
  to the channel and by `filename` to the chip; the chips show `bytes_sent/bytes_total`.
- Ack-on-read: `Timeline.viewing` (timeline **or** composer zone focused) gates
  `reachedBottom`; `active` (timeline zone only) still drives the cursor and border.

---

## 4. Backend conventions (lifecycle, packaging, keyring)

### Build & install locations

| What | Where | Never |
|---|---|---|
| Build output | `$XDG_CACHE_HOME/omarchy-discord/target` | the plugin tree (hot reload kills builds) |
| Installed binary | `~/.local/lib/omarchy-discord/omarchy-discord-backend` (dir 700, bin 755; override `OMARCHY_DISCORD_RUNTIME_DIR`) | the plugin tree |
| Install stamp | `~/.local/lib/omarchy-discord/installed-version` (mode 600; the manifest version `setup.sh` installed from, its mtime the upgrade marker) | the plugin tree |
| Shipped prebuilt | `backend/dist/x86_64/omarchy-discord-backend` (gitignore-negated, real file, selected by `uname -m`) | a symlink |
| Socket | `$XDG_RUNTIME_DIR/omarchy-discord/backend.sock` (0600; parent dir created; stale file unlinked on bind; removed on shutdown) | — |
| Media cache | `$XDG_CACHE_HOME/omarchy-discord/media/` (LRU capped by `mediaCacheMB`) | — |
| Staged uploads | `$XDG_RUNTIME_DIR/omarchy-discord/staged/` | — |

`build-backend.sh`: prebuilt-wins (if `dist/$(uname -m)/omarchy-discord-backend` is a
real executable file, `install -m 755` it into the runtime dir); a symlink there is
refused outright; else require the Go toolchain and build with output outside the
tree, then `install`. Only `x86_64` ships a prebuilt today, so other architectures
need Go. Backend gitignore mirrors spotify's dist-negation block (verify with `git
check-ignore -v backend/dist/x86_64/omarchy-discord-backend`).

Exit codes, shared by `build-backend.sh`, `setup.sh` and `backend-runtime.sh sync`
and mapped to distinct UI messages in `DaemonManager.qml`: **30** = no prebuilt for
this architecture and no Go toolchain; **31** = the build or install failed; `sync`
additionally uses **0** = already current and **10** = installed or updated. Never
collapse 30 and 31 into one message — "Go is not installed" is wrong and confusing
for a build that simply failed.

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

`ProtectSystem=strict` leaves `/usr` and the system hierarchy read-only while
`ProtectHome` stays at its default `no`, so `~/.cache/omarchy-discord/` and
`$XDG_RUNTIME_DIR/omarchy-discord/` remain writable — verified on the live unit and
with a transient `systemd-run --user` unit carrying the same settings. Do not "fix"
this with `ReadWritePaths`. `setup.sh` rewrites `ExecStart` in the installed copy when
`OMARCHY_DISCORD_RUNTIME_DIR` is set, so the unit points at the binary it installed.

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

### Keyring — secret-tool over stdin, inside the backend

The token travels over **stdin, never argv** (argv is visible in `ps`), never a file.
Unlike spotify — which shells out from QML through a `keyring-store.sh` — the Go
backend owns the keyring end to end in `backend/internal/keyring`: it runs
`secret-tool` as a child and writes the token to its stdin. There is no keyring shell
script in this plugin, and the frontend never holds the token beyond the `login`
command it immediately clears.

Attributes (shared by store, lookup and clear):

```
secret-tool store --label='Omarchy Discord user token' \
  service quickshell-discord kind user-token
```

- Lookup: `["secret-tool", "lookup", "service", "quickshell-discord", "kind", "user-token"]`.
- Clear: `secret-tool clear service quickshell-discord kind user-token` — removes
  **one** entry per invocation; purge scripts loop (cap 20 iterations).
- QML write side: `Process { stdinEnabled: true; onStarted: { write(token + "\n");
  token = "" } }` — clear the property immediately after write.
- Processes that may see the token consume stdout/stderr with discarding `SplitParser`
  handlers — never `StdioCollector` — so secrets can't land in the journal.
- The backend reads the token from the keyring itself on start, or receives it via
  the socket `login` command or the in-process QR flow; either way it holds it only
  in memory.

### Teardown

`scripts/remove-runtime.sh [--purge]`: stop unit, remove unit file + installed binary
+ install stamp (so the runtime dir can be `rmdir`ed), `daemon-reload`; config dir moved to `.bak.<timestamp>` (deleted only with `--purge`
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
RSA-2048 key → `nonce_proof` (RSA-OAEP-SHA256 decrypt the nonce, reply the base64url of the RAW decrypted nonce — NOT a digest; see internal/remoteauth) →
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
