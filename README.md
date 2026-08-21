# Omarchy Discord

A text-only Discord client shipped as an Omarchy Quattro shell plugin
(`quickshell.discord`): a themed Quickshell panel and bar widget in front of a small
Go backend, instead of the 1 GB Electron app. Same architecture as
quickshell.spotify: QML owns everything visible, a systemd user unit owns the
Discord connection, and a private JSON-lines socket joins them.

**Status: Phase 2 complete (notifications, media, QR login).** Everything from Phase 1
(bar mark with mention badge and unread dot, guild rail + channel list, virtualized
timeline with history paging and markdown rendering, live read state, typing line)
plus the composer (send, reply, edit, delete, outgoing typing, image paste: `Ctrl+V`
stages a clipboard image as an attachment chip, `Enter` uploads it), desktop
notifications through the Omarchy notification center, media through the backend's
cache (avatars, guild icons, inline image attachments with spoiler covers, embed
thumbnails/images, custom emoji in messages and reaction chips), and QR login. Reactions,
the quick switcher, and the emoji picker are Phase 3; threads and forums are listed but
open in Phase 3; voice and stage channels are hidden entirely (voice is a non-goal). See
`docs/PLAN.md` for the roadmap and `docs/CONVENTIONS.md` for the mechanics contract.

## Install (development)

```sh
git clone https://github.com/mattcalayo/omarchy-discord
cd omarchy-discord
scripts/install-local.sh          # --section left|center|right
```

`install-local.sh` validates the manifest, installs the backend to
`~/.local/lib/omarchy-discord/` and its static user unit to
`~/.config/systemd/user/`, copies the checkout into
`~/.config/omarchy/plugins/quickshell.discord/` (a copy, not a symlink, because
`omarchy plugin validate` refuses symlinks), rescans, and enables the widget.
Re-run it after every change; the backend is rebuilt whenever a file under
`backend/` is newer than the installed binary (`scripts/setup.sh
--reinstall-backend` forces it). A bundled binary under `backend/dist/$(uname -m)/`
is used when present; otherwise the script builds with Go, outside the plugin tree.
Setting `OMARCHY_DISCORD_RUNTIME_DIR` relocates the binary; setup rewrites the
installed unit's `ExecStart` to match.

The backend unit is never enabled at login. The enabled plugin starts it and keeps
it connected while the `stayConnected` setting is On (the default).

Remove the runtime with `scripts/remove-runtime.sh` (`--purge` also clears the
keyring entry and cache), then `omarchy plugin remove quickshell.discord --yes`.

## Hyprland bind

```ini
bindd = SUPER SHIFT, D, Discord, exec, omarchy-shell quickshell.discord.panel toggle
```

`open` and `close` are also available on that target. The zero-config fallback is
`omarchy-shell shell toggle quickshell.discord`.

## Logging in

Click the bar mark (or use the bind). The login screen offers two ways in; `Tab` cycles
Scan QR → token field → Log in → Close, `Enter` activates, `Esc` closes.

**Scan QR** (default): the panel shows a QR code; open the Discord mobile app, go to
Settings › Scan QR Code, scan it, and confirm on the phone. The panel follows along
("Logging in as … — confirm on your phone", then "Approved, connecting…"). The code is
valid for about two minutes; when it expires, is declined, or you press `Esc`, a
[Try again] button produces a fresh one. No password or captcha is involved; the token
the phone hands over goes straight into the keyring.

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

Three focus zones: the **sidebar** (guild rail + channel list, `h`/`l` or arrows move
between the two columns), the **timeline**, and the **composer**. Focus is always
visible; the active column, timeline, or composer carries the focus border. Opening a
channel focuses the composer.

| Key | Action |
|---|---|
| `Alt+h` / `Alt+l` | Move focus zone: sidebar ↔ timeline ↔ composer |
| `j` / `k`, arrows | Move the cursor in the focused column / timeline |
| `Enter` (rail) | Select the server and focus its channel list |
| `Enter` (channel list) | Open the channel and focus the composer |
| `Enter` (timeline) | Reveal the focused message's spoiler images (click works too) |
| `h` / `l`, Left / Right | Rail ↔ channel list ↔ timeline |
| `Alt+↑` / `Alt+↓` | Previous / next channel in the current list (works from the composer too) |
| `Alt+Shift+↑` / `Alt+Shift+↓` | Previous / next **unread** channel |
| `gg` / `G`, Home / End, `PgUp` / `PgDn` | Timeline top (pages history) / newest / page |
| `R` (timeline) | Reply to the focused message: reply line appears in the composer, `Esc` cancels it |
| `D` `D` (timeline) | Delete the focused message if it is yours: the first `D` arms it for 3 s ("D again to delete"), the second deletes; `Esc` or moving disarms |
| `E` (timeline) | Reactions: not yet, shows a hint (Phase 3) |
| `Y` / `O` | Copy the focused message's text / open its first link or attachment (attachments open from the local media cache when already downloaded) |
| `Enter` (composer) | Send; with staged attachments, upload them with the text |
| `Shift+Enter` (composer) | Newline |
| `↑` (empty composer) | Edit your last message in the loaded window; `Enter` saves, `Esc` cancels |
| `Ctrl+V` (composer) | Paste: an image on the clipboard becomes an attachment chip (screenshot → `Ctrl+V` → `Enter`); text pastes normally |
| `Tab` / `Shift+Tab`, Left / Right (composer) | Move the cursor onto the attachment chips; `x` / Delete removes the focused chip; `Esc` returns to the input |
| `r` (sidebar) | Reload (structure, channel list, open channel; starts the backend if stopped) |
| `Esc` (composer) | Cancel reply/edit mode, else mark the channel read and focus the timeline |
| `Esc` (timeline) | Mark the channel read and return to the sidebar |
| `Esc` (channel list) | Back to the rail |
| `Esc` (rail) | Close the panel |
| `Tab` / `Shift+Tab` | Cycle rail → channel list → timeline → composer (→ its chips) → Log out → Close; `Esc` on a button returns to the last zone |

Reaching the bottom of the timeline while the timeline or the composer is focused
marks the channel read (debounced); scrolling back up never acks. Sent messages show
immediately as a muted pending row and are re-keyed when the gateway echoes them; a
failed send removes the row and puts the text back into the composer with the error
in the footer. Drafts and staged attachments are kept per channel while the shell
runs. Staged images live in `$XDG_RUNTIME_DIR/omarchy-discord/staged/` (0700) and are
removed once uploaded or when their chip is removed. Outgoing typing is sent at most
once per 8 s per channel. Middle-clicking the bar mark opens the most recent unread DM.
The open channel keeps a rolling window of the newest 500 messages while you are at
the bottom (older rows become pageable history again); nothing is trimmed while you
are scrolled up.

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
messages fold into the next one as "(+N more)". The summary is "Author in #channel"
("Author" for a DM), the body the first ~200 characters of the message as plain text,
plus a paperclip when it carries attachments.

Images (avatars, guild icons, attachments, embed images, custom emoji) are downloaded by
the backend into `$XDG_CACHE_HOME/omarchy-discord/media/` (Discord CDN hosts only) and
rendered from there. `imagePreviews` `Off` turns attachment and embed images back into
filename chips (avatars and emoji stay). Spoiler images stay covered until you press
`Enter` on the message or click them. `mediaCacheMB` caps the cache (LRU, default 512).

## Settings

Stored inline on the plugin's `shell.json` entry; edit with
`omarchy bar set quickshell.discord <key> <value>`.

| Key | Values | Default | Effect |
|---|---|---|---|
| `stayConnected` | `On` / `Off` | `On` | Keep the backend connected while the plugin is enabled; `Off` idles it out 15 min after the last open surface |
| `notifications` | `All` / `Mentions and DMs` / `Off` | `Mentions and DMs` | Desktop notification filter (see above) |
| `showMentionCount` | `On` / `Off` | `On` | Show the mention count next to the bar mark |
| `middleClick` | `Last unread DM` / `Raise panel` | `Last unread DM` | Middle-click action on the bar mark |
| `imagePreviews` | `On` / `Off` | `On` | Inline image attachments and embed images in the timeline |
| `mediaCacheMB` | 64–4096 | 512 | Media cache size cap in MiB (pushed to the backend with `set_config`) |

## License

MIT. The Discord mark geometry in `DiscordIcon.qml` is adapted from
[thisisgm/omarchy-discord](https://github.com/thisisgm/omarchy-discord) (MIT).
