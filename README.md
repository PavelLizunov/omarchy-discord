# Omarchy Discord

A text-only Discord client shipped as an Omarchy Quattro shell plugin
(`quickshell.discord`): a themed Quickshell panel and bar widget in front of a small
Go backend, instead of the 1 GB Electron app. Same architecture as
quickshell.spotify: QML owns everything visible, a systemd user unit owns the
Discord connection, and a private JSON-lines socket joins them.

**Status: Phase 1, read-only client.** Bar mark with mention badge and unread dot,
token login, guild rail + channel list, a virtualized message timeline with history
paging and markdown rendering, live read state (ack on read), and a typing line.
No composer yet (Phase 2); threads and forums are listed but open in Phase 3; voice
and stage channels are hidden entirely (voice is a non-goal). See `docs/PLAN.md` for
the roadmap and `docs/CONVENTIONS.md` for the mechanics contract.

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

Click the bar mark (or use the bind) and paste a user token into the form; Enter
submits. The token goes over the socket to the backend, which stores it in the GNOME
keyring via `secret-tool` over stdin. It is never written to disk, logs, or shown in
the UI. QR login arrives in a later phase.

From a terminal instead:

```sh
printf '%s' "$TOKEN" | ~/.local/lib/omarchy-discord/omarchy-discord-backend login
```

`login` reads the token from stdin. `omarchy-discord-backend check` prints an
environment summary.

## Keyboard (Phase 1 panel)

Two focus zones: the **sidebar** (guild rail + channel list, `h`/`l` or arrows move
between the two columns) and the **timeline**. Focus is always visible; the active
column or timeline carries the focus border.

| Key | Action |
|---|---|
| `Alt+h` / `Alt+l` | Move focus zone: sidebar ↔ timeline |
| `j` / `k`, arrows | Move the cursor in the focused column / timeline |
| `Enter` (rail) | Select the server and focus its channel list |
| `Enter` (channel list) | Open the channel and focus the timeline |
| `h` / `l`, Left / Right | Rail ↔ channel list ↔ timeline |
| `Alt+↑` / `Alt+↓` | Previous / next channel in the current list |
| `Alt+Shift+↑` / `Alt+Shift+↓` | Previous / next **unread** channel |
| `gg` / `G`, Home / End, `PgUp` / `PgDn` | Timeline top (pages history) / newest / page |
| `Y` / `O` | Copy the focused message's text / open its first link or attachment |
| `r` | Reload (structure, channel list, open channel; starts the backend if stopped) |
| `Esc` (timeline) | Mark the channel read and return to the sidebar |
| `Esc` (channel list) | Back to the rail |
| `Esc` (rail) | Close the panel |
| `Tab` / `Shift+Tab` | Cycle rail → channel list → timeline → Log out → Close; `Esc` on a button returns to the last zone |

Reaching the bottom of the timeline while it is focused marks the channel read
(debounced); scrolling back up never acks. Middle-clicking the bar mark opens the
most recent unread DM. The open channel keeps a rolling window of the newest 500
messages while you are at the bottom (older rows become pageable history again);
nothing is trimmed while you are scrolled up.

## Settings

Stored inline on the plugin's `shell.json` entry; edit with
`omarchy bar set quickshell.discord <key> <value>`: `stayConnected`,
`notifications`, `showMentionCount`, `middleClick`, `imagePreviews`, `mediaCacheMB`.

## License

MIT. The Discord mark geometry in `DiscordIcon.qml` is adapted from
[thisisgm/omarchy-discord](https://github.com/thisisgm/omarchy-discord) (MIT).
