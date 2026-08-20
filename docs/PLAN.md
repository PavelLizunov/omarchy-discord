# Omarchy Discord — Implementation Plan

> Transcribed from the plan artifact (2026-08-20). This document is the authority on
> scope and design. Modeled on quickshell.spotify v1.0.2 · target: Omarchy Quattro (4.x).

A lightweight Discord client as an Omarchy Quattro shell plugin — the **Omarchy Spotify**
treatment applied to Discord: a themed Quickshell panel and bar widget in front of a small
native backend, instead of a 1 GB Electron app.

- **Verdict:** no client plugin exists — build it
- **Frontend:** QML / Quickshell
- **Backend:** Go · arikawa + ningen
- **Model:** quickshell.spotify architecture

## Survey — what exists today

Nothing in the plugin ecosystem is a Discord *client*. The closest match,
[thisisgm/omarchy-discord](https://github.com/thisisgm/omarchy-discord), is a companion
widget for the official desktop app — genuinely well built (attention badge, call state
via PipeWire, memory readout), but it still requires the Electron client to be running.
It is complementary to this project, not a substitute for it.

The good news: the hard protocol work is already done in the open. Discordo — the TUI
client — and Dissent both sit on the same Go library stack, which is mature, maintained,
and designed specifically for building user-account clients.

| Project | What it is | Relevance |
|---|---|---|
| thisisgm/omarchy-discord | Omarchy bar widget wrapping the official desktop app | Not a client. Reference for bar-widget conventions and panel keyboard handling. |
| diamondburned/arikawa | Go Discord API + gateway library | Backend foundation: REST, gateway, state. |
| diamondburned/ningen | Client-behavior layer over arikawa | The key dependency: read state, mention counts, guild subscriptions, member list — the semantics official clients have. |
| ayn2op/discordo | Go TUI client | Proof the stack works; reference for login and message flows. |
| diamondburned/dissent | GTK4 native client on ningen | The closest architectural sibling: a full GUI client on this exact stack. Primary reference implementation. |

## Scope — goal and non-goals

**Goal**
- Read and send messages in guilds, threads, and DMs
- Accurate unread / mention badges in the bar
- Native notifications through the Omarchy shell
- Full Omarchy theme integration, light themes included
- Keyboard-first, mirroring the Spotify plugin's shortcut culture
- Tens of MB of RAM, not ~1 GB

**Non-goals (v1)**
- **Voice and video.** Out of scope. Show call state in the sidebar at most; join calls elsewhere.
- Server management, moderation tooling, Nitro store surfaces
- Multiple simultaneous accounts
- Password login — token/QR only, so captchas never enter the picture

## Architecture — same shape as Omarchy Spotify

The Spotify plugin's split is exactly right for this: QML owns everything visible, a
small native daemon owns the protocol and the persistent connection, and a private
line-delimited JSON socket joins them. Copy the shape wholesale.

```
BarWidget.qml (badge · presence dot)   Panel.qml (full client · lazy-loaded)   QuickSwitch.qml (Ctrl+K jump — the "mini player")
                        ▲ bind to shared state ▲
Service.qml — socket client · state store · reconnect · notification fan-out · runs while plugin enabled
                        ▲ $XDG_RUNTIME_DIR/omarchy-discord/backend.sock — JSON lines, protocol v1 ▲
omarchy-discord-backend (Go) — arikawa v3 gateway + REST · ningen read-state/mentions ·
media cache · systemd user unit, static, never enabled at login
                        ▲ wss gateway + REST, official-client identify ▲
Discord — user account session
```

Two deliberate differences from Spotify. First, **Go instead of Rust**: librespot made
Rust the obvious choice there; here every mature user-account library is Go, and a
static Go binary ships in `backend/dist/x86_64/` just as easily. Second, **the backend
wants to stay up**: Spotify's daemon idles out after playback, but a chat client that
disconnects stops getting mentions. Default to connected-while-enabled, with a settings
toggle and an idle-disconnect option for battery-minded users.

One hard-won Spotify lesson to keep: Omarchy hot-reloads a plugin on any write inside
its directory, so the backend must build to `$XDG_CACHE_HOME/omarchy-discord/target`,
never into the plugin tree.

## Backend — socket protocol v1

Same framing as the Spotify backend: UTF-8 JSON, one object per line, caller-chosen
integer ids, versioned envelope, stable machine-readable error codes, full snapshot on
connect. The backend is the single source of truth; QML holds a mirror, never
authoritative state.

```json
{"v":1,"id":12,"command":"send","channel_id":"…","content":"on my way","reply_to":"…"}
{"type":"response","v":1,"id":12,"ok":true,"result":{"message_id":"…"}}
{"type":"event","v":1,"event":"message_create","channel_id":"…","message":{}}
```

| Group | Commands | Events pushed |
|---|---|---|
| Session | hello · ping · get_state · login(token) · logout · set_presence | state_changed (connecting / ready / reauth_needed) |
| Structure | list_guilds · list_channels · list_dms · quick_switch(query) | guilds_synced · channel_update |
| Messages | open_channel · close_channel · history(before_id) · send · edit · delete · react · unreact · typing | message_create / update / delete · typing_start |
| Read state | ack(channel, message) | read_state_changed — per-channel unread flag + mention count; the bar badge is a pure reduction of this |
| Media | fetch_media(url) → cached file path · upload(channel, path) | media_ready · upload_progress |

`open_channel` is the load-bearing command: it subscribes the gateway (ningen's guild
subscriptions), returns the last ~50 messages, and starts streaming. QML's `ListView`
virtualizes the timeline, so the frontend never holds more than the visible window plus
history pages it explicitly asked for. Media downloads land in
`$XDG_CACHE_HOME/omarchy-discord/media/` with an LRU size cap; QML renders them by file
path.

## Auth — token in the keyring, nothing else

- **Primary flow — QR remote auth.** The backend opens Discord's remote-auth gateway,
  the panel shows the QR, you scan with the phone app, the token arrives. No password,
  no captcha. (Discordo ships this; port the flow.)
- **Fallback — paste a token** from browser devtools, same as every TUI client.
- Token goes to GNOME Keyring via `secret-tool` over stdin, mirroring the Spotify
  plugin's `keyring-store.sh`. It never touches disk, logs, or the QML layer; error
  strings are redacted before they can reach the UI.
- REST calls restricted to `https://discord.com/api/v9`; media fetches to Discord's CDN
  hosts only.

## Frontend

The Spotify plugin's UI grammar maps almost one-to-one. Familiar layout from the
official client, drawn in the active Omarchy theme, driven by the keyboard:

```
┌─┬───────────────┬──────────────────────────────────────┬─────────┐
│ │  Search  ⌃K   │  #channel-name · topic               │ members │
│g├───────────────┼──────────────────────────────────────┤ (toggle)│
│u│ ▾ SERVER      │  chat timeline                       │         │
│i│   # general   │  grouped by author · replies inline  │         │
│l│   # dev   ③   │  images/embeds lazy via media cache  │         │
│d│ ▾ DMs         │  day dividers · unread marker line   │         │
│s│   @ ada    ●  ├──────────────────────────────────────┤         │
│ │   @ lin       │  composer · typing: "ada is typing…" │         │
└─┴───────────────┴──────────────────────────────────────┴─────────┘
```

### Bar widget
- Vector mark drawn in theme foreground (the thisisgm plugin proves the approach);
  dims when disconnected.
- Mention count in the theme's urgent color — this is the number that matters. Plain
  unreads are a subtle dot, optional.
- Left click: panel. Middle click: most recent unread DM. Configurable, like Spotify's
  `showMiniPlayer`.

### The "mini player" equivalent

Spotify's mini player earns its keep by answering the common case without the full app.
Discord's common case is "who pinged me?" — so the compact surface is a **quick
switcher**: `Ctrl+K` style fuzzy list of unread channels and DMs first, then everything,
with inline preview of the last message. Enter opens the panel on that channel. Small,
fast, keyboard-only.

### Keyboard navigation — a requirement, not a nicety

The whole client operates without a mouse, holding the same contract as the Spotify
mini-player: every interactive element reachable with `Tab` or the arrows, `Enter`
activates, `Esc` walks back out, and focus is always visible. Three focus zones —
**sidebar**, **timeline**, **composer** — with a roving cursor inside each.

| Key | Action |
|---|---|
| `Ctrl+K` · `/` | Quick switcher from anywhere |
| `Alt+h` / `Alt+l` | Move focus zone: sidebar ↔ timeline ↔ composer |
| `j` / `k` · `↑↓` | Move the cursor: channels in the sidebar, messages in the timeline |
| `Alt+↑` / `Alt+↓` | Previous / next channel; add Shift to jump unreads only |
| `gg` / `G` · `PgUp` | Timeline top (paging history) / newest message |
| `R` · `E` · `Y` · `O` | On the focused message: reply · react · copy text · open link or attachment |
| `↑` in composer | Edit your last message |
| `Ctrl+V` | Paste — stages an image attachment when the clipboard holds one |
| `Esc` | Composer → timeline (marking read) → close panel |
| `Ctrl+/` | Full cheatsheet overlay |

Opening a channel focuses the composer; opening the panel from a Hyprland bind takes
keyboard focus immediately, exactly like the Spotify mini-player. Focus behavior is a
Phase 1 acceptance criterion, not polish — each view lands with its keyboard story or it
doesn't land.

### Paste images into the composer

- `Ctrl+V` in the composer checks the Wayland clipboard first: `wl-paste --list-types`;
  on `image/*`, the image is written to `$XDG_RUNTIME_DIR/omarchy-discord/staged/` and
  appears as a thumbnail chip above the composer. Text pastes fall through to the normal
  path.
- Multiple staged attachments queue as chips; `x` (or Del on a focused chip) removes
  one. `Enter` sends text plus uploads through the backend's `upload` command, with
  `upload_progress` events driving a progress bar on the chip.
- This makes Omarchy's screenshot-to-clipboard flow a two-keystroke share: screenshot,
  `Ctrl+V`, `Enter`.
- Drag-and-drop from a file manager rides the same staging path later; paste ships
  first.

### Theme and text rendering
- Every color from Omarchy shell theme tokens, exactly as the Spotify plugin does it —
  light themes are first-class, which no Discord client anywhere offers natively themed.
- Markdown subset (bold/italic/strike, code, spoilers, mentions, links) rendered to Qt
  rich text in a QML util; fenced code blocks in the theme's mono font.
- Custom emoji are just CDN images through the media cache; unicode emoji use the
  system color font.

### Notifications

Fire through the shell's notification service from `Service.qml`, so they render in the
themed notification center and respect its do-not-disturb. ningen's read state already
encodes Discord-side mute and suppression settings — honor them, and add one
plugin-level setting: notify on *All / Mentions and DMs / Off*.

## Manifest — plugin surface

```json
{
  "id": "quickshell.discord",
  "kinds": ["service", "bar-widget", "panel"],
  "activation": "on-demand",
  "entryPoints": { "service": "Service.qml",
                   "barWidget": "BarWidget.qml",
                   "panel": "Panel.qml" },
  "barWidget": { "category": "Communication",
                 "aliases": ["discord", "chat"], "schema": [
    "stayConnected        On | Off (idle-disconnect minutes)",
    "notifications        All | Mentions and DMs | Off",
    "showMentionCount     On | Off",
    "middleClick          Last unread DM | Raise panel",
    "imagePreviews        On | Off",
    "mediaCacheMB         512" ] }
}
```

Install path mirrors Spotify: `omarchy plugin add <repo> --enable`, no privileged hooks;
the service installs the bundled backend binary and user unit on first load. A Hyprland
bind (`SUPER+SHIFT+D`?) via `omarchy shell -q quickshell.discord.panel toggle` goes in
the README, not the installer.

## Phases

**0 — Skeleton that connects** (~a weekend): Repo, manifest, stub bar widget. Go
backend: socket server, protocol envelope, keyring token, gateway connect via ningen,
`ready` snapshot into QML. Exit test: your guild list prints in the panel.

**1 — Read-only client** (1–2 weeks): Channel/DM sidebar, virtualized timeline with
author grouping and history paging, markdown rendering, read-state → live mention badge
in the bar. This milestone already replaces "keep Discord open to see if anyone needs
me."

**2 — Participation** (1–2 weeks): Composer: send, reply, edit, delete, typing both
directions, ack-on-read. Notifications through the shell. Image/embed rendering via the
media cache, and the clipboard→staged-attachment→upload pipeline for pasting images. QR
login flow so nobody ever touches devtools.

**3 — Feel** (ongoing): Quick switcher, reactions + emoji picker (reuse the shell's),
uploads, threads, member list, settings schema, full shortcut cheatsheet, light-theme QA
pass across bundled Omarchy themes. Publish to omarchyplugins.com.

**4 — Stretch, explicitly deferred**: Presence-rich member list, search, forum
channels, drag-and-drop uploads, multiple accounts.

## Risks

| Risk | Odds | Handling |
|---|---|---|
| Undocumented API shifts | Certain, eventually | That's why the protocol layer is a shared community library and not our code. Track arikawa/ningen releases. |
| Timeline rendering cost in QML | Medium | Virtualized ListView, pre-parsed rich text from a worker, media strictly lazy. Budget: idle panel < 5% CPU. |
| Backend memory creep | Low | ningen's caches are bounded; target < 80 MB RSS backend + shell delta. |
| Scope creep toward "all of Discord" | High | The non-goals list is load-bearing. Ship Phase 1 before touching Phase 3 ideas. |

## Repository skeleton

```
omarchy-discord/                    # plugin id: quickshell.discord
├── manifest.json
├── Service.qml                     # socket client, state, notifications
├── BarWidget.qml
├── Panel.qml
├── QuickSwitch.qml
├── components/                     # MessageRow, Timeline, Composer,
│                                   # ChannelList, GuildRail, Markdown.js
├── backend/
│   ├── cmd/omarchy-discord-backend/main.go
│   ├── internal/{socket,protocol,session,readstate,media}/
│   └── dist/x86_64/omarchy-discord-backend   # static, shipped in releases
├── systemd/omarchy-discord.service # static user unit, never enabled at login
├── scripts/{build-backend.sh, install-local.sh, keyring-store.sh}
└── docs/{BACKEND_PROTOCOL.md, TECHNICAL.md}
```
