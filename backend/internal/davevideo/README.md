# Experimental native video DAVE adapter

Build tag: `libdave`. This package is not selected by the current voice engine, viewer or any user-facing screen-sharing action. Normal builds exclude it and keep the installed audio adapter.

The adapter calls the official libdave C API for MLS, H.264 media encryption and media decryption. It owns and deterministically releases native handles, serializes operations, rejects unavailable epochs and never enables passthrough. Duplicate transition events do not reinstall the same sending ratchet: official SetKeyRatchet resets its nonce, so the last MLS epoch authenticator identifies whether installation is actually new.

## Verified dependency

Tests used official `discord/libdave` tag `v1.2.1/cpp`, commit `8de72b1f8a2ac3c5a5270755bb8091a62e3c6169`, Linux X64 BoringSSL release asset. Library, headers and dependency licenses were extracted into a private task directory, not installed globally or added to the normal plugin archive. The upstream root license is MIT; the release includes BoringSSL, MLS++ and JSON licenses. Review and retain those licenses when packaging a native-enabled build.

Requires FFmpeg with libx264 for the generated-frame test, the libdave headers/shared library and a C compiler. Provide a `dave.pc` for a separately provisioned library, add its directory to PKG_CONFIG_PATH and make its shared-library directory visible to the loader. The pkg-config entry needs Cflags pointing to the parent of dave/dave.h and Libs containing -L for the library directory and -ldave. Then run:

```sh
go test -mod=readonly -tags 'libdave,nolibopusfile' -race -count=1 -timeout=30s ./internal/davevideo
```

The test is finite and synthetic. It creates a real local native MLS epoch from an inert external sender fixture, encrypts/decrypts 50 H.264 units interleaved with 50 audio frames with unique shared nonces, repeats immediate and staged transition events, rejects tampering and tests unready/closed state. A real FFmpeg-generated H.264 frame passes native DAVE encryption, the actual video RTP writer, both localhost transport AEAD modes, reassembly, native decryption and FFmpeg decode. No desktop source, account, Discord connection or recording is involved.

## Not yet established

This is not a live Discord sender. Multi-member MLS welcomes, changing rosters, real gateway transitions, recovery, malformed control traffic, native memory instrumentation and independent review still require broader checks before enabling it. The native library's full cryptographic implementation is not re-audited by this wrapper test. It is not substituted into a current call. Capture/encode UI, owned stream-create signaling, RTCP sender feedback, RTX and congestion handling remain separate work.
