# Checks and troubleshooting

Run these checks in an isolated worker or the repository CI, not against the live account. CI covers the repository guard, Node and Go checks. Host-specific Qt and Quickshell acceptance remains separate.

```sh
python3 scripts/check-repository.py
```

```sh
node --test tests/*.test.cjs components/harness/*.test.js
node tests/no-media.cjs
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software \
  /usr/lib/qt6/bin/qmltestrunner -input tests/visual -import .
cd backend
go test -tags nolibopusfile ./...
go test -race -tags nolibopusfile ./...
go vet -tags nolibopusfile ./...
```

Offscreen fixtures use inert services and load the actual UI components. They do not test microphone/playback, phone QR approval, live account operations, compositor placement or screen-reader behavior. `TestPulseSmoke` is opt-in. Backend socket tests require loopback sockets in the test environment. A standalone Qt runner may not load Quickshell's native I/O plugin even when the shared shell does.

Inspect the user service without sending account commands:

```sh
systemctl --user status omarchy-discord.service
journalctl --user -u omarchy-discord.service -n 50
```

The bundled backend is selected only when its production-source fingerprint matches `backend/dist/x86_64/source.sha256`; otherwise the build script compiles from source. This stamp is a freshness guard, not a signature. Keep the binary and stamp synchronized after backend edits.
