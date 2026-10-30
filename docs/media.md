# Media implementation boundaries

Omacord remains a standalone client. It does not control an official Discord or Tuple process.

## Reused components

- `hraban/opus` and system libopus encode, decode, classify silence with DTX, and conceal lost packets. Capture does not discard quiet speech with a fixed RMS threshold. Playback keeps the decoded packet duration independent of the 20 ms output block; loss concealment uses libopus’s last packet duration.
- `jfreymuth/pulse` connects capture and playback to the desktop audio server.
- Pion RTP reconstructs H.264 NAL units. The local layer retains strict FU/STAP validation, bounded reordering, retransmission requests and authenticated DAVE frame admission.
- Pion RTCP serializes PLI, NACK, receiver reports and transport delivery feedback. Discord-specific AEAD headers, stream ownership and feedback scheduling remain local.
- FFmpeg decodes the selected incoming H.264 stream. The QML viewer still receives JPEG snapshots; this is not direct GPU video rendering.

The audio jitter queue and mixer are still local. Replacing packet serializers does not replace the whole transport or establish long-call stability. Dependency versions and licenses are in [the notices](../THIRD-PARTY.md).

## Experimental outbound screen sharing

The Video window has an explicit Share screen action. It requests one monitor or window through the desktop ScreenCast portal. The backend does not advertise a Go Live stream until the helper supplies an encoded frame after consent. Capture is video-only; no desktop audio or webcam is captured. Closing Video, stopping sharing, leaving voice or retiring the account session cancels the owned capture and stream. The helper never receives account credentials or saves a recording.

The helper uses PyGObject and GStreamer `pipewiresrc`, `queue`, `videoconvert`, `videoscale`, `videorate`, `x264enc`, `h264parse` and `appsink`. GStreamer owns raw frame layout, conversion and encoding; Python only frames encoded access units with their presentation timestamps. CPU x264 currently targets 720p30 at 2.5 Mbps from the UI. The raw queue drops old whole frames before encoding; encoded access units retain backpressure. Signal, portal closure and pipeline error paths close the owned capture session. A synthetic GStreamer-to-FFmpeg pipeline passed on the earlier capture candidate; real portal selection and cancellation remain unverified.

The sender negotiates a separate Go Live transport, uses only server-assigned video/RTX sources, encrypts complete H.264 frames with official native libdave, and packetizes them with Pion. It handles authenticated PLI/FIR and bounded NACK retransmissions, sends RTCP sender reports, and waits for a keyframe after encryption becomes ready. The 512-packet retransmission cache is bounded. There is no adaptive bitrate controller yet. Native DAVE replaces the audio-only Go session adapter for both send and receive; no video-crypto guard is bypassed. A native lone-member session without a committed epoch stays in a waiting state until an encrypted group is established. Protocol fields were cross-read against Discord-RE/Discord-video-stream commit `33a44a60ec8c4e3ed3bba53074918c9660161931`; that is source evidence, not interoperability acceptance.

## Candidate status

The bundled x86_64 backend is rebuilt from these sources with its matching source stamp. Pion dependency manifests are intentional fork hooks, not upstream-identical files. The candidate also preserves installed private-channel seeding, unnamed-conversation handling and late voice-member name resolution.

The preceding media checkpoint passed repository checks, 74 Node tests and synthetic H.264 encoding/decoding. The native-DAVE/sender candidate also passed the repository guard, all 74 Node tests, Go race tests across all packages and vet, including real native H.264 encryption through Pion packetization/reassembly and authenticated transport feedback. Thirteen scoped camera QML checks passed with the inert offscreen runner. The full private-Xvfb suite was unavailable because `xvfb-run` is missing. An actual-component render was inspected; strict preview acceptance reported host AT-SPI warnings, not a clean render gate. Regression coverage includes large passthrough audio, variable-duration loss concealment, malformed H.264, duplicate/reordered transport feedback and complete participant-source cleanup. The concurrent-login test now acquires the same operation lock as both production login paths instead of racing an internal lock-required function.

Native portal interaction, actual Discord outbound sharing, live remote media and physical audibility remain separate acceptance gaps. Automated checks do not certify them. See [check commands](checks.md).
