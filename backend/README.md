# omarchy-discord-backend

Go daemon behind the `quickshell.discord` plugin: owns the Discord session
(arikawa v3 + ningen v3) and serves the line-delimited JSON protocol in
`docs/BACKEND_PROTOCOL.md` over `$XDG_RUNTIME_DIR/omarchy-discord/backend.sock`.

## Build

Never build into the plugin tree (Omarchy hot-reloads on any write):

```sh
cd backend
export GOCACHE="${XDG_CACHE_HOME:-$HOME/.cache}/omarchy-discord/gocache"
go build -o "${XDG_CACHE_HOME:-$HOME/.cache}/omarchy-discord/target/omarchy-discord-backend" \
  ./cmd/omarchy-discord-backend
```

Quality gate (all must pass): `gofmt -l .` (empty), `go vet ./...`,
`go test ./...`, `go build ./...`. Golden fixtures are regenerated with
`go test ./internal/... -update` and must be committed with the change.

## Subcommands

| command | what it does |
|---|---|
| `serve` (default) | run the socket server and the Discord session; logs to stderr only |
| `check` | print a JSON environment summary to stdout and exit (`secret_tool`, `runtime_dir_writable`, `media_cache_writable`, `staged_dir_writable`, `token_present`, `audio_server` (the Pulse socket voice needs); never the token) |
| `login` | read a token from the first line of stdin, validate it with `GET /users/@me`, store it in the keyring, exit |
| `logout` | clear the keyring entry (looped, max 20) |

Flags: `--socket-path PATH` overrides the socket location (before or after the
subcommand).

The token lives in the GNOME keyring under `service quickshell-discord kind
user-token`, written by `secret-tool store` with the token on stdin. It is held
only in memory while serving and never logged.

## Exit codes

| code | meaning |
|---|---|
| 0 | ok (`check`: environment ready and a token is stored) |
| 1 | usage error / unreadable stdin |
| 2 | runtime, staged, or media cache dir not writable, or socket bind failed |
| 3 | `secret-tool` not installed |
| 4 | `check`: no token stored; `login`: empty stdin |
| 5 | `login`: token rejected by Discord |
| 6 | keyring store/clear failed |
