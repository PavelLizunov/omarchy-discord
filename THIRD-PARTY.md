# Media dependency notices

The vendored Disgo voice transport retains its [Apache-2.0 license](backend/internal/voicewire/LICENSE) and [patch provenance](backend/internal/voicewire/OMACORD-PATCH.md).

The media implementation imports these Go modules at the exact versions pinned in `backend/go.mod` and `backend/go.sum`; their sources are not copied into this tree:

- `github.com/pion/rtp v1.10.6`: H.264 depacketization, MIT.
- `github.com/pion/rtcp v1.2.20`: standard RTCP packet serialization, MIT.
- `github.com/pion/randutil v0.1.0`: indirect RTP dependency, MIT.

Their upstream license texts are reproduced below without replacing upstream placeholders. These notices accompany any backend built with these modules. Existing libopus, PulseAudio/PipeWire, FFmpeg, GStreamer and x264 installations retain their respective upstream licenses; they are external runtime components, not newly bundled binaries.

## Native DAVE

The native adapter derives from `github.com/disgoorg/godave/golibdave v0.3.0` and imports `github.com/disgoorg/godave/libdave v0.3.0`, both Apache-2.0. Their notice and adaptation record are in [the adapter directory](backend/internal/dave/PROVENANCE.md). The official libdave `v1.2.2/cpp` x86_64 SDK is bundled with unchanged headers/binary and libdave, BoringSSL, mlspp and nlohmann-json notices in [the SDK directory](backend/native/README.md). No custom cryptography is added.

## Pion rtp v1.10.6

MIT License

Copyright (c) <year> <copyright holders>

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Pion rtcp v1.2.20

MIT License

Copyright (c) 2026 The Pion community <https://pion.ly>

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Pion randutil v0.1.0

MIT License

Copyright (c) 2020 Pion

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
