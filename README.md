# Omarchy Discord

A text-only Discord client shipped as an Omarchy Quattro shell plugin
(`quickshell.discord`): a themed Quickshell panel and bar widget in front of a small
Go backend, instead of the 1 GB Electron app. Same architecture as
quickshell.spotify: QML owns everything visible, a systemd user unit owns the
Discord connection, and a private JSON-lines socket joins them.

**Status: Phase 0, skeleton that connects.** Bar mark with mention badge, token
login, and a bare guild/channel browser. No message timeline yet. See
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
Re-run it after every change. A bundled binary under `backend/dist/$(uname -m)/`
is used when present; otherwise the script builds with Go, outside the plugin tree.

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

## Keyboard (Phase 0 panel)

| Key | Action |
|---|---|
| `j` / `k`, arrows | Move the cursor in the focused list |
| `Enter` / `l` / Right | Load and focus the selected server's channels |
| `h` / Left / `Esc` | Back to the server list |
| `g` / `G`, Home / End | First / last row |
| `r` | Refresh |
| `Esc` (server list) | Close the panel |

## Settings

Stored inline on the plugin's `shell.json` entry; edit with
`omarchy bar set quickshell.discord <key> <value>`: `stayConnected`,
`notifications`, `showMentionCount`, `middleClick`, `imagePreviews`, `mediaCacheMB`.

## License

MIT. The Discord mark geometry in `DiscordIcon.qml` is adapted from
[thisisgm/omarchy-discord](https://github.com/thisisgm/omarchy-discord) (MIT).
