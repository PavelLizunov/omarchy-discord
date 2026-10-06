# Omacord

A keyboard-first, text-only Discord client for the Omarchy desktop. Maintained
in [PavelLizunov/omarchy-discord](https://github.com/PavelLizunov/omarchy-discord),
forked from [zgt/omarchy-discord](https://github.com/zgt/omarchy-discord),
originally authored by Matt Calayo. Omacord is an unofficial client and is not
affiliated with Discord.

The plugin ID remains `quickshell.discord`; backend paths, the systemd unit,
keyring identifiers and existing window titles remain unchanged for compatibility.
Omacord and the original plugin share these identifiers and are not separate,
side-by-side installations.

A text-only Discord client shipped as an Omarchy Quattro shell plugin
(`quickshell.discord`): a themed Quickshell panel and bar widget in front of a small
Go backend, without Electron. Same architecture as
quickshell.spotify: QML owns everything visible, a systemd user unit owns the
Discord connection, and a private JSON-lines socket joins them.

What you get: the bar mark with a mention badge and unread dot; a panel with the
server rail, channel list (threads and forums included), a virtualized timeline with
history paging, markdown, text embeds, reactions and spoilers; a composer with
send / reply / edit / delete, typing both ways and screenshot paste; a member list
with presence; the `Ctrl+K` quick switcher that works from any app; desktop
notifications through the Omarchy notification center; QR login; and every colour
from the active Omarchy theme, light themes included. It is keyboard-first
throughout — `Ctrl+/` shows the cheatsheet.

**Status: development snapshot, not a verified release.** The text-only changes
are published on `feature/omacord-text-only`; the fork's `main` branch still
contains the inherited upstream version. Clone the development branch explicitly
to inspect this build. Fresh full-suite, visual and live acceptance are pending.
`docs/PLAN.md` records the inherited upstream roadmap, not Omacord release
acceptance. The inherited voice backend supports joining a guild voice channel,
talking and listening, muting / deafening, and seeing participants and speakers. Video, screen share, server
management and multiple accounts remain non-goals, and stage channels stay hidden
entirely.

## Local text-only build

This checkout keeps the Discord connection and voice backend but renders a
text-only client. It never requests `fetch_media`: no server icons, avatars,
attachment previews or custom-emoji images are downloaded or read from the old
media cache. Custom emoji use `:name:`; attachments show filename, size and an
explicit link. Opening a link delegates to the system handler, which may download
it. Pasting an image for an explicit upload remains supported; its staging chip
is text-only. The locally generated login QR remains the only client image.

`ClientView.qml` and `SwitcherView.qml` are ordinary Qt Quick items. The running
Omarchy wrappers and MCP fixtures consume these same components. `ui/` carries
the native host controls and theme tokens with desktop I/O removed; `Panel.qml`
supplies the active Omarchy theme. The MCP does not need Quickshell imports,
mock modules, or a production desktop connection to render these consumers.

The local development checkout is `/home/slovn/Work/omarchy-plugins/quickshell.discord`; Omarchy
loads the copy under `~/.config/omarchy/plugins/quickshell.discord/`. Preserve
these local changes before pulling upstream updates. `imagePreviews` is fixed
to `Off` in this build; `mediaCacheMB` is an old disk-cache setting, not a RAM
reservation. Existing cached files are retained and are not displayed.

Run the inert checks from this checkout:

```sh
node tests/no-media.cjs
node --test components/harness/api.test.js components/harness/markdown.test.js
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software \
  /usr/lib/qt6/bin/qmltestrunner -input tests/visual
```

For QML Preview MCP 0.3.0, render `tests/visual/Preview.qml` with
`readyProperty: ready`, `locale: ru_RU`, no additional imports, and explicit
dependency hashes. States: `chat`, `empty`, `loading`, `error`, `login`, `qr`,
`members`, `voice`. `lightTheme` and `themeRadius` select fixture theme variants.
`tests/visual/SwitcherPreview.qml` renders the real quick switcher. The fixture
QR encodes an `example.invalid` URL and cannot sign into Discord. Rendering is
separate from keyboard checks and live authenticated behavior.

Use `omarchy-shell quickshell.discord.panel status` for connection state and
text-mode media queue counts. It does not expose messages or credentials.

Reaction-only message updates do not reparse unchanged Markdown. Text edits
and context changes still update formatting. `tests/visual/tst_computation.qml`
checks parser call counts using the production MessageRow.

Voice participants and members without a cached display name use a known
message-author name when available, otherwise `User #<id>`. This fallback
does not issue REST lookups or download avatars; the underlying backend may
not have received that participant's profile yet.

## Screenshots

_Coming with the first release: panel with an open channel, the member list, the
quick switcher, and the same views on a light theme._

## Install

The URL below targets the Omacord fork's default branch, `main`. It currently
installs the inherited upstream version, not the text-only development snapshot
described above. Enabling the plugin can install and start the backend; installing
this fork replaces the plugin with the same `quickshell.discord` ID.

```sh
omarchy plugin add https://github.com/PavelLizunov/omarchy-discord --enable
```

No build step and no toolchain: the repo ships a prebuilt x86_64 backend at
`backend/dist/x86_64/omarchy-discord-backend`. Omarchy runs no install hooks, so the
enabled service does the install itself the first time it loads — the binary goes to
`~/.local/lib/omarchy-discord/`, its static user unit to
`~/.config/systemd/user/omarchy-discord.service`, and the unit is then started. On any
other architecture that same step builds the backend from source and needs Go
installed; until it can, the panel says so instead of showing the login screen.

`omarchy plugin update quickshell.discord` upgrades the frontend and the backend
together. On every load the service compares the backend it ships against the one
installed, reinstalls when they differ, and restarts a running backend so the new
binary takes over immediately.

The unit is never enabled at login: the plugin starts it and keeps it connected while
the `stayConnected` setting is On (the default). Requirements: Omarchy 4 with the
Quickshell shell, `secret-tool` (GNOME keyring) for the token, `notify-send`,
`wl-paste` for image paste, `xdg-open`, `opus` (ships with Omarchy as a
pipewire-audio dependency).

### Development install

```sh
git clone --branch feature/omacord-text-only https://github.com/PavelLizunov/omarchy-discord
cd omarchy-discord
scripts/install-local.sh          # --section left|center|right
```

`install-local.sh` validates the manifest, installs the backend and unit, rsyncs the
checkout into `~/.config/omarchy/plugins/quickshell.discord/` (a copy, not a symlink,
because `omarchy plugin validate` refuses symlinks and in-tree edits would hot-reload
the shell), rescans, and enables the widget. Run it only for an intentional,
verified deployment: it can replace installed code and restart the backend.
Saving source changes alone does not deploy them. The backend is reinstalled
whenever a file under `backend/` is newer than the installed binary
(`scripts/setup.sh --reinstall-backend` forces it); on x86_64 that means installing
the committed prebuilt, elsewhere building it with Go, always outside the plugin tree.
`OMARCHY_DISCORD_RUNTIME_DIR` relocates the binary; setup rewrites the installed
unit's `ExecStart` to match. See `docs/TECHNICAL.md` for the harnesses, the quality
gate, and how to refresh the committed prebuilt.

### Removal

```sh
~/.config/omarchy/plugins/quickshell.discord/scripts/remove-runtime.sh --purge
omarchy plugin remove quickshell.discord --yes
```

In that order, and don't skip the first line: removing the plugin directory leaves the
backend behind as a systemd user unit holding a live Discord session and your token in
the keyring. `remove-runtime.sh` stops the unit, deletes it and the installed binary,
and reloads systemd. Without `--purge` it keeps the keyring entry and the media cache
and moves `~/.config/omarchy-discord/` aside as a timestamped `.bak`; with `--purge` it
also deletes `~/.config/omarchy-discord/` and `~/.cache/omarchy-discord/` and clears the
`quickshell-discord` keyring entries.

If the plugin directory is already gone, the same cleanup by hand:

```sh
systemctl --user stop omarchy-discord.service
rm -f ~/.config/systemd/user/omarchy-discord.service
rm -rf ~/.local/lib/omarchy-discord ~/.cache/omarchy-discord ~/.config/omarchy-discord
systemctl --user daemon-reload
secret-tool clear service quickshell-discord kind user-token
```

## Hyprland binds

```ini
bindd = SUPER SHIFT, D, Discord, exec, omarchy-shell quickshell.discord.panel toggle
bindd = SUPER SHIFT, K, Discord quick switcher, exec, omarchy-shell quickshell.discord.switcher toggle
```

`open` and `close` are also available on both targets. The zero-config fallback for
the panel is `omarchy-shell shell toggle quickshell.discord`. The panel takes the
keyboard the moment it maps, so a bind drops you straight into the last zone; the
switcher is a small themed overlay (the "mini player"): unread channels and DMs
first, then recents, each with the last message; type to search, `Enter` opens the
panel on that channel.

### Persistent window

With the `window` setting on `Persistent` the client window is mapped from shell
start instead of being created on each open, so Hyprland rules can place it. One rule
puts it on the scratchpad:

```lua
o.window({ title = "^(Omarchy Discord)$" }, { workspace = "special:scratchpad silent" })
```

```sh
omarchy bar set quickshell.discord window Persistent
```

The bind summons the window to the workspace you are on and focuses it — it does not
reveal the scratchpad in place — and pressing it again sends the window back to its park
(the special workspace the rule first mapped it on) without switching you away. So the
window boots parked on the scratchpad but opens wherever you are. Without the rule (on a
normal workspace) dismissing just unmaps the window instead. `SUPER+W` closes the window
too; the next press of the bind maps and summons it again. Read acks, refresh and the
member subscription follow window focus, not map/unmap, so a visible but unfocused window
does not mark channels read.

## First login

Click the bar mark (or use the bind). The login screen offers two ways in; `Tab` cycles
Scan QR → token field → Log in → Close, `Enter` activates, `Esc` closes.

**Scan QR** (default): the panel shows a QR code; open the Discord mobile app, go to
Settings › Scan QR Code, scan it, and confirm on the phone. The panel follows along
("Logging in as … — confirm on your phone", then "Approved, connecting…"). The code is
valid for about two minutes; when it expires, is declined, or you press `Esc`, a
[Try again] button produces a fresh one. If the backend connection blips mid-flow the
code comes back on its own (briefly "Reconnecting to QR login…"); should it not, the
panel offers [Try again] and `Esc` still cancels. No password or captcha is involved;
the token the phone hands over goes straight into the keyring.

**Paste a token** instead: paste a user token into the field and press Enter. The token
goes over the socket to the backend, which stores it in the GNOME keyring via
`secret-tool` over stdin. It is never written to disk, logs, or shown in the UI.

When a session expires (token revoked elsewhere) the same screen comes back with
"Session expired — log in again".

From a terminal instead:

```sh
printf '%s' "$TOKEN" | ~/.local/lib/omarchy-discord/omarchy-discord-backend login
```

`login` reads the token from stdin. `omarchy-discord-backend check` prints an
environment summary.

## Keyboard

Four focus zones: the **sidebar** (server rail + channel list; `h`/`l` or the arrows
move between the two columns), the **timeline**, the **composer**, and the **member
list** while it is shown (`m`). Focus is always visible — the active column, timeline,
or pane carries the focus border — and opening a channel focuses the composer.
`Alt+h` / `Alt+l` move between zones, `Tab` cycles every stop including the panel
controls on the channel-title row, and `Esc` walks back out as far as the server
rail, where it stops; the panel closes from the Close button, the bar widget, or the
Hyprland bind. Entering a server opens the channel it was last left on.

The tables below are generated from `Keymap.js`, the single key table the footer
hints and the `Ctrl+/` cheatsheet render from. Regenerate them whenever a key
changes:

```sh
node -e 'var s=require("fs").readFileSync("Keymap.js","utf8");eval(s);sections().forEach(function(x){if(!x.rows.length)return;console.log("**"+x.title+"**\n\n| Key | Action |\n|---|---|");x.rows.forEach(function(r){console.log("| `"+r.keys+"` | "+r.action+" |")});console.log("")})'
```

**Anywhere in the panel**

| Key | Action |
|---|---|
| `Ctrl+K · /` | Quick switcher: unread channels and DMs first, then everything (/ outside text inputs; from any app via the Hyprland bind) |
| `Ctrl+/ · ?` | This cheatsheet (? outside text inputs); Esc closes it |
| `Alt+h / Alt+l` | Move the focus zone: sidebar ↔ timeline ↔ composer (↔ member list while it is shown) |
| `m · Alt+m` | Show / hide the member list (m outside text inputs, Alt+m in the composer too) |
| `Alt+↑ / Alt+↓` | Previous / next channel in the list; add Shift to jump between unread channels only |
| `Tab / Shift+Tab` | Cycle rail → channels → timeline → composer (and its attachments) → member list → Members → Log out → Close |
| `r` | Reload state, the channel list and the open channel (outside text inputs) |
| `r` | While the backend is down: start it, or re-pull state |
| `Esc` | Walk back out: composer → timeline (marking read) → channel list → server rail, where it stops — Esc never closes the panel |
| `Esc` | From a panel button: back to the last zone |

**Sidebar — servers and channels**

| Key | Action |
|---|---|
| `j / k · ↑ / ↓` | Move the cursor (servers in the rail, channels in the list) |
| `g / G · Home / End` | First / last row |
| `Enter · l · →` | Rail: open the server's channel list and the channel it was last left on (a server also falls back to #general, then its first channel; Direct messages restore only what you left open) |
| `Enter` | Channel list: open the channel or thread (the composer takes focus); on a forum: show its threads |
| `Enter` | Channel list: on a voice channel, join it — on the one you are already in, focus the call bar |
| `t` | Channel list: show / hide the channel's active threads beneath it (from the timeline: the open channel's) |
| `h · ← · Esc` | Channel list: back to the server rail |
| `l · →` | Channel list: focus the open channel's timeline |

**Timeline — cursor on a message**

| Key | Action |
|---|---|
| `j / k · ↑ / ↓` | Move the message cursor (k past the top loads history) |
| `gg / G · Home / End` | Oldest loaded message (paging history) / newest message |
| `PgUp / PgDn` | Scroll a page (PgUp at the top loads history) |
| `R` | Reply to the message (the composer enters reply mode) |
| `E` | React: opens the emoji picker; the message's existing reactions come first and Enter on one toggles it |
| `D D` | Delete your own message (press D twice within 3 s) |
| `Y` | Copy the message text (plus attachment URLs) to the clipboard |
| `O` | Open the first link, attachment (from the cache when present) or embed |
| `L` | Copy the first link, attachment or embed URL to the clipboard (a link hovered with the mouse offers a Copy link chip) |
| `Ctrl+C` | Copy the highlighted text (drag the mouse over a message to highlight it); `Y` still copies the whole message |
| `Enter` | Reveal the message's spoiler images |
| `Esc` | Mark the channel read and go back to the channel list (cancels an armed delete, then clears a text selection, first) |

**Composer**

| Key | Action |
|---|---|
| `Enter` | Send (uploads the staged attachments when there are any) |
| `Shift+Enter` | Insert a newline |
| `↑` | In an empty input: edit your newest message |
| `Ctrl+V` | Paste: an image on the clipboard is staged as an attachment, text pastes normally |
| `Tab · ← / →` | Move from the input onto the staged attachments (← at the start / → at the end too) |
| `Esc` | Cancel edit or reply mode first, else mark the channel read and focus the timeline |
| `Enter` | Edit mode: save |
| `Esc` | Edit / reply mode: cancel (a stashed draft comes back) |
| `← / → · h / l` | On an attachment: move between the chips |
| `x · Delete` | On an attachment: remove it (and its staged file) |
| `Esc` | On an attachment: back to the text input |

**Member list**

| Key | Action |
|---|---|
| `j / k · ↑ / ↓` | Move the cursor over the members (g / G: first / last) |
| `Y` | Copy the member's @username to the clipboard |
| `Esc` | Back to the composer (Alt+h too) |

**Voice call**

| Key | Action |
|---|---|
| `Ctrl+Shift+M` | Mute / unmute your microphone (from any app via the Hyprland bind) |
| `Ctrl+Shift+D` | Deafen / undeafen: stop playing what the others say |
| `Ctrl+Shift+H` | Leave the voice channel |

**Quick switcher**

| Key | Action |
|---|---|
| `type` | Type to search channels and DMs (empty: unread first, then recent) |
| `↑ / ↓ · Tab · Ctrl+j / Ctrl+k · Ctrl+n / Ctrl+p` | Move the cursor (plain j/k type into the search) |
| `Enter` | Open the channel in the panel |
| `Esc` | Clear the search, then close |

**Emoji picker**

| Key | Action |
|---|---|
| `type` | Type to filter by name |
| `arrows · Ctrl+h/j/k/l` | Move across the grid: the message's reactions first, then frequently used, then everything (plain hjkl type into the filter) |
| `Enter` | React with the emoji; on one of the message's existing reactions: add or remove yours |
| `Esc` | Clear the filter, then close |

**Login screen**

| Key | Action |
|---|---|
| `Enter` | Activate the focused button / submit the token |
| `Tab` | Cycle Scan QR → token field → Log in → Close |
| `Esc` | Close the panel |
| `Esc` | QR view: cancel the running flow |
| `Esc` | QR view: dismiss a finished flow |
| `Enter` | QR view: start a new code |
| `Tab` | QR view: Tab reaches Close |

### Threads and forums

A text or announcement channel with active threads shows a muted "⌥ N threads" hint;
`t` on it (or on the open channel from the timeline) lists the threads beneath it,
newest activity first, with the same unread dot / mention badge as channels. `Enter`
on a thread opens it like any channel — the header reads "#parent › thread" — and
`t` again folds the list. A forum channel is not itself readable: `Enter` on it lists
its threads. Discord only tells a client about a server's threads once it has opened
a channel in that server, so the counts fill in after the first channel of a server
is opened.

### Member list

`m` (or `Alt+m` in the composer, or the Members button) shows a right-hand pane with
the channel's member list as Discord serves it: one header per group — hoisted roles,
then Online and Offline — with Discord's total count ("Online — 1,204"), and the first
hundred or so rows with text initials, name, status dot (online = accent, idle = muted,
do-not-disturb = urgent, offline = faded) and the activity line. DMs list their
recipients. The pane follows the open channel, updates live (member list and presence
events), and is a focus zone of its own while shown: `Alt+l` from the composer, `j`/`k`
to move, `Y` copies `@username`, `Esc` returns to the composer. "Loading members…"
shows until the first list arrives; if nothing comes within 15 s the pane says so.
Guild member lists are never fetched over REST (that gets user accounts flagged);
they come from the gateway exactly as the official client asks for them.

### Quick switcher

`Ctrl+K` or `/` in the panel, or the Hyprland bind above from anywhere. Type to
search (the backend ranks: unread and mentioned channels first, then by match, then
recency; an empty query shows unreads, then recents). `↑`/`↓`, `Tab`/`Shift+Tab`,
`Ctrl+j`/`Ctrl+k` or `Ctrl+n`/`Ctrl+p` move (plain `j`/`k` type into the search),
`Enter` opens the panel on the channel and focuses the composer, `Esc` clears the
search, then closes. Rows show the channel glyph, name, server, the last message,
and an unread dot or mention badge. While logged out the only row is "Log in to
Discord". The switcher counts as an open surface for the `stayConnected` idle rule.

### Reactions and emoji

`E` on the focused message opens the emoji picker. Type to filter by name; the
arrows (or `Ctrl+h/j/k/l` — plain letters type into the filter) move across the
grid; `Enter` reacts; `Esc` clears the filter, then closes. Sections, top to bottom:
**Toggle** (the message's existing reactions — picking one you already reacted with
removes yours), **Frequently used** (your last 16 distinct picks by count, stored on
the plugin's `shell.json` entry as `frequentEmoji`), one section per **server** with
custom emoji — the current server first, 48 per server until you type, 200 with a
filter — and the unicode catalogue (the shell's own emoji list). Custom emoji
render as `:name:` text and react as `name:id`; Discord may refuse another
server's emoji for accounts without Nitro. Clicking a reaction chip under a message
toggles it too. Reaction changes arrive back through `message_update`, so the chips
reflect Discord, not an optimistic guess. Typing `:shortcodes:` in the composer is
not supported.

### Reading, sending, drafts

Reaching the bottom of the timeline while the timeline or the composer is focused
marks the channel read (debounced); scrolling back up never acks. Sent messages show
immediately as a muted pending row and are re-keyed when the gateway echoes them; a
failed send removes the row and puts the text back into the composer with the error
in the footer. An upload clears the input the moment you press Enter (the chips show
its progress); a failed upload hands its text back too. Drafts and staged
attachments are kept per channel while the shell runs; staged images live in
`$XDG_RUNTIME_DIR/omarchy-discord/staged/` (0700) and are removed once uploaded or
when their chip is removed. Outgoing typing is sent at most once per 8 s per
channel. Middle-clicking the bar mark opens the most recent unread DM. The open
channel keeps a rolling window of the newest 500 messages while you are at the
bottom; nothing is trimmed while you are scrolled up.

### Voice

Voice channels show in the channel list with their occupants indented beneath them and
a speaking ring on whoever is talking. `Enter` on a voice channel joins it; a call bar
appears at the bottom of the channel column with the channel name, connection status,
and mute / deafen / leave. `Ctrl+Shift+M` mutes, `Ctrl+Shift+D` deafens, `Ctrl+Shift+H`
leaves — from any zone, the composer included. `Enter` on the channel you are already in
focuses the call bar. One call at a time; joining another leaves the first.

The same three actions are on the `quickshell.discord.voice` IPC target
(`mute` / `deafen` / `leave`), so you can drive the call from Hyprland without the panel
focused, the way the panel and switcher binds work:

```ini
bindd = CTRL SHIFT, M, Discord mute, exec, omarchy-shell quickshell.discord.voice mute
bindd = CTRL SHIFT, D, Discord deafen, exec, omarchy-shell quickshell.discord.voice deafen
bindd = CTRL SHIFT, H, Discord leave call, exec, omarchy-shell quickshell.discord.voice leave
```

**Audio (PipeWire).** The backend captures and plays through the default PipeWire source
and sink; it appears as a PipeWire node named `omarchy-discord`, so route it, set its
volume, or mute it in `wiremix` or `pavucontrol` like any other stream. The client does
no echo cancellation or noise suppression of its own — that is PipeWire's to do, and
yours to enable system-side. Drop either of these into
`~/.config/pipewire/pipewire.conf.d/` and restart PipeWire
(`systemctl --user restart pipewire`), then point the `omarchy-discord` capture at the
processed source in `wiremix`.

Echo cancellation with the WebRTC backend (also does noise suppression and AGC),
`echo-cancel.conf`:

```
context.modules = [
  { name = libpipewire-module-echo-cancel
    args = {
      aec.method = webrtc
      aec.args = {
        webrtc.gain_control       = true
        webrtc.noise_suppression  = true
      }
    }
  }
]
```

Or RNNoise as a filter-chain source (needs the `rnnoise-ladspa` plugin), `rnnoise.conf`:

```
context.modules = [
  { name = libpipewire-module-filter-chain
    args = {
      node.description = "Noise Cancelling Source"
      filter.graph = {
        nodes = [
          { type = ladspa  label = noise_suppressor_mono
            plugin = librnnoise_ladspa
            control = { "VAD Threshold (%)" = 50.0 } }
        ]
      }
      capture.props = { node.name = "rnnoise_capture" }
      source.props  = { node.name = "rnnoise_source"  media.class = Audio/Source }
    }
  }
]
```

## Notifications and media

New messages raise desktop notifications through the Omarchy notification center
(`notify-send`, app name "Omarchy Discord", normal urgency, the author's cached avatar as
the icon). Which ones is decided by the `notifications` setting on top of Discord's own
per-channel settings as the backend evaluates them (muted channels never notify):

| `notifications` | Notifies on |
|---|---|
| `All` | every message Discord would notify about, including "All messages" channels |
| `Mentions and DMs` (default) | messages that mention you (or `@everyone` where not suppressed) and DMs / group DMs |
| `Off` | nothing |

Suppressed regardless: your own messages, anything while your Discord status is Do Not
Disturb, and messages in the channel you are looking at (panel open, focused, on that
channel). Bursts are rate-limited to one notification per channel every 3 s; held
messages fold into one "(+N more)" notification when that window ends, even while
messages keep arriving. The summary is "Author in #channel"
("Author" for a DM), the body the first ~200 characters of the message as plain text,
plus a paperclip when it carries attachments.

Omacord's frontend does not admit remote media requests. Avatars and server icons
use text initials, custom emoji use text names, and attachments use explicit links.
The inherited backend retains its media cache support under
`$XDG_CACHE_HOME/omarchy-discord/media/`, but Omacord does not request or display
those files. `imagePreviews` is fixed to `Off`; `mediaCacheMB` remains a legacy
cache limit (default 512 MiB), not a RAM reservation.

## Settings

Stored inline on the plugin's `shell.json` entry; edit with
`omarchy bar set quickshell.discord <key> <value>`. Values are written as JSON
strings unless you pass `--json`, so the one numeric setting needs it:
`omarchy bar set quickshell.discord mediaCacheMB 1024 --json`. Changes apply live: the service re-reads its entry on every `shell.json`
change, values are normalized (unknown enum values fall back to the default,
`mediaCacheMB` is clamped to 64–4096).

| Key | Values | Default | Effect |
|---|---|---|---|
| `stayConnected` | `On` / `Off` | `On` | `On` keeps the backend unit running and the socket connected while the plugin is enabled (restarted within 5 s if it dies), so mentions and notifications keep arriving. `Off` stops the backend once no Discord surface (panel, quick switcher) has been open for 15 minutes (`Service.idleDisconnectMinutes`); the next open starts it again |
| `notifications` | `All` / `Mentions and DMs` / `Off` | `Mentions and DMs` | Desktop notification filter (see above) |
| `showMentionCount` | `On` / `Off` | `On` | Show the mention count next to the bar mark (`Off` keeps the dimmed-mark / unread-dot states) |
| `middleClick` | `Last unread DM` / `Raise panel` | `Last unread DM` | Middle-click action on the bar mark: open the panel on the most recent unread DM (the panel itself when there is none) / open or remap the panel |
| `window` | `On demand` / `Persistent` | `On demand` | `On demand` maps the client window when you open it and unmaps it on close. `Persistent` keeps it mapped from shell start so Hyprland rules can place it, and the bind focuses / hides it instead (see "Persistent window") |
| `imagePreviews` | `Off` | `Off` | Fixed in the text-only build; remote image requests and previews are disabled |
| `mediaCacheMB` | 64–4096 | 512 | Legacy backend disk-cache limit in MiB; Omacord does not request or display cached remote media |

The entry also carries `frequentEmoji`, a small JSON string the emoji picker maintains;
it is not a setting and survives `omarchy bar set` of the other keys.

## Themes

Every colour is an Omarchy shell theme token (`Color.*`, `Style.*`), so the client
follows `omarchy-theme-set` live, light themes included. Secondary text uses the
theme's `muted` colour when it is legible against the background and the foreground
at 60 % otherwise — several bundled themes define `muted` almost equal to their
background. The Phase 3 QA pass rendered every view on the five bundled light themes
(catppuccin-latte, flexoki-light, lupine, rose-pine, white) and two dark ones.

## Troubleshooting

- **"Backend not running" / the panel says the backend is stopped.** Press `r` in
  the panel, or `systemctl --user start omarchy-discord`. Logs:
  `journalctl --user -u omarchy-discord -f`. The unit is static by design
  (`systemctl --user status omarchy-discord` shows it as such); the plugin starts it.
- **Environment check.** `~/.local/lib/omarchy-discord/omarchy-discord-backend check`
  prints a JSON summary (socket and runtime paths and whether they are writable,
  media cache directory, `secret_tool`, `token_present`).
  `scripts/backend-runtime.sh check|status|start|stop|sync` is the shim the plugin
  itself uses; `sync` is the one that reinstalls the backend after a plugin update.
- **Login required after a restart.** The token is stored with `secret-tool`; if the
  keyring is locked or unavailable the panel says so and the session will not survive
  a backend restart. Unlock the keyring and log in again.
- **Socket path.** `$XDG_RUNTIME_DIR/omarchy-discord/backend.sock` (0600). The Quickshell
  client leaves the path empty and reports it when `XDG_RUNTIME_DIR` is unset.
- **A DM refuses to open ("send a message from the official client first").** Opening
  a DM with no history from a third-party client is a spam-flag risk; the backend
  refuses it on purpose.
- **Hot reload.** Any write inside `~/.config/omarchy/plugins/` reloads the whole
  plugin system. The backend survives that (it is a separate unit), the panel is
  recreated; never point builds, logs or caches into the plugin tree.
- **The backend looks stale after an update.** The service reinstalls it on load, so
  a shell restart (or any edit inside the plugin directory) re-runs the check;
  `scripts/setup.sh --reinstall-backend` from the plugin directory forces it, followed
  by `systemctl --user try-restart omarchy-discord.service`.
- **Reset everything.** `scripts/remove-runtime.sh --purge`, then reinstall.

## Non-goals

Video, screen share and stage channels, server management and moderation
tooling, Nitro store surfaces, password login (token / QR only, so captchas never
enter the picture), multiple simultaneous accounts, and `:shortcode:` emoji typing.
Search, drag-and-drop uploads and a presence-rich member pane beyond the first
hundred rows are deferred (PLAN.md Phase 4).

## Credits

- Matt Calayo — original Omarchy Discord author; Omacord forks
  [zgt/omarchy-discord](https://github.com/zgt/omarchy-discord). The original
  copyright notice and MIT license are preserved in `LICENSE`.
- [diamondburned/arikawa](https://github.com/diamondburned/arikawa) and
  [diamondburned/ningen](https://github.com/diamondburned/ningen) — the Go Discord
  library stack the backend is built on (gateway, REST, read state, member lists).
- [diamondburned/dissent](https://github.com/diamondburned/dissent) — the GTK client
  on the same stack; the reference for session, member-list and notification
  semantics.
- [ayn2op/discordo](https://github.com/ayn2op/discordo) — the TUI client whose QR
  remote-auth flow the backend ports.
- [thisisgm/omarchy-discord](https://github.com/thisisgm/omarchy-discord) — the bar
  companion for the official app; the Discord mark geometry in `DiscordIcon.qml` is
  adapted from it (MIT).
- [quickshell.spotify](https://github.com/stappmus/Omarchy-Spotify) — the plugin
  architecture this one copies wholesale.

## License

MIT.
