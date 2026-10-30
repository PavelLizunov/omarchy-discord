# Native DAVE adapter

`session.go` adapts `github.com/disgoorg/godave/golibdave v0.3.0` under its Apache-2.0 license (reproduced in `LICENSE`). The upstream transition orchestration is retained. Local changes serialize native operations with retirement, reject empty buffers and unknown participants, bound maps, gate media on negotiated encryption, and expose H.264 assignment and explicit video encrypt/decrypt methods. MLS, key derivation, media encryption and H.264 authenticated ranges remain in Discord's libdave, not a local cryptographic implementation.

The low-level binding is `github.com/disgoorg/godave/libdave v0.3.0`. Native resource destruction is owned by its Go finalizers. The adapter releases references on close. The binding supports the explicit `MediaTypeVideo` calls missing from the upstream high-level adapter.

The synthetic encryption test establishes a local epoch using an empty proposal batch, then exercises real native H.264 encryption/decryption. This fixture is not a Discord handshake. Production never fabricates a commit or relaxes readiness for a missing key ratchet. A lone-member transition without a committed native epoch remains not ready until Discord establishes one.
