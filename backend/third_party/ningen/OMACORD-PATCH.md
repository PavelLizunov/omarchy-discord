# Omacord state patches

Vendored source: github.com/diamondburned/ningen/v3, version v3.0.1-0.20260306213430-5a08d3a709b4. Upstream source and MIT license are retained.

The read-state change is in states/read/read.go: ReadState copies its gateway.ReadState under the existing mutex and returns the detached copy. Previously it returned the map's mutable pointer after unlocking, racing with markRead/MarkUnread during DM metadata and acknowledgement handling. All callers, including ningen's own unread helpers, now receive a snapshot. Public signature and nil/invalid-boundary semantics are unchanged.

The local Go module replacement avoids modifying shared module caches and includes the patch in offline/source builds. scripts/build-backend.sh includes this local dependency in its bundled-source fingerprint. Task regression tests in backend/internal/session/dm_visibility_test.go cover detached values and concurrent writes/readers without drain-based serialization.

READY private-channel decoding in events.go explicitly unmarshals both the embedded discord.Channel and recipient_ids. Without that method, Channel's promoted UnmarshalJSON consumes the object while the sibling recipient_ids field remains empty. hackReady can then resolve the recipients against READY users and restore private-channel names and the DM presence index. Explicit recipients payloads remain supported. Already connected sessions need a fresh READY to recover previously discarded recipient IDs; deploying the source alone does not repair their in-memory cache.

These patches do not provide a transaction across different read-state queries, order asynchronous UpdateEvent delivery, or certify other upstream state stores. No upstream submission is implied.
