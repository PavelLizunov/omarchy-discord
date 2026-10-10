# Omacord receive-only camera transport

Verdict: Extend the pinned Disgo voice transport instead of adding a second Discord account or voice connection.

This package is copied from `github.com/disgoorg/disgo/voice` at `v0.19.7-0.20260825183231-aa92366d296d`. Its original Apache-2.0 LICENSE is retained. Only the voice package is copied; the main gateway and public Discord models remain the existing dependency. Upstream tests were not copied; Omacord's voice integration checks exercise this local package.

Changes: receive-capable video Identify and H264-only codec advertisement, opcode 12 stream metadata and SSRC ownership, opcode 15 subscriptions, RTP marker/payload demultiplexing, transport-only H264 fragment decryption, larger UDP receive buffer, serialized outgoing encryption and encrypted RTCP PLI. Complete video frames go through the existing DAVE session after H264 depacketization. Audio retains its existing per-packet path.

The MVP watches one selected webcam, not Go Live streams. It does not capture a local camera, publish video, record incoming video, implement RTX/NACK, or support codecs other than H264. The backend owns and reaps one FFmpeg decoder, with fixed pipe-only arguments and bounded output dimensions/queues/frame sizes. Close, Leave and backend teardown stop it. UI snapshots contain only a recent image in memory.

Local synthetic decoder/packet checks and offscreen UI evidence do not certify live Discord negotiation, remote webcam delivery or physical user acceptance. Historical upstream gateway heartbeat/lifecycle limitations remain outside this patch.
