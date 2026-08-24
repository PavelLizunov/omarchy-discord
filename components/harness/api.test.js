// Unit test for the pure Api.js helpers behind the per-guild last-visited
// channel. Run: node --test components/harness/api.test.js
const test = require("node:test")
const assert = require("node:assert/strict")
const Api = require("../../Api.js")

const ch = (id, name, type) => ({ id: id, name: name, type: type || "text" })

test("parseLastChannels tolerates garbage", () => {
  assert.deepEqual(Api.parseLastChannels(undefined), [])
  assert.deepEqual(Api.parseLastChannels(""), [])
  assert.deepEqual(Api.parseLastChannels("not json"), [])
  assert.deepEqual(Api.parseLastChannels("{}"), [])
  assert.deepEqual(Api.parseLastChannels("null"), [])
  assert.deepEqual(Api.parseLastChannels('[null,{"g":"1"},{"c":"2"},{"g":"1","c":"2"}]'),
    [{ g: "1", c: "2" }])
  // Numeric snowflakes survive as strings (JSON would lose 64-bit precision).
  assert.deepEqual(Api.parseLastChannels('[{"g":1,"c":2}]'), [{ g: "1", c: "2" }])
})

test("parseLastChannels truncates at the cap", () => {
  const raw = []
  for (let i = 0; i < Api.LAST_CHANNEL_CAP + 10; i++) raw.push({ g: "g" + i, c: "c" + i })
  assert.equal(Api.parseLastChannels(JSON.stringify(raw)).length, Api.LAST_CHANNEL_CAP)
})

test("bumpLastChannel returns the same reference when nothing moved", () => {
  const list = [{ g: "1", c: "10" }, { g: "2", c: "20" }]
  assert.equal(Api.bumpLastChannel(list, "1", "10"), list)
  // A different channel in the same guild is a move, not a no-op.
  assert.notEqual(Api.bumpLastChannel(list, "1", "11"), list)
  // Missing ids never write.
  assert.equal(Api.bumpLastChannel(list, "", "10"), list)
  assert.equal(Api.bumpLastChannel(list, "1", ""), list)
})

test("bumpLastChannel is a per-guild LRU", () => {
  let list = []
  list = Api.bumpLastChannel(list, "1", "10")
  list = Api.bumpLastChannel(list, "2", "20")
  list = Api.bumpLastChannel(list, "1", "11")
  assert.deepEqual(list, [{ g: "1", c: "11" }, { g: "2", c: "20" }])
  assert.equal(Api.lastChannelFor(list, "1"), "11")
  assert.equal(Api.lastChannelFor(list, "2"), "20")
  assert.equal(Api.lastChannelFor(list, "3"), "")
  assert.equal(Api.lastChannelFor(null, "1"), "")
})

test("bumpLastChannel evicts the least recently used guild", () => {
  let list = []
  for (let i = 0; i < Api.LAST_CHANNEL_CAP; i++) list = Api.bumpLastChannel(list, "g" + i, "c" + i)
  assert.equal(list.length, Api.LAST_CHANNEL_CAP)
  assert.equal(list[list.length - 1].g, "g0")
  list = Api.bumpLastChannel(list, "new", "cnew")
  assert.equal(list.length, Api.LAST_CHANNEL_CAP)
  assert.equal(list[0].g, "new")
  assert.equal(Api.lastChannelFor(list, "g0"), "")
})

const guild = [
  ch("cat", "Text Channels", "category"),
  ch("1", "announcements", "announcement"),
  ch("2", "voice-lounge", "voice"),
  ch("3", "General"),
  ch("4", "random"),
  ch("5", "help-forum", "forum"),
  ch("6", "stage", "stage"),
  { id: "7", name: "a thread", type: "thread", parent_id: "1" }
]

test("guildEntryChannel prefers the remembered channel", () => {
  assert.equal(Api.guildEntryChannel(guild, "4", true), "4")
  // A remembered thread resolves: threads are only in the raw list.
  assert.equal(Api.guildEntryChannel(guild, "7", true), "7")
})

test("guildEntryChannel falls through a stale or unopenable memory", () => {
  assert.equal(Api.guildEntryChannel(guild, "does-not-exist", true), "3")
  assert.equal(Api.guildEntryChannel(guild, "2", true), "3", "voice is not openable")
  assert.equal(Api.guildEntryChannel(guild, "5", true), "3", "a forum is not openable")
  assert.equal(Api.guildEntryChannel(guild, "cat", true), "3", "a category is not openable")
})

test("guildEntryChannel defaults to general, case-insensitively", () => {
  assert.equal(Api.guildEntryChannel(guild, "", true), "3")
  assert.equal(Api.guildEntryChannel([ch("1", "zzz"), ch("2", "GENERAL")], "", true), "2")
})

test("guildEntryChannel defaults to the first openable row without a general", () => {
  const rows = [ch("cat", "Cat", "category"), ch("2", "voice", "voice"), ch("1", "announcements", "announcement"),
    ch("4", "random")]
  assert.equal(Api.guildEntryChannel(rows, "", true), "1")
})

test("guildEntryChannel never defaults to a thread", () => {
  const rows = [{ id: "7", name: "a thread", type: "thread", parent_id: "1" }, ch("8", "chat")]
  assert.equal(Api.guildEntryChannel(rows, "", true), "8")
  // Threads only — nothing to open by default.
  assert.equal(Api.guildEntryChannel([rows[0]], "", true), "")
})

test("guildEntryChannel opens nothing by default when defaults are off (DMs)", () => {
  const dms = [ch("d1", "ada"), ch("d2", "lin")]
  assert.equal(Api.guildEntryChannel(dms, "", false), "")
  assert.equal(Api.guildEntryChannel(dms, "d2", false), "d2")
  assert.equal(Api.guildEntryChannel(dms, "gone", false), "")
})

test("guildEntryChannel survives an empty or missing list", () => {
  assert.equal(Api.guildEntryChannel([], "1", true), "")
  assert.equal(Api.guildEntryChannel(null, "", true), "")
})
