# Omarchy Discord

A text-only Discord client shipped as an Omarchy Quattro shell plugin
(`quickshell.discord`): a themed Quickshell panel and bar widget in front of a small
Go backend, instead of the 1 GB Electron app. Same architecture as
quickshell.spotify: QML owns everything visible, a systemd user unit owns the
Discord connection, and a private JSON-lines socket joins them.

What you get: the bar mark with a mention badge and unread dot; a panel with the
server rail, channel list (threads and forums included), a virtualized timeline with
history paging, markdown, images, embeds, reactions and spoilers; a composer with
send / reply / edit / delete, typing both ways and screenshot paste; a member list
with presence; the `Ctrl+K` quick switcher that works from any app; desktop
notifications through the Omarchy notification center; QR login; and every colour
from the active Omarchy theme, light themes included. It is keyboard-first
throughout — `Ctrl+/` shows the cheatsheet.

**Status: Phase 3 complete** (see `docs/PLAN.md` for the roadmap and what is
deliberately deferred). Voice, video, server management and multiple accounts are
non-goals; voice and stage channels are hidden entirely.

## Screenshots

_Coming with the first release: panel with an open channel, the member list, the
quick switcher, and the same views on a light theme._

## Install

```sh
omarchy plugin add https://github.com/mattcalayo/omarchy-discord --enable
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
`wl-paste` for image paste, `xdg-open`.

### Development install

```sh
git clone https://github.com/mattcalayo/omarchy-discord
cd omarchy-discord
scripts/install-local.sh          # --section left|center|right
```

`install-local.sh` validates the manifest, installs the backend and unit, rsyncs the
checkout into `~/.config/omarchy/plugins/quickshell.discord/` (a copy, not a symlink,
because `omarchy plugin validate` refuses symlinks and in-tree edits would hot-reload
the shell), rescans, and enables the widget. **Re-run it after every change** — the
plugin runs from that copy, not from your checkout. The backend is reinstalled
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

The bind focuses the window when it is unfocused and hides the special workspace it
sits on when it is focused, so `SUPER SHIFT, D` and `togglespecialworkspace scratchpad`
are interchangeable. Without the rule (on a normal workspace) closing just unmaps the
window instead. `SUPER+W` closes the window too; the next press of the bind maps and
focuses it again. Read acks, refresh and the member subscription follow window focus,
not map/unmap, so a visible but unfocused window does not mark channels read.

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
hundred or so rows with avatar, name, status dot (online = accent, idle = muted,
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
render through the media cache and react as `name:id`; Discord may refuse another
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

Images (avatars, guild icons, attachments, embed images, custom emoji) are downloaded by
the backend into `$XDG_CACHE_HOME/omarchy-discord/media/` (Discord CDN hosts only) and
rendered from there. `imagePreviews` `Off` turns attachment and embed images back into
filename chips (avatars and emoji stay). Spoiler images stay covered until you press
`Enter` on the message or click them. `mediaCacheMB` caps the cache (LRU, default 512).

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
| `imagePreviews` | `On` / `Off` | `On` | Inline image attachments and embed images in the timeline; `Off` shows filename chips instead (avatars and emoji stay) |
| `mediaCacheMB` | 64–4096 | 512 | Media cache size cap in MiB, pushed to the backend with `set_config` on connect and on change |

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

Voice and video (call state is not shown either), server management and moderation
tooling, Nitro store surfaces, password login (token / QR only, so captchas never
enter the picture), multiple simultaneous accounts, and `:shortcode:` emoji typing.
Search, drag-and-drop uploads and a presence-rich member pane beyond the first
hundred rows are deferred (PLAN.md Phase 4).

## Credits

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
