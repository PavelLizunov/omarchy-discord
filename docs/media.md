# Media implementation boundaries

Omacord remains a standalone client. It does not control an official Discord or Tuple process.

## Reused components

- `hraban/opus` and system libopus encode, decode, classify silence with DTX, and conceal lost packets. Capture does not discard quiet speech with a fixed RMS threshold. Playback keeps the decoded packet duration independent of the 20 ms output block; loss concealment uses libopus’s last packet duration.
- `jfreymuth/pulse` connects capture and playback to the desktop audio server.
- Pion RTP reconstructs H.264 NAL units. The local layer retains strict FU/STAP validation, bounded reordering, retransmission requests and authenticated DAVE frame admission.
- Pion RTCP serializes PLI, NACK, receiver reports and transport delivery feedback. Discord-specific AEAD headers, stream ownership and feedback scheduling remain local.
- FFmpeg decodes the selected incoming H.264 stream. The QML viewer still receives JPEG snapshots; this is not direct GPU video rendering.

The audio jitter queue and mixer are still local. Replacing packet serializers does not replace the whole transport or establish long-call stability. Dependency versions and licenses are in [the notices](../THIRD-PARTY.md).

## Outbound capture is not screen sharing

`scripts/capture-screen.py` is an unintegrated capture helper, not a plugin feature. It requests one monitor or window through the desktop ScreenCast portal, opens the supplied PipeWire remote, and emits H.264 on stdout. It is not invoked by the plugin and does not join Discord, obtain account credentials or save a recording.

The helper uses PyGObject and GStreamer `pipewiresrc`, `queue`, `videoconvert`, `videoscale`, `videorate`, `x264enc`, `h264parse` and `fdsink`. GStreamer owns frame layout, conversion, encoding and pipe backpressure; there is no Python pixel-copy loop or second encoder process. Encoding currently uses CPU x264. The raw queue drops old whole frames before encoding; encoded bytes are not deliberately dropped. Signal, portal closure and pipeline error paths close the owned capture session. These paths remain unverified on the target machine.

A complete sender still needs stream creation/negotiation, video-capable DAVE session integration, RTP packetization, feedback/keyframe handling and lifecycle/UI integration. Pinned `godave v0.3.0` exposes only Opus, and the `dave-go v0.5.1` session adapter maps only that codec. Its internal H.264 codec functions do not make an arbitrary `godave.Codec` cast a video adapter: unknown codecs fall back to Opus framing. No cryptography is replaced here.

## Candidate status

These source changes have not been built, tested or loaded. The existing bundled binary and its source stamp are intentionally unchanged; the installer detects the mismatch and needs a source build. They must not be relabeled as this candidate without producing the corresponding binary. The repository check has not run for this candidate; its source-fingerprint comparison would reject the unchanged bundled artifact until packaging is completed.

The current work uses only the designated test machine for tests and deployment. Test-host authentication is unavailable, so the [check commands](checks.md), native portal interaction and live voice/video acceptance have not run for this candidate. Historical checks on earlier commits are not evidence for these changes.
