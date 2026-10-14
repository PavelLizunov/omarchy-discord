# Changelog

## 0.2.0-beta.2 — Bar audio and live server counters

This testing prerelease includes the reviewed changes after beta.1. Earlier release tags and assets remain unchanged.

### Included

- The selected solid stepped application mark at 16 pixels, a smaller 12-pixel audio glyph, a 1-pixel canvas gap and reduced trailing padding.
- Right-click on the bar audio control silences both microphone and Discord playback. Repeating it restores listening and the previous microphone preference. Left-click while silenced explicitly enables both directions. Other applications are unaffected.
- Single-flight audio actions wait for both the command reply and matching voice state, with room/revision cancellation and a bounded timeout.
- Active-voice server counts and filters synchronize all cached rosters on client load/open and every 15 seconds while the window is open, without browsing servers, joining voice or acknowledging messages.
- Superseded server-stat responses cannot replace a newer snapshot after reconnect or service replacement.
- Shorter installation documentation, preserved detailed guides, a pinned fork-boundary guard and isolated backend/QML continuous checks.

### Evidence and limitations

The installed counter was compared with all 17 backend guild rosters without navigation. Actual QML regressions cover unvisited guilds, repeated timers, delayed events/replies, disconnects and timeouts; the old client fails the new counter regression. Natural remote join/leave while the window is open remains unobserved. Cached rosters depend on upstream Discord events.

The complete release checks are recorded in the GitHub notes. This is not a completed independent repository audit or stable-release certification. Physical bar clicks and audibility, live webcam delivery, different-sender transitions, long-running voice stability and recovery remain incomplete. The video limitations from beta.1 still apply: H.264 only, at most 5 JPEG updates per second, no smooth full-rate presentation, no audio/video synchronization or automatic stream reconnect.

## 0.2.0-beta.1 — Video preview

This prerelease freezes the reviewed video development checkpoint. It is available for testing, not a stable or fully verified video client. It does not include the next icon redesign.

### Included

- Experimental authenticated H.264 camera and Go Live receiving paths, with bounded access-unit assembly, explicit RTX mapping and missing-packet feedback.
- A separate movable video window that remains open when the main client closes or switches Compact.
- Screen Opus playback through the existing call output, independent stream volume from 0 to 200 percent, and local participant mute/deafen integration.
- Requested 360p, 720p and 1080p output caps and server pixel hints; the remote sender may supply less detail.
- Two image slots retain the current frame until the next JPEG is ready.
- Stream and quality actions queued behind frame polling, with stable stream-choice controls and delayed-response mouse/keyboard regressions.
- Voice-first compact popup, participant local audio controls, presence display and guarded backend recovery.

### Evidence and limitations

Real authenticated screen video and screen audio decoding/output were observed on this installation. This does not establish physical audibility, uninterrupted voice stability, live webcam delivery or switching between different live senders. The last control repair passed actual offscreen mouse and keyboard checks; no sender was available for its final live switching check.

Presentation remains JPEG-based at up to 5 updates per second, not smooth full-rate video. Only H.264 video is supported. Audio/video synchronization and automatic stream reconnect are not implemented. Discord voice error 4006 and upstream lifecycle problems may still interrupt a call. Attachments remain links, not remote image previews.

Exact release checks are listed in the GitHub release notes. Direct source review is not independent certification. This unofficial client runs inside the shared Omarchy shell with the user's permissions; installation and reload can affect other panels. Original MIT attribution and the pinned transport license are retained.

## 0.1.0 — First beta

The first published Omacord checkpoint. GitHub tag: `v0.1.0`; prerelease status: beta. This fork builds on the existing Omarchy Discord client; the original MIT attribution is retained.

### Included

- Native server-name navigation, server archive, filters, sorting and category-based search.
- Explicit server browsing: selecting a server shows its channels without automatically opening a remembered or default chat.
- Independent voice-channel text chat. Joining audio requires the explicit Join voice action.
- Servers → channels → content layout, with voice participants and controls inside the content area. Compact and Full preserve the visible conversation and drafts.
- Bounded server-control scrolling in small windows, keyboard focus routing, native themed icons and tooltips, and composited text-contrast corrections for tested states.
- Account options, notification/connection preferences, guarded logout, bounded error feedback and a shorter shortcuts reference.
- At most one fresh-session recovery for voice error 4006 per explicit join, with cancellation and stale-generation protection.
- Confirmed read acknowledgements: Discord failures are surfaced, and older local requests cannot move the read boundary backwards.
- Search request generations prevent late results from a previous opening replacing the current results.
- A bundled Linux x86_64 backend with a production-source fingerprint. Release assets include the complete plugin, the backend executable and checksums.

### Verification scope

The checkpoint has 57 passing Node tests, 88 passing native offscreen Qt tests, passing normal Go tests and Go vet, scoped read/voice race checks, and isolated exact-binary socket/lifecycle checks. Native images were inspected for the affected layouts. Installed files and the running backend were compared with the checked candidate.

These checks use inert account, storage and media fixtures where necessary. They do not certify real message delivery, QR approval, microphone/playback continuity, live voice recovery, assistive technology, every shortcut or every possible theme. Review was direct self-review, not independent signoff.

### Known limitations

- Participant video and Go Live stream viewing are not implemented. Attachments remain links; remote images are not previewed.
- A voice gateway Hello/heartbeat initialization race remains unresolved in an upstream dependency. A broader session race-mode run also exposed a concurrent-login teardown assertion failure; scoped passes do not establish full race safety.
- The read-acknowledgement failure mechanism was reproduced and repaired, but the exact cause of the user's repeated-unread report has not been established on the live account.
- Discord integration is unofficial and uses account/session behavior that can change upstream. Treat this as a beta, not a compatibility guarantee.
- Installation/reload affects the shared Omarchy shell. Preservation of unrelated transient buffers across a shared plugin reload is not verified.
