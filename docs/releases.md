# Testing release: 0.2.0-beta.2

GitHub tag `v0.2.0-beta.2` freezes the reviewed bar-audio, server-counter and repository-check changes after the video preview. Download the complete plugin archive, rather than only the backend binary, and verify it against the release checksums. See [CHANGELOG.md](../CHANGELOG.md) for changes and known limitations.

This release contains the selected solid stepped mark, separate microphone/listening control, all-server cached voice-roster synchronization and delayed server-stat protection. The earlier immutable `v0.2.0-beta.1` retains its original video checkpoint; `v0.1.0` does not contain video viewing.

Screen video and audio have bounded live evidence. Live webcam delivery, different-sender switching, physical audio/control acceptance and uninterrupted voice recovery remain incomplete. Video presentation is H.264/JPEG-based at at most 5 updates per second. Quality is an output cap and server hint, not a promise of remote source detail. Audio/video synchronization and automatic stream reconnect are not implemented. This is a prerelease for testing, not stable-release certification.

Server voice counts reflect cached, permission-filtered rosters, excluding archived servers. They refresh on load/open and every 15 seconds while the window is open; missing upstream events can still leave stale data. The installed snapshot matches the backend, but a natural remote join/leave acceptance check remains pending.
