# Native DAVE SDK

The x86_64 SDK is the unmodified official Discord libdave release `v1.2.2/cpp`, commit `6396c90aedbb9a6ee117ea9ce1ce9832af6a3375`.

Archive: https://github.com/discord/libdave/releases/download/v1.2.2/cpp/libdave-Linux-X64-boringssl.zip

Archive SHA-256: `e9688ba3a1b0908e5e7aa037f400a2f59c69c98677a83a857ed0f2849c338312`.

`lib/libdave.so` SHA-256: `ccdbf9659f0dd1afec25480a31d6db15cd1c0eeb62658e26c74ea98b1d115f71`.

Headers, binary and all dependency notices are preserved. Only the relocatable `lib/pkgconfig/dave.pc` is local. The SDK is not installed globally. The backend installer copies the shared library next to the executable; an `$ORIGIN` runpath resolves it. System libopus remains required.

Before direct Go builds or tests, source `backend/native/env.sh` from the repository root. Tests do not access a live account or capture devices. No other architecture is bundled.
