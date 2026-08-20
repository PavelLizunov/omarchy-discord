# omarchy-discord backend protocol — v1 (draft)

Line-delimited JSON over a private unix socket. Same framing, envelope, and error-code
discipline as the quickshell.spotify backend protocol; the event model deliberately
departs (granular events instead of full-state-only — a chat timeline cannot be a
single snapshot).

- Socket: `$XDG_RUNTIME_DIR/omarchy-discord/backend.sock`. When `XDG_RUNTIME_DIR` is
  unset, backend **and** client use the single shared fallback
  `/run/user/<uid>/omarchy-discord/backend.sock` — never `/tmp`. Parent dir created;
  stale socket file unlinked before bind; socket `chmod 0600`; removed on clean
  shutdown. Overridable via `--socket-path`.
- Transport: UTF-8 JSON, **one object per line**, `\n`-terminated. No pretty-printing.
- The backend is the single source of truth; QML holds a mirror, never authoritative
  state.

## Envelope

Request — caller-chosen integer `id`, command params flattened at top level:

```json
{"v":1,"id":7,"command":"ping"}
{"v":1,"id":12,"command":"send","channel_id":"1049931213073821696","content":"on my way","reply_to":"1049931302442426390"}
```

Success response (echoes `id`):

```json
{"type":"response","v":1,"id":12,"ok":true,"result":{"message_id":"1049931339989602304","nonce":"k3q9x1"}}
```

Failure response (`result` and `error` are mutually exclusive; only one appears):

```json
{"type":"response","v":1,"id":12,"ok":false,"error":{"code":"unknown_channel","message":"channel is not accessible"}}
```

Event push:

```json
{"type":"event","v":1,"event":"message_create","channel_id":"1049931213073821696","message":{...},"notify":false}
```

Rules:

- Responses may arrive out of request order; correlation is by `id` only. Long-running
  commands (`upload`, `history` on a cold channel) keep the connection usable.
- A request line that fails to parse is answered with `invalid_request` and **`id: 0`**
  (the real id is unknowable); clients must tolerate responses whose id matches no
  pending request. A line that parses but has a missing/empty `command` is also
  `invalid_request`, with its `id` echoed so the pending request can be failed.
- All snowflakes (guild/channel/message/user ids) are **strings** on the wire —
  64-bit integers do not survive QML/JS number round-trips.
- Timestamps are RFC 3339 UTC strings (`"2026-08-20T14:03:22.117Z"`).
- All `error.message` strings and the state `error` field are redacted at the point
  they are built; user content (names, topics, message text) is never altered by
  redaction. Internal sequencing fields never appear on the wire.
- The golden fixtures in `backend/internal/protocol/testdata/` are the canonical,
  byte-exact examples of every request, response, and event shape below.

## Versioning

- `v` is `1` in every request, response, and event.
- An unsupported protocol version is rejected (`unsupported_version`), never guessed.
- Adding an optional field, command, or event is backward-compatible. Removing or
  renaming a field, changing its meaning, or changing framing requires a version bump.
- When the socket is absent, QML shows the disconnected/bootstrap UI and retries with
  backoff; there is no fallback transport.

## Snapshot on connect

On every client connect, before reading any request, the backend pushes:

1. one `state_changed` event (full session state, below);
2. if `lifecycle` is `ready`: one `guilds_synced` event (full guild + DM structure,
   including per-channel unread/mention counts).

Thereafter events are pushed as things change. A client can force a refresh at any
time with `get_state` / `list_guilds`.

Both `state` and `guilds_synced` carry `generation`, one monotonically increasing
counter bumped on every state or structure change. A snapshot is a consistent cut at
the current generation, but an event produced *before* the snapshot may still be
delivered after it (events reach the socket asynchronously). Clients keep the highest
generation seen across both events and **discard any `state_changed` or
`guilds_synced` whose generation is lower than it**. Equal generations (the two
snapshot lines) are applied. A snapshot's two lines are never interleaved with other
events.

## Error codes (stable, machine-readable)

| code | meaning |
|---|---|
| `invalid_request` | request line failed to parse (answered with id 0) or parsed without a `command` (id echoed) |
| `unsupported_version` | `v != 1` |
| `unknown_command` | `command` is not (yet) implemented by this backend |
| `invalid_argument` | known command, missing/ill-typed field |
| `serialization_error` | internal failure serializing a result/snapshot |
| `not_logged_in` | no session exists (`starting`, `logged_out`, `error` without a token). `reauth_needed` keeps its cached session and does **not** produce this for read-only structure commands |
| `login_failed` | token rejected at login (invalid/revoked) |
| `qr_unavailable` | remote-auth gateway unreachable or flow already running |
| `gateway_unavailable` | a session exists but has not processed its first READY (`connecting` after login/start) — structure is not yet known; for write commands, also when the gateway is currently disconnected. Retry later |
| `unknown_guild` | guild id not in session state |
| `unknown_channel` | channel id not visible to this account |
| `unknown_message` | message id not found in the channel |
| `channel_not_open` | command requires a prior `open_channel` |
| `forbidden` | Discord denied the action (missing permission) |
| `rate_limited` | Discord rate limit; `message` includes retry hint |
| `empty_dm_refused` | refusing to open a DM with zero history (spam-flag guard) |
| `upload_too_large` | file exceeds the account/guild upload cap |
| `media_error` | media fetch/cache failure (disallowed host, HTTP error, disk) |
| `discord_error` | any other Discord API error (redacted detail in `message`) |
| `internal_error` | backend bug; redacted detail in `message` |

## Wire objects

### `state` (session state — carried by `state_changed`, returned by `get_state`)

| field | type | notes |
|---|---|---|
| `protocol_version` | int | `1` |
| `backend_version` | string | semver of the binary |
| `lifecycle` | string | `starting` · `logged_out` · `qr_pending` · `connecting` · `ready` · `reauth_needed` · `error` |
| `user` | object\|null | `{id, username, display_name, avatar_url}` — null until ready |
| `presence` | string | own status: `online` · `idle` · `dnd` · `invisible` — `""` until ready |
| `total_mention_count` | int | bar badge number (ningen `TotalMentionCount`); also updated by `read_state_changed` |
| `unread_dm_channel_id` | string\|null | most recent unread DM (bar middle-click target); null if none |
| `generation` | int | monotonically increasing; discard stale states |
| `error` | string | redacted human-readable detail when lifecycle is `error`/`reauth_needed`, else `""` |

`reauth_needed` semantics: entered when the gateway closes with a fatal code
(4004/4010–4014 — token invalid) or a REST 401. The backend drops the in-memory token,
clears the keyring entry, stays running, and keeps serving structure from cache
read-only (`list_guilds` / `list_channels` / `list_dms` succeed). Recovery is `login` or `start_qr_login`. A new `login` resets
`user`, `presence`, `total_mention_count`, and `unread_dm_channel_id` to their
pre-ready values (null / `""` / 0 / null) in the `connecting` state it emits, even
when it replaces a live session. Transient disconnects surface as
`connecting`, never `reauth_needed`; QML should apply a ~3 s grace before showing
reconnect UI.

### `guild`

`{id, name, icon_url (string|null, CDN), unread ("read"|"unread"|"mentioned"),
mention_count (int), position (int)}`

### `channel`

`{id, guild_id (string|null — null for DMs), type
("text"|"announcement"|"category"|"thread"|"forum"|"voice"|"dm"|"group_dm"),
name, topic (string), parent_id (string|null), position (int),
last_message_id (string|null), unread ("read"|"unread"|"mentioned"),
mention_count (int), muted (bool)}`

`recipients` (`[{id, username, display_name, avatar_url}]`) is always present: the
members for `dm`/`group_dm` (possibly empty), `[]` for guild channels. For DMs, `name`
is the recipient display name (group DMs: joined names or set name).

### `message`

| field | type | notes |
|---|---|---|
| `id` | string | |
| `channel_id` | string | |
| `guild_id` | string\|null | |
| `author` | object | `{id, username, display_name, avatar_url, bot}` — `display_name` is guild nick > global display name > username, resolved from the cache (a guild member not yet cached falls back to the global name); `avatar_url` is the CDN URL with `?size=64` appended |
| `content` | string | raw Discord markdown; QML renders it |
| `timestamp` | string | RFC 3339 UTC with milliseconds |
| `edited_timestamp` | string\|null | |
| `nonce` | string | `""` unless present. Official clients also send nonces, so other people's messages carry one too — dedup own optimistic rows by matching against the nonces *you* sent, never by mere presence |
| `reply_to` | object\|null | `{message_id, author_display_name, preview}` — null unless the message is an inline reply (forwards are not replies). `preview` is the referenced message's content collapsed to one line (≤120 runes; attachment filenames / `[embed]` / `[sticker]` when it has no text). When the referenced message is unknown (deleted or uncached) `message_id` is still set and `author_display_name`/`preview` are `""` |
| `attachments` | array | `[{id, filename, content_type, size, url, proxy_url, width, height, spoiler}]` — `url` is the CDN URL; fetch through `fetch_media`. `width`/`height` are 0 for non-images; `spoiler` is derived from the `SPOILER_` filename prefix |
| `embeds` | array | `[{type, title, description, url, image_url, thumbnail_url, color}]` (subset; enough to render). Absent parts are `""`; `color` is the 0xRRGGBB integer, 0 when unset |
| `reactions` | array | `[{emoji (string — unicode or "name:id"), count, me (bool)}]` — always an array (possibly empty) |
| `mentions_self` | bool | ningen `MessageMentions` has `MessageMentions` (explicit mention or unsuppressed `@everyone`; role mentions are not implemented by ningen) |
| `system` | bool | join/pin/boost/call/thread/stage etc. (every type other than default, inline reply, and slash/context commands). `content` is replaced by a rendered plain-text line that **includes the actor's display name** (e.g. `"Ada joined the server."`, `"Ada pinned a message."`, `"Ada added lin to the group."`), so QML renders system rows as a single line without the author header. Unknown types render `"<name> sent a system message (type N)."` |

`attachments`, `embeds`, and `reactions` are never null.

---

## Commands

### Session

#### `hello`
Answered in the socket layer without touching the session.
- Request: no fields.
- Result: `{"protocol_version":1,"backend_version":"0.1.0","engine":"arikawa"}`
- Errors: none.

```json
{"v":1,"id":1,"command":"hello"}
{"type":"response","v":1,"id":1,"ok":true,"result":{"protocol_version":1,"backend_version":"0.1.0","engine":"arikawa"}}
```

#### `ping`
- Result: `{"pong":true}`. Errors: none.

#### `get_state`
- Result: the full `state` object. Errors: `serialization_error`.

#### `login`
Log in with a pasted user token. The token is validated (REST `/users/@me`), written
to the keyring (stdin path), and held in memory; this request line is exempt from all
logging. Any existing session (live or `reauth_needed`) is replaced; concurrent
`login`/`logout` requests are serialized so exactly one session survives.
- Request: `{token: string}`
- Result: `{user: {id, username, display_name, avatar_url}, keyring_stored: bool}` —
  lifecycle proceeds `connecting` → `ready` via `state_changed` events.
  `keyring_stored: false` means the session is live for this process but the token
  could not be persisted (the user must log in again after a restart); the client
  should surface a warning.
- Errors: `invalid_argument`, `login_failed`, `qr_unavailable` (QR flow in progress).

```json
{"v":1,"id":4,"command":"login","token":"<redacted>"}
{"type":"response","v":1,"id":4,"ok":true,"result":{"user":{"id":"183627919046737920","username":"m","display_name":"m","avatar_url":"https://cdn.discordapp.com/avatars/..."},"keyring_stored":true}}
```

#### `logout`
Disconnect the gateway, drop the in-memory token, clear the keyring entry.
- Request: no fields. Result: `{}`. Lifecycle → `logged_out`.
- Errors: `not_logged_in`.

#### `set_presence`
- Request: `{status: "online"|"idle"|"dnd"|"invisible"}`
- Result: `{}` — sends the gateway presence update and PATCHes user settings so it
  persists across devices (ningen `SetStatus`).
- Errors: `invalid_argument`, `not_logged_in`, `gateway_unavailable`.

#### `start_qr_login`
Open Discord's remote-auth gateway and begin the QR flow. Progress arrives as events
(`qr_code`, `qr_scanned`, `qr_approved`, `qr_cancelled`); the command returns as soon
as the gateway session is established.
- Request: no fields. Result: `{}`. Lifecycle → `qr_pending`.
- Errors: `qr_unavailable` (gateway unreachable, or a QR flow / logged-in session
  already active).

#### `cancel_qr_login`
- Request: no fields. Result: `{}` — closes the remote-auth WS; emits
  `qr_cancelled {reason:"cancelled"}`; lifecycle → `logged_out`.
- Errors: `qr_unavailable` (no flow running).

### Structure

Structure commands read the session cache. They succeed in `ready` and in
`reauth_needed` (cached, read-only); before the first READY of a session they return
`gateway_unavailable`; with no session at all, `not_logged_in`.

#### `list_guilds`
- Result: `{guilds: [guild]}` sorted by user's guild order.
- Errors: `not_logged_in`, `gateway_unavailable`.

#### `list_channels`
- Request: `{guild_id: string}`
- Result: `{channels: [channel]}` — permission-filtered, empty categories removed,
  category-grouped display order (ningen `Channels(guildID, allowedTypes)`).
- Errors: `not_logged_in`, `gateway_unavailable`, `invalid_argument` (non-snowflake
  id), `unknown_guild`.

#### `list_dms`
- Result: `{channels: [channel]}` sorted by `last_message_id` desc (ningen
  `PrivateChannels`).
- Errors: `not_logged_in`, `gateway_unavailable`.

#### `quick_switch`
Fuzzy match over channels and DMs for the Ctrl+K switcher.
- Request: `{query: string, limit?: int (default 20)}`
- Result: `{entries: [{channel, guild_name (string|null), last_message_preview
  (string), score (number)}]}` — unread/mentioned entries rank first; empty query
  returns the unread set then recents.
- Errors: `not_logged_in`.

### Messages

#### `open_channel`
The load-bearing command: subscribes the guild (ningen `MemberState.Subscribe` — Op 14,
enables guild TypingStart), fetches the last ~50 messages (cache-aware), and starts
streaming `message_*` / `typing_start` events for this channel.
- Request: `{channel_id: string}`
- Result: `{channel: channel, messages: [message] (ascending by id, i.e.
  oldest→newest, at most 50), has_more: bool}` — `has_more` is true when a full
  50 were returned (a channel with exactly 50 messages reports `true` and the next
  `history` page comes back empty with `has_more: false`).
- Errors: `invalid_argument` (non-snowflake id), `not_logged_in`,
  `gateway_unavailable` (before the first READY), `unknown_channel`, `forbidden`
  (guild channel without View Channel), `empty_dm_refused` (1:1 DM with zero
  history — send one from the official client first; empty group DMs open fine),
  `discord_error` / `rate_limited` (the REST history fetch failed).
- The open set is **per connection**: each socket client tracks its own open
  channels, and they are all closed when that connection drops. Multiple channels
  may be open concurrently; opening an open channel re-sends the current tail
  (idempotent).

```json
{"v":1,"id":20,"command":"open_channel","channel_id":"1049931213073821696"}
{"type":"response","v":1,"id":20,"ok":true,"result":{"channel":{...},"messages":[{...},{...}],"has_more":true}}
```

#### `close_channel`
Stops event streaming for the channel (read-state tracking continues globally; the
guild subscription is retained).
- Request: `{channel_id: string}`. Result: `{}`.
- Errors: `invalid_argument`, `channel_not_open` (not open on *this* connection).

#### `history`
Page older messages. Serves from cache when it holds a full page older than
`before_id`, else REST `MessagesBefore` (deep pages bypass the 100-message cache
deliberately).
- Request: `{channel_id: string, before_id: string, limit?: int (default 50, max 100;
  out-of-range values are clamped)}`
- Result: `{messages: [message] (ascending), has_more: bool}` — `has_more` is
  `len(messages) == limit`; `false` means start of channel history (the page may
  still be non-empty — it is the last one).
- Errors: `invalid_argument`, `channel_not_open` (this connection must have opened
  it), `not_logged_in`, `gateway_unavailable`, `unknown_channel`, `forbidden`,
  `rate_limited`, `discord_error`.

#### `send`
- Request: `{channel_id: string, content: string, reply_to?: string (message id),
  reply_mention?: bool (default true)}`
- Result: `{message_id: string, nonce: string}` — the backend generates the nonce and
  sets it on `SendMessageData`; the gateway echo (`message_create` carrying the same
  `nonce`) is the QML-side dedup for optimistic rows.
- Errors: `channel_not_open`, `invalid_argument` (empty content, over length limit),
  `forbidden`, `rate_limited`, `discord_error`.

```json
{"v":1,"id":31,"command":"send","channel_id":"1049931213073821696","content":"on my way","reply_to":"1049931302442426390"}
{"type":"response","v":1,"id":31,"ok":true,"result":{"message_id":"1049931339989602304","nonce":"a1b2c3d4"}}
```

#### `edit`
Own messages only.
- Request: `{channel_id: string, message_id: string, content: string}`
- Result: `{}` — the updated row arrives as `message_update`.
- Errors: `channel_not_open`, `unknown_message`, `forbidden`, `discord_error`.

#### `delete`
- Request: `{channel_id: string, message_id: string}`
- Result: `{}` — row removal is driven by the `message_delete` event, not the response.
- Errors: `channel_not_open`, `unknown_message`, `forbidden`, `discord_error`.

#### `react` / `unreact`
- Request: `{channel_id: string, message_id: string, emoji: string}` — unicode emoji
  or `"name:id"` for custom. The backend strips U+FE0F variation selectors before the
  REST call (they 400 otherwise).
- Result: `{}` — reaction state updates arrive via `message_update`.
- Errors: `channel_not_open`, `unknown_message`, `invalid_argument`, `forbidden`,
  `discord_error`.

#### `typing`
Broadcast own typing. The backend throttles to one Discord call per 10 s per channel;
QML may call freely on keystrokes.
- Request: `{channel_id: string}`. Result: `{}` (also when throttled).
- Errors: `channel_not_open`.

### Read state

#### `ack`
Mark a channel read up to a message. Calls ningen `ReadState.MarkRead` (dedupes;
sends the REST ack only for non-self messages). The message **must be in the
backend's cache** — i.e. it arrived via `open_channel`'s tail or a `message_create`
— otherwise the command is refused rather than silently moving only the local
marker. Does not require the channel to be open on this connection.
- Request: `{channel_id: string, message_id: string}`
- Result: `{}` — the resulting change arrives as `read_state_changed` (and a
  `state_changed` if `total_mention_count` changed). Acking an already-read message
  is a no-op that produces no event.
- Errors: `invalid_argument`, `not_logged_in`, `gateway_unavailable`,
  `unknown_channel`, `unknown_message` (not cached — open the channel first).

### Media

#### `fetch_media`
Fetch a Discord CDN URL into the media cache and return a local path. Only
`cdn.discordapp.com` and `media.discordapp.net` are allowed. Cache lives at
`$XDG_CACHE_HOME/omarchy-discord/media/` with an LRU cap (`mediaCacheMB` setting).
- Request: `{url: string, size?: int (power-of-two hint appended as ?size= for
  avatars/emoji)}`
- Result — cache hit: `{cached: true, path: "/home/…/media/ab12…"}`; miss:
  `{cached: false}` with completion pushed later as a `media_ready` event keyed by
  `url`.
- Errors: `invalid_argument`, `media_error` (disallowed host, immediately).

#### `upload`
Send a message with file attachments (the clipboard-paste pipeline). Streams the
files; progress is pushed as `upload_progress` events carrying this request's `id`.
The response arrives on completion.
- Request: `{channel_id: string, paths: [string], content?: string,
  reply_to?: string, spoiler?: bool}`
- Result: `{message_id: string, nonce: string}`
- Errors: `channel_not_open`, `invalid_argument` (unreadable path),
  `upload_too_large` (checked against `DetermineUploadSize` before sending),
  `forbidden`, `rate_limited`, `discord_error`.

```json
{"v":1,"id":44,"command":"upload","channel_id":"1049931213073821696","paths":["/run/user/1000/omarchy-discord/staged/shot-1.png"],"content":"look at this"}
{"type":"event","v":1,"event":"upload_progress","upload_id":44,"filename":"shot-1.png","bytes_sent":262144,"bytes_total":1048576}
{"type":"event","v":1,"event":"upload_progress","upload_id":44,"filename":"shot-1.png","bytes_sent":1048576,"bytes_total":1048576}
{"type":"response","v":1,"id":44,"ok":true,"result":{"message_id":"1049931401540221011","nonce":"e5f6a7b8"}}
```

---

## Events

Ordering: all events are serialized through a single writer goroutine; within one
connection, event order is the order the backend processed them (ningen's
`read.UpdateEvent` and gateway events are funneled, never raced onto the socket).

### `state_changed`
`{state: state}` — full session-state object (§ wire objects). Fires on connect
(snapshot), on every lifecycle/user/presence/total-mention change. Unchanged states
are not re-sent. This is the only event a client is guaranteed before `ready`.

### `guilds_synced`
`{generation: int, guilds: [guild], dms: [channel]}` — the complete structure. Fires
after READY processing completes (including reconnect/resume), and on guild
join/leave/reorder; each such change bumps the shared state generation and
`generation` carries the new value. QML replaces its whole structure mirror on
receipt, unless `generation` is lower than the highest already seen (§ Snapshot on
connect).

### `channel_update`
`{change: "create"|"update"|"delete", channel: channel}` — channel/thread created,
renamed, reordered, or deleted anywhere in the session (`channel.id` alone is
meaningful for `delete`). Fires from gateway Channel*/Thread* events.

### `message_create`
`{channel_id, guild_id (string|null), message: message, notify: bool,
channel_name: string}`
Fires for (a) every new message in a channel **this connection has open**, and (b)
any message anywhere whose ningen `MessageMentions` flags include `MessageNotifies`
(delivered to every connection) — so Service.qml can raise a desktop notification
(`notify: true`, with `channel_name` and `message.author` supplying the notification
title) without every channel being open. `notify` follows Discord's own
notification settings as ningen evaluates them: DMs, explicit mentions, unsuppressed
`@everyone`, **and every message in a guild/channel set to "All messages"**; muted
channels/guilds and "Nothing" never notify; own messages never notify. A message
that is both in an open channel and notifying arrives exactly once with
`notify: true`. The plugin-level All/Mentions/Off filter and open-channel
suppression are applied in QML before `notify-send` (use `message.mentions_self` to
tell a mention apart from an "All messages" notify). `channel_name` is the sidebar
name (DMs: the recipient's display name). Own sent messages echo here with `nonce`
set — re-key the optimistic row, don't append (match on your own pending nonces;
other people's messages carry nonces too).

### `message_update`
`{channel_id, guild_id, message: message}` — edits, embed resolution, and reaction
changes (add / remove / remove-all / remove-emoji are folded into this event) for
open channels. The `message` is the full updated object rebuilt from the cache;
replace in place. A partial MESSAGE_UPDATE for a message that is no longer cached
(older than the 100-message cache) is dropped; reactions on uncached messages are
likewise dropped.

### `message_delete`
`{channel_id, guild_id, message_id: string}` — open channels only. Remove the row.
A bulk delete arrives as one `message_delete` per id.

### `typing_start`
`{channel_id, guild_id (string|null), user_id: string, display_name: string,
timestamp: string}` — open channels only. Guild channels emit this only because
`open_channel` subscribed the guild. `display_name` is resolved from the event's
member, the member cache, or the DM recipient list and is `""` when unknown.
`timestamp` is the RFC 3339 start time. QML expires typers after 10 s and clears a
typer on their `message_create`.

### `read_state_changed`
`{channel_id, guild_id (string|null), unread: bool, mention_count: int,
last_read_message_id: string|null, total_mention_count: int}`
Fires on every ningen `read.UpdateEvent` (delivered to every connection, regardless
of open channels): new messages anywhere, local `ack`, and acks from other devices
(MESSAGE_ACK). `unread: true` with `mention_count: 0` is a plain unread; the
"mentioned" indication is `mention_count > 0`. The bar badge is a pure reduction of
these (`total_mention_count` is precomputed for convenience and matches
`state.total_mention_count`); a `state_changed` follows **only** when the total
actually changed. Because ningen raises these on its own goroutine, a
`read_state_changed` for a new message may arrive before or after that message's
`message_create`.

### `media_ready`
`{url: string, ok: bool, path: string ("" on failure), error: string ("" on
success, redacted)}` — completion of a `fetch_media` cache miss. Keyed by `url`;
multiple pending requests for the same URL coalesce into one event.

### `upload_progress`
`{upload_id: int (the originating request id), filename: string, bytes_sent: int,
bytes_total: int}` — emitted from a counting reader wrapping each file as the
multipart body streams; final event has `bytes_sent == bytes_total`. Frequency capped
(≥100 ms between events per upload).

### QR login events

#### `qr_code`
`{url: string, fingerprint: string, expires_in_ms: int}` — the remote-auth gateway
issued a fingerprint; `url` is `https://discord.com/ra/<fingerprint>`, which the panel
renders as a QR. Fires after `start_qr_login`, and again with a fresh code if the
backend reconnects after expiry while the flow is still wanted.

#### `qr_scanned`
`{user: {id, username, discriminator, avatar_hash}}` — the phone scanned the code
(remote-auth `pending_ticket`, decrypted user payload). Show "logging in as X —
confirm on your phone".

#### `qr_approved`
`{}` — the user confirmed on the phone; the backend exchanged the ticket
(`ExchangeRemoteAuthTicket`), decrypted the token, stored it in the keyring, and is
connecting. Followed by `state_changed` events (`connecting` → `ready`). The token
itself never appears on the socket.

#### `qr_cancelled`
`{reason: "declined"|"expired"|"cancelled"|"error", error: string (redacted, ""
unless reason is "error")}` — the flow ended without login. Lifecycle returns to
`logged_out`. `expired` fires when the hello `timeout_ms` (~2 min) lapses without a
scan and the client did not keep the flow alive; call `start_qr_login` again for a
fresh code.

---

## Event → source mapping (implementation reference)

| Event | Backed by |
|---|---|
| `state_changed` | `ningen.ConnectedEvent` / `DisconnectedEvent` (fatal-code check via `IsLoggedOut()`), login/logout, `SetStatus`, `TotalMentionCount` deltas |
| `guilds_synced` | ningen post-READY (`Open` returned / `ConnectedEvent`), GuildCreate/Delete |
| `channel_update` | `ChannelCreateEvent`/`ChannelUpdateEvent`/`ChannelDeleteEvent`, `ThreadCreateEvent` etc. |
| `message_create/update/delete` | `MessageCreateEvent`/`MessageUpdateEvent`/`MessageDeleteEvent`/`MessageDeleteBulkEvent` + `MessageReaction{Add,Remove,RemoveAll,RemoveEmoji}` (folded into `message_update` from the cache); per-connection routing in the socket layer (`socket.Routed`) |
| `typing_start` | `TypingStartEvent` (guilds require the Op 14 subscribe from `open_channel`) |
| `read_state_changed` | ningen `read.UpdateEvent` (async goroutine — serialized into the writer) |
| `media_ready` / `upload_progress` | backend media cache / counting reader |
| `qr_*` | ported remote-auth gateway client (discordo protocol) + `ExchangeRemoteAuthTicket` |
