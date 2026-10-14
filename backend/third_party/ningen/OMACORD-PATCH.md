# Omacord read-state snapshot patch

Vendored source: github.com/diamondburned/ningen/v3, version v3.0.1-0.20260306213430-5a08d3a709b4. Upstream source and MIT license are retained.

The only production change is in states/read/read.go: ReadState copies its gateway.ReadState under the existing mutex and returns the detached copy. Previously it returned the map's mutable pointer after unlocking, racing with markRead/MarkUnread during DM metadata and acknowledgement handling. All callers, including ningen's own unread helpers, now receive a snapshot. Public signature and nil/invalid-boundary semantics are unchanged.

The local Go module replacement avoids modifying shared module caches and includes the patch in offline/source builds. scripts/build-backend.sh includes this local dependency in its bundled-source fingerprint. Task regression tests in backend/internal/session/dm_visibility_test.go cover detached values and concurrent writes/readers without drain-based serialization.

This patch does not provide a transaction across different read-state queries, order asynchronous UpdateEvent delivery, or certify other upstream state stores. No upstream submission is implied.
