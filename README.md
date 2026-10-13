# Omacord

A text-focused Discord client for Omarchy Quattro, with a native panel, channel switcher, mention indicator, voice controls and an experimental camera and screen viewer. Plugin ID: `quickshell.discord`.

The plugin runs with your user permissions inside the shared Omarchy shell. The backend connects to Discord, stores a sign-in token through Secret Service, and uses a user systemd service. This is an unofficial client; use it at your own risk.

## Video prerelease: 0.2.0-beta.1

GitHub tag `v0.2.0-beta.1` freezes the reviewed experimental video checkpoint. Download the complete plugin archive, rather than only the backend binary, and verify it against the release checksums. See [CHANGELOG.md](CHANGELOG.md) for features and known limitations.

Screen video and audio have bounded live evidence, while live webcam delivery and the latest different-sender switching repair remain unverified. Video presentation is limited to 5 JPEG updates per second. Quality is an output cap and server hint, not a promise of remote source detail. This is a prerelease for testing, not stable-release certification.

The earlier immutable `v0.1.0` beta remains available and does not contain video viewing. The forthcoming icon redesign is separate from this release.

## Requirements and installation

Requires Omarchy Quattro, Quickshell, Qt Quick Controls, systemd, `secret-tool`, `libopus`, `rsync` and `jq`. Experimental video viewing additionally requires `ffmpeg`. Clipboard image staging uses `wl-paste`. The bundled backend is for x86_64; other architectures or changed backend sources require Go and build dependencies. Source builds may download Go modules. Never start a second Quickshell instance to run the plugin.

From this checkout:

```sh
bash scripts/install-local.sh --section right
```

This validates the plugin, installs the backend and user unit, copies source into `~/.config/omarchy/plugins/quickshell.discord`, rescans plugins and enables the widget if needed. It can restart an existing backend. The installer mirrors the destination with deletion, so back up local installed-copy changes before using it. The unit is not enabled at login by setup; the plugin manages its lifecycle.

For updates, rerun the installer after preserving drafts and local changes. A shell reload may be necessary when cached QML remains loaded; preserve unsent work in other plugins before a shell restart.

## Use

Left-click the bar icon for the compact, bar-anchored panel. During a call it opens your current voice room with participants, microphone, listening and leave controls. Click a participant once for local mute and playback volume. Browse conversations switches to the server and DM list; Escape returns to the call before closing the panel. Outside a call the conversation list is the default. Browse a server's channels without joining voice or marking messages read. Open window, right-click on the icon, or selecting a conversation opens the retained client window. Close hides the interface without logging out or leaving voice. Restart Discord performs a full backend restart and reconnects the interface; it ends the call but keeps login and settings. Drafts, staged attachments, pending sends, uploads and active sign-in block restart until resolved. The solid stepped bar mark uses the selected Omarchy-style silhouette. Its separate microphone button is always visible: it is dimmed and unavailable outside a call, and left-click toggles your microphone during a call without opening the client. Right-click silences both your microphone and Discord call/stream playback; repeat it to restore listening and the microphone state from before silencing. Other applications are unaffected. The icon becomes crossed-out headphones while listening is disabled. Left-click in that mode enables both microphone and listening, regardless of the earlier microphone state; right-click instead restores the earlier microphone state. Pending audio requests temporarily disable the button. Tab, Return and Space operate microphone mute; Shift+Return or Shift+Space operates combined silence when the bar has keyboard focus. During a call, the compact panel and window toolbar also expose microphone controls. Middle click keeps its configured quick action. Log in from the window by scanning the QR code with Discord on your phone, or paste a user token into the login field. Do not share the token.

- Search finds servers, text channels, voice rooms and direct messages, not full message history. Ctrl+K opens the same switcher; category buttons and result labels identify what is being searched. Tab and Shift+Tab move between categories and the search input; Return or Space selects a category, and arrow keys move through results while the input is focused.
- The toolbar uses icon buttons with descriptive tooltips and accessible names. Compact switches to a 520×560 window with persistent voice-room controls within the channel-content area whenever a call is active or has failed. Full opens a wider 1040×680 floating window instead of restoring a potentially smaller tile. Switching sizes preserves the visible conversation and does not change the voice connection. Drafts and the conversation stay in the same client. Server-name hover/cursor tooltips are disabled.
- Compact and narrow windows start with servers and the selected server's channel tree side by side. Clicking a server only lists its text and voice channels; it never opens the first or remembered chat. Select a channel explicitly to open its conversation, then use Channels or Escape to return. Reopening the compact client starts at navigation unless an explicit channel link was requested. Existing drafts are retained.
- Direct Messages and Archived servers are icon navigation links above a separated Servers section, not server tiles. Find server filters server names. Visible icon buttons select All, Unread, Mentions, Voice or Archive. Mentions filters servers with a nonzero mention counter, not only personal tags. Channel buttons select All, Text, Voice or Unread. Tab reaches the visible filters; Return or Space applies them. Channel navigation and activation use the filtered list. The slider icon exposes additional search and sorting; sort by Discord order, name, mentions or approximate online count. Online counts load only on request, are snapshots rather than chat activity, and failed/missing counts remain No data. Each refresh is a serial sweep capped at 200 servers or 2 minutes; remaining servers have no data.
- Right-click a server, or press the Menu key or Shift+F10 from its rail cursor, for Server settings, Mark all as read, Archive/Unarchive server and Leave server. Archiving moves inactive servers out of the main list into an Archived section in the rail and filter dropdown. Unarchiving restores the server. Settings currently expose account-wide mute/unmute, not administration or roles. Leaving requires an explicit confirmation; server owners must transfer ownership first. Mark all as read acknowledges visible cached server channels at their last known message; errors may leave a partially acknowledged server. Account actions depend on Discord and are not exercised by inert tests.
- The Options button opens an account card with shortcuts reference and guarded Log out, keeping destructive and secondary actions away from the primary toolbar.
- Voice channels also support text chat: selecting a voice channel in the list or search opens its messages without connecting to voice. Join voice above the messages connects only when explicitly pressed. Clicking Chat or the room title in the call bar opens its text channel beside the call controls without changing the call.
- Navigation follows servers, the selected server's channels, then channel content. In Full, the voice room's people and controls occupy the left side of that content area, not the server or channel list. In narrow browsing, the server rail keeps its readable width and the retained call sits below the selected server's channel tree. In narrow chat, people and controls remain to the left of the messages. They never overlay navigation. Compact suppresses the optional server-wide member column during a call, not the voice panel. The room list follows voice membership events and remains visible without a text channel or when another server is selected. Missing membership data is marked unavailable.
- Experimental video module: join a voice channel, press Video in the toolbar, then choose a participant's Camera or Screen. A screen share opens its own Go Live connection and encryption group; closing it does not leave voice. One H.264 video is decoded by an owned FFmpeg process, displayed in a separate movable and resizable Video window and refreshed at most 5 times per second. Drag its title to move and its lower-right corner to resize. Closing the main client or switching Compact does not close Video. Close or Escape inside Video stops that stream without leaving voice. Quality selects a 360p, 720p or 1080p output cap and corresponding server pixel hint; sender quality can be lower, and this is not a guaranteed remote encoding mode. Stream Volume ranges from 0 to 200 percent independently of the participant's voice volume; participant mute/deafen still apply. Two image slots retain the displayed frame while the next JPEG loads, then switch only when it is ready. Closing Video or leaving voice stops watching and preserves the conversation and draft. It never turns on your camera, captures your screen or records video. Camera subscription changes retain known audio streams and disable only announced video streams, never unspecified media. H.264 screen shares play authenticated Opus audio through the existing call output, respecting deafen and the participant's local mute and volume. Closing the viewer stops stream audio without ending voice. Other codecs and automatic stream reconnect are not implemented. Video assembly waits up to 500 ms across at most four frames, accounts for padding-only sequence numbers, tolerates reordered RTX and requests explicitly missing packets with bounded NACK rounds. H.264 uses the native receive catalog (RTP105/RTX106); RTP101 padding is not evidence of H.264. Authenticated transport delivery reports include padding probes, and explicit RTX mappings restore received retransmissions. Live H.264 screen reception has produced decoded images through an authenticated Discord stream; the installed live viewer has shown advancing ready images and decoded stream audio with no media errors in a bounded check. Physical audibility, pointer interaction, uninterrupted call stability and live webcam reception remain unverified. The immutable 0.1.0 beta above does not include this module.
- Direct-message rows show the recipient's available presence and custom status, with ordinary activity used when no custom status is supplied. The conversation header also shows how long the unchanged presence and status text have been observed by this loaded view. These are observation durations, not Discord's historical activation time; they reset after disconnection or UI recreation. Missing presence remains Status unknown, and offline may include invisible mode. Group conversations do not pretend to have one shared presence. Closed-chat presence events update the lists without joining voice or acknowledging messages.
- Participant rows show Discord presence as text: Online, Away, Do not disturb, Offline or Status unknown. Offline may also mean invisible mode; missing presence is not treated as offline. Speaking and voice membership are separate from account presence. A recent departure remains visible for one minute with its last known presence and updates when Discord supplies a new presence event. It does not identify whether the person left deliberately or lost connectivity. The bar mark uses an original theme-colored outline consistent with the action icons.
- Right-click a participant in the call roster, or focus the roster, select a person with arrow keys and press Return, to adjust their local audio. Mute locally affects only what you hear; the volume slider ranges from 0 to 200 percent. Other participants, microphone transmission and Discord account settings are unchanged. Preferences survive room changes and reconnects during the current signed-in backend session, but not a backend restart. Back to people or Escape restores the roster.
- Audio details in the call panel shows microphone level, actual successful encrypted UDP sends, incoming non-silence packets, output samples, active input/output devices, system mute/zero-volume state and media error counters. Details replaces the roster temporarily; Show people restores it. Devices and errors is a separate disclosure. Reconnect audio rebuilds the same-room voice connection without logging out and preserves mute/deafen flags; it briefly interrupts the call. It is an explicit recovery action, not a permanent fix for receive-audio loss. Test speakers requests one half-second, low-amplitude local tone through the call's existing output, never through Discord. Microphone transmission is suppressed for one second around the test to prevent acoustic feedback from sending the tone into the room. It does not change system volume or mute, so an inaudible test remains a useful warning. Packet delivery does not confirm that another person hears you, and supplied output samples do not certify audible speakers. No incoming audio can mean silence or a receive fault; it is not an away status. Diagnostics runs only while the call panel is visible and connected.
- A failed voice session retains its room panel with a Disconnected status and an explicit Reconnect button. Room members are not labeled as an active call while disconnected. An invalid voice session (4006) triggers at most one fresh-session recovery per explicit join, preserving the room and mute/deafen flags. A repeat failure stops at Disconnected; Reconnect starts a new user-requested attempt. Leaving cancels stale recovery. The exact gateway error remains in the reconnect tooltip. Error 4006 means the voice session is no longer valid, not that Discord login expired.
- The optional server Members list appears beside the conversation when there is room. It is hidden while browsing compact server/channel navigation and returns with the conversation. The people icon shows or hides it; narrow active calls reserve that space for the persistent left voice panel. Narrow lists wrap names and omit activity subtitles. Hover or keyboard focus reveals full member details. Escape returns focus to the composer.
- Read acknowledgements wait for Discord before updating local read state. Failed acknowledgements surface an error rather than reporting local success. Older local acknowledgement requests do not move the read boundary backwards; new messages can still make a channel unread.
- Failed actions show a bounded, scrollable error message with a Dismiss error button. Dismissing only clears the message: it does not retry the action, leave a call or discard a draft. A subsequent failure shows its message again.
- Log out asks for confirmation before ending the session and attempting to remove its saved token. Cancel is initially focused. Close only hides the panel.
- Escape walks back through focus zones; it does not close a ready panel from the server rail.

Attachments remain links, not image previews. The experimental call-camera viewer is separate from attachment fetching. Bar settings control connection lifetime, notifications, mention count and window persistence. Real QR login, delivery, clipboard and audio depend on the account, network and desktop; inert tests do not certify them.

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
