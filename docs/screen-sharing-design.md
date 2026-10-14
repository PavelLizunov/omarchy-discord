# Outbound Discord screen sharing

Status: implementation design with a tested H.264 RTP payload packetizer, not an available broadcasting feature. Receiving and publishing are different media owners; a working viewer does not imply a working sender.

## Product contract

Start from an explicit Share screen button while connected to a voice room. Ask the desktop portal to select one window or monitor. Show a local preview and source label before Start sharing. Keep a persistent sharing indicator and a Stop sharing action in the call UI even when the main client closes. Cancelled selection sends no Discord stream-create command. Microphone and screen audio are separate toggles; screen audio is off in the first version. No background capture restart after reconnect.

Resolution, frame rate and compression are separate settings. The initial target is 720p30 with a high-detail variable bitrate, not an unbounded maximum. A small preview must not force a low-detail remote layer. Bound sender bitrate and queues, then adapt to measured loss/RTT and receiver feedback. Do not market requested bitrate as delivered quality. A 1080p30 option follows measured encoder/network acceptance.

## Capture and encode

Use org.freedesktop.portal.ScreenCast CreateSession, SelectSources, Start and OpenPipeWireRemote. The portal owns source selection and session revocation. Pass the returned PipeWire file descriptor and node ID to one task-owned capture helper; never read the whole desktop through a compositor workaround. Close the portal session and FD on cancel, stop, session loss or helper failure. No frame files or recording.

The current desktop advertises ScreenCast version6 and source types7. GStreamer pipewiresrc is installed; x264enc is absent. FFmpeg has libx264 and the installed libavcodec/libswscale development libraries are available. Use a small PipeWire/libavcodec helper for capture-to-H.264 access units rather than assume ffmpeg can consume portal nodes: this FFmpeg build has no PipeWire input device. Vulkan H.264 is available but is an optional later optimization, not the only startup path. Use low-delay H.264, no B frames, periodic IDR plus feedback-triggered keyframes and a bounded latest capture queue. Encoder output preserves all reference pictures required by later frames.

## Discord lifecycle and transport

Keep the main voice connection/account. Create an owned outbound stream using the gateway Stream Create operation18, wait finitely for matching STREAM_CREATE and STREAM_SERVER_UPDATE, validate the current room and permission, then open a separate RTC/DAVE stream connection. Reuse handshake validation and cleanup patterns, not the receive-only videoModule selection key. Stop uses operation19 for this exact owned stream and must not end the voice call or an unrelated viewer.

This transport currently advertises H264 encode:false/decode:true. A sender must deliberately negotiate encode support and validate assigned video/RTX SSRCs; do not guess offsets. Map frame timing to a 90kHz RTP clock, fragment H.264 within MTU, preserve marker/access-unit boundaries, implement RTCP sender reports, PLI/FIR handling and bounded RTX history/NACK replies. The existing UDP Write method is Opus-specific and cannot send video safely. PacketizeH264 now converts one already-encrypted Annex B access unit into bounded single-NAL/FU-A payloads. A synthetic receiver-assembly roundtrip covers multi-packet units, sequence wrap, ownership and malformed/MTU/packet-budget rejection. It is not wired to a sender and does not perform encryption or network transmission. Add a dedicated video writer with separate sequence/timestamp state and shared authenticated transport nonce ownership.

DAVE encrypts complete encoded access units before RTP fragmentation, followed by transport AEAD. The pinned dave-go codec implementation supports H.264 internally, but its godave Session adapter maps only CodecOpus; unknown codecs are not a supported sender route. Extend that owning adapter with a typed H.264 assignment or a small reviewed local wrapper, then test actual encrypted encode/decode. Never reuse Opus assignment or plaintext passthrough to pretend video is encrypted. This is a concrete prerequisite, not a solved feature.

## State and bounded cleanup

Idle -> selecting -> preview -> creating -> negotiating -> sharing -> stopping -> idle. Every asynchronous result is tied to room, account/session generation and request revision. One sender, one portal session, one encoder and one stream transport. A 15-second create/handshake budget and a 3-second graceful stop budget end in explicit error/cleanup, not silent retry. Room leave, portal revoke, encoder exit, DAVE failure and gateway stream deletion invalidate the sender. Keep receiver and sender state independent so watching another stream can coexist with sharing.

## Acceptance before enabling

1. Cancel/revoke a synthetic portal fixture and confirm no stream-create, no retained FD/process and no frame writes.
2. Encode moving synthetic content at low resolution; measure delivered rate, keyframe latency, detail and queue bounds.
3. Encrypted local RTP roundtrip for both supported transport AEAD modes, DAVE video authentication, fragmentation/RTX/PLI and malformed packet controls.
4. Actual native Start/Stop controls, delayed replies, room/session changes and call/microphone/receive-state preservation.
5. User-selected portal source, explicitly initiated sharing and a consenting second Discord consumer confirming visible motion/text. This live check is separate from synthetic packet tests.

Do not add a disabled button that suggests the feature already works. The implementation begins with the DAVE video sender and portal helper prerequisites, then integrates the UI and live publishing last.
