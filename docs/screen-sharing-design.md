# Outbound Discord screen sharing

Status: portal source-selection helper, opt-in native MLS/video encryption and tested RTP sender foundations, not an available broadcasting feature. Receiving and publishing are different media owners; a working viewer does not imply a working sender.

## Product contract

Start from an explicit Share screen button while connected to a voice room. Ask the desktop portal to select one window or monitor. Show a local preview and source label before Start sharing. Keep a persistent sharing indicator and a Stop sharing action in the call UI even when the main client closes. Cancelled selection sends no Discord stream-create command. Microphone and screen audio are separate toggles; screen audio is off in the first version. No background capture restart after reconnect.

Resolution, frame rate and compression are separate settings. The initial target is 720p30 with a high-detail variable bitrate, not an unbounded maximum. A small preview must not force a low-detail remote layer. Bound sender bitrate and queues, then adapt to measured loss/RTT and receiver feedback. Do not market requested bitrate as delivered quality. A 1080p30 option follows measured encoder/network acceptance.

## Capture and encode

Use org.freedesktop.portal.ScreenCast CreateSession, SelectSources, Start and OpenPipeWireRemote. The portal owns source selection and session revocation. Pass the returned PipeWire file descriptor and node ID to one task-owned capture helper; never read the whole desktop through a compositor workaround. Close the portal session and FD on cancel, stop, session loss or helper failure. No frame files or recording.

The current desktop advertises ScreenCast version6 and source types7. GStreamer pipewiresrc is installed; x264enc is absent. FFmpeg has libx264 and the installed libavcodec/libswscale development libraries are available. scripts/screen-encode.py now provides an import-only GStreamer pipewiresrc-to-FFmpeg raw-frame bridge: this FFmpeg build has no PipeWire input device. It duplicates an explicitly supplied portal FD/node without opening another desktop connection. No current UI or service instantiates this encoder. The helper passes finite moving synthetic encode/decode and backpressure-stop tests, not real portal capture acceptance. Vulkan H.264 is available but is an optional later optimization, not the only startup path. Use low-delay H.264, no B frames, periodic IDR plus feedback-triggered keyframes and a bounded latest capture queue. Encoder output preserves all reference pictures required by later frames. The present helper uses RGB conversion and two bounded raw-frame slots, FFmpeg libx264 with zerolatency/no B frames and repeated SPS/PPS/AUD, and bounded process cleanup. Its caller must continuously drain the encoded pipe through access_units(), which splits the explicit x264 AUD boundaries with a 4MiB per-picture limit. Synthetic tests cover every two-byte read boundary, missing delimiters, oversized pictures and frame counts. Source pipeline failure stops the encoder instead of retaining capture. Feedback-triggered keyframes and room/portal-to-encoder orchestration remain unfinished.

## Discord lifecycle and transport

Keep the main voice connection/account. Create an owned outbound stream using the gateway Stream Create operation18, wait finitely for matching STREAM_CREATE and STREAM_SERVER_UPDATE, validate the current room and permission, then open a separate RTC/DAVE stream connection. Reuse handshake validation and cleanup patterns, not the receive-only videoModule selection key. Stop uses operation19 for this exact owned stream and must not end the voice call or an unrelated viewer.

This transport currently advertises H264 encode:false/decode:true. A sender must deliberately negotiate encode support and validate assigned video/RTX SSRCs; do not guess offsets. Map frame timing to a 90kHz RTP clock, fragment H.264 within MTU, preserve marker/access-unit boundaries, implement RTCP sender reports, PLI/FIR handling and bounded RTX history/NACK replies. The existing UDP Write method is Opus-specific and cannot send video safely. PacketizeH264 now converts one already-encrypted Annex B access unit into bounded single-NAL/FU-A payloads. A synthetic receiver-assembly roundtrip covers multi-packet units, sequence wrap, ownership and malformed/MTU/packet-budget rejection. The packetizer itself does not perform encryption or network transmission. The dedicated VideoWriter described below now uses it with separate sequence/timestamp state and shared authenticated transport nonce ownership; there is still no user-facing sender.

DAVE encrypts complete encoded access units before RTP fragmentation, followed by transport AEAD. The pinned dave-go version 0.5.1 explicitly rejects video encryption: codecs.supportedForEncrypt permits only Opus and unknown full-encrypt fallback. Its error documentation cites unaudited H.26x nonce retries and potential AES-GCM nonce reuse. The retained H.264 functions are not evidence of supported sending; the earlier design assumption was incorrect. Do not enable those paths by changing a codec constant or bypassing the rejection. An opt-in official libdave adapter is now implemented under backend/internal/davevideo with build tag libdave. Official libdave 1.2.1 was provisioned in a task-private directory, not installed globally or selected by the normal build. Its single-member synthetic MLS/video/RTP roundtrip and duplicate-transition nonce guards pass. Multi-member live gateway transitions and native-enabled packaging remain unverified; the working audio connection is unchanged.

VideoWriter now requires an explicit EncryptH264 adapter and ready epoch before fragmenting a complete encrypted access unit. It shares the transport AEAD write lock/nonce space, uses separately supplied video SSRC/timestamp, enforces packet bounds, a finite write deadline and cancellation, and never falls back to Opus or plaintext. OpenMediaSender negotiates H.264 encode only with that adapter and closes its own connection on cancellation. The normal production audio-only adapter is rejected before opening a sender network connection. A separately built native-enabled adapter passes real local MLS/H.264 encryption and FFmpeg-generated frame decoding through the actual sender writer under both transport AEAD modes. Synthetic fixed-key DAVE frame authentication and both real localhost transport AEAD modes passed roundtrip/tamper/no-fallback checks; this is not live MLS or Discord sender verification. RTCP sender reports, RTX history and congestion adaptation are still pending.

## Implemented source-selection helper

scripts/screen-source.py owns a real ScreenCast portal request/session lifecycle through Gio. It subscribes before calls, verifies request identity, selects exactly one monitor/window without persistent permission, and closes requests/session on cancellation or a finite timeout. Portal revocation and stdin closure release the session and any PipeWire descriptor. OpenPipeWireRemote is an explicit API call, never automatic selection-side capture. The standalone helper only selects and holds the source session; it does not encode, record, create a Discord stream or transmit.

The contract test runs the actual helper against a synthetic portal on a private D-Bus session, including transferred Unix descriptors. It must not run on the production session bus:

```sh
timeout 12s dbus-run-session -- /usr/bin/python3 -B tests/screen_source_portal.py
```

Requires Python with PyGObject/Gio and dbus-run-session for the test. The encoder additionally requires the Gst introspection bindings, GStreamer pipewiresrc/videoconvert/videoscale/videorate/appsink and FFmpeg libx264. Its separate no-desktop check is `timeout 12s /usr/bin/python3 -B tests/screen_encode.py`; run it in an isolated worker with no session bus or PipeWire socket. No UI Share button invokes the helper yet. Portal selection alone is not successful screen sharing, and synthetic descriptors do not establish real PipeWire capture.

## State and bounded cleanup

Idle -> selecting -> preview -> creating -> negotiating -> sharing -> stopping -> idle. Every asynchronous result is tied to room, account/session generation and request revision. One sender, one portal session, one encoder and one stream transport. A 15-second create/handshake budget and a 3-second graceful stop budget end in explicit error/cleanup, not silent retry. Room leave, portal revoke, encoder exit, DAVE failure and gateway stream deletion invalidate the sender. Keep receiver and sender state independent so watching another stream can coexist with sharing.

## Acceptance before enabling

1. Cancel/revoke a synthetic portal fixture and confirm no stream-create, no retained FD/process and no frame writes.
2. Encode moving synthetic content at low resolution; measure delivered rate, keyframe latency, detail and queue bounds.
3. Encrypted local RTP roundtrip for both supported transport AEAD modes, DAVE video authentication, fragmentation/RTX/PLI and malformed packet controls.
4. Actual native Start/Stop controls, delayed replies, room/session changes and call/microphone/receive-state preservation.
5. User-selected portal source, explicitly initiated sharing and a consenting second Discord consumer confirming visible motion/text. This live check is separate from synthetic packet tests.

Do not add a disabled button that suggests the feature already works. The implementation begins with the DAVE video sender and portal helper prerequisites, then integrates the UI and live publishing last.
