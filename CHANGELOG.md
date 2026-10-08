# Changelog

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
