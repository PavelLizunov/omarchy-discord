# Actual-consumer preview

`Preview.qml` loads the production `ClientView.qml` with an inert model.
`SwitcherPreview.qml` loads production `SwitcherView.qml`. No Quickshell,
mock import modules, production credentials, network or commands are used.

MCP: `qml-preview.render_qml`, `readyProperty: ready`, `locale: ru_RU`,
`warningsPolicy: error`, explicit production/fixture dependency hashes.
Test 1040×680 and 640×420 at DPR 1; members/light theme at 640×520, DPR 2.
States: chat, loading, empty, error, login, qr, members, voice.

The QR image is a synthetic `example.invalid` URL, not an authentication code.
The readiness condition waits for its actual Image.Ready state.

Keyboard checks:

```sh
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software \
  /usr/lib/qt6/bin/qmltestrunner -input tests/visual
```

These exercise production navigation, composer, attachment links, spoiler
reveal, copied text, search/activation, login actions and voice control routes
against a recorder. They do not send Discord messages or establish a call.
