const test = require("node:test")
const assert = require("node:assert/strict")
const M = require("../../Markdown.js")

const ctx = {
  users: { "1": "ada", "2": "lin" },
  channels: { "9": "general" },
  roles: { "5": "mods" },
  selfId: "1",
  mentionColor: "#m",
  mentionBg: "#mb",
  linkColor: "#l",
  codeBg: "#c",
  spoilerColor: "#s",
  mutedColor: "#u",
  monoFamily: "mono",
  fontSize: 12,
  now: 1700003600 * 1000
}

const r = (s) => M.render(s, ctx)
const p = (s) => M.plainText(s, ctx)

test("inline formatting", () => {
  assert.equal(r("**b**"), "<b>b</b>")
  assert.equal(r("*i* _i_"), "<i>i</i> <i>i</i>")
  assert.equal(r("***bi***"), "<b><i>bi</i></b>")
  assert.equal(r("__u__"), "<u>u</u>")
  assert.equal(r("~~s~~"), "<s>s</s>")
  assert.equal(r("snake_case_word"), "snake_case_word")
  assert.equal(r("a * b * c"), "a <i> b </i> c")
})

test("code", () => {
  assert.equal(r("`x<y`"), "<code style=\"font-family:'mono';background-color:#c;\">x&lt;y</code>")
  assert.equal(r("`**not bold**`"), "<code style=\"font-family:'mono';background-color:#c;\">**not bold**</code>")
  assert.equal(r("```js\nlet a = 1;\n```"), "<pre style=\"font-family:'mono';background-color:#c;white-space:pre-wrap;\">let a = 1;</pre>")
  assert.equal(r("```\n<b>\n```"), "<pre style=\"font-family:'mono';background-color:#c;white-space:pre-wrap;\">&lt;b&gt;</pre>")
  assert.equal(r("```\n  a\n  b\n```"), "<pre style=\"font-family:'mono';background-color:#c;white-space:pre-wrap;\">  a\n  b</pre>")
  assert.equal(r("||```\nx\n```||"),
    "<span style=\"background-color:#s;color:#s\"><pre style=\"font-family:'mono';white-space:pre-wrap;background-color:#s;color:#s\">x</pre></span>")
  assert.equal(p("```js\nlet a = 1;\n```"), "let a = 1;")
})

test("blocks: quote, headers, lists, line breaks", () => {
  assert.equal(r("> q\nn"), "<span style=\"color:#u\">&#9613;</span> q<br>n")
  assert.equal(r(">>> a\nb"), "<span style=\"color:#u\">&#9613;</span> a<br><span style=\"color:#u\">&#9613;</span> b")
  assert.equal(r("# H"), "<span style=\"font-size:17px\"><b>H</b></span>")
  assert.equal(r("### H"), "<span style=\"font-size:13px\"><b>H</b></span>")
  assert.equal(r("- a\n- b"), "&#8226; a<br>&#8226; b")
  assert.equal(r("a\r\nb"), "a<br>b")
  assert.equal(p("# H\n- a"), "H - a")
})

test("spoilers", () => {
  assert.equal(r("||x||"), "<span style=\"background-color:#s;color:#s\">x</span>")
  assert.equal(M.render("||x||", {}), "<s>x</s>")
  assert.equal(p("||x||"), "x")
})

test("spoilers cover what they wrap", () => {
  const cover = (inner) => `<span style="background-color:#s;color:#s">${inner}</span>`
  assert.equal(r("||<@1>||"), cover("<span style=\"background-color:#s;color:#s\"><b>@ada</b></span>"))
  assert.equal(r("||[t](https://x.y)||"), cover("<a href=\"https://x.y\" style=\"background-color:#s;color:#s\">t</a>"))
  assert.equal(r("||https://x.y/a||"), cover("<a href=\"https://x.y/a\" style=\"background-color:#s;color:#s\">https://x.y/a</a>"))
  assert.equal(r("||`c`||"), cover("<code style=\"font-family:'mono';background-color:#s;color:#s\">c</code>"))
  assert.equal(r("<@1> ||x||"), "<span style=\"color:#m;background-color:#mb;\"><b>@ada</b></span> " + cover("x"))
  assert.equal(r("**||a||**"), "<b>" + cover("a") + "</b>")
  assert.equal(r("||**a**||"), cover("<b>a</b>"))
  assert.equal(r("||`**x**`||"), cover("<code style=\"font-family:'mono';background-color:#s;color:#s\">**x**</code>"))
  assert.equal(p("||<@1> [t](https://x.y)||"), "@ada t")
})

test("mentions and emoji", () => {
  const chip = (t) => `<span style="color:#m;background-color:#mb;"><b>${t}</b></span>`
  assert.equal(r("<@1>"), chip("@ada"))
  assert.equal(r("<@!2>"), chip("@lin"))
  assert.equal(r("<@3>"), chip("@3"))
  assert.equal(r("<#9>"), chip("#general"))
  assert.equal(r("<@&5>"), chip("@mods"))
  assert.equal(r("<:smile:123> <a:wave:4>"), ":smile: :wave:")
  assert.equal(p("hi <@1> in <#9>"), "hi @ada in #general")
  assert.equal(M.render("<@7>", { selfId: "7" }), "<b>@you</b>")
})

test("custom emoji images", () => {
  const paths = { "123": "/home/m/.cache/omarchy-discord/media/ab.png" }
  const ectx = Object.assign({}, ctx, { emojiSize: 17, emojiPath: (id) => paths[id] || "" })
  const img = "<img src=\"file:///home/m/.cache/omarchy-discord/media/ab.png\" width=\"17\" height=\"17\" alt=\":smile:\">"
  assert.equal(M.render("<:smile:123>", ectx), img)
  assert.equal(M.render("<a:smile:123>", ectx), img)
  assert.equal(M.render("<:wave:4>", ectx), ":wave:")
  assert.equal(M.plainText("<:smile:123>", ectx), ":smile:")
  assert.equal(M.render("<:smile:123>", { emojiPath: () => "/a.png" }),
    "<img src=\"file:///a.png\" width=\"17\" height=\"17\" alt=\":smile:\">")
  assert.equal(M.render("**<:smile:123>**", ectx), "<b>" + img + "</b>")
  assert.equal(M.render("||<:smile:123>||", ectx),
    "<span style=\"background-color:#s;color:#s\">" + img + "</span>")
  let seen = null
  M.render("<:x:999>", Object.assign({}, ectx, { emojiPath: (id, animated) => { seen = [id, animated]; return "" } }))
  assert.deepEqual(seen, ["999", false])
  M.render("<a:x:999>", Object.assign({}, ectx, { emojiPath: (id, animated) => { seen = [id, animated]; return "" } }))
  assert.deepEqual(seen, ["999", true])
})

test("emoji path cannot inject markup", () => {
  const bad = ["\"><script>x</script>", "/a.png\" onload=\"x", "/a.png'>", "/a<b>.png", "/a&b.png",
    "relative.png", "file:///a.png", "http://evil/x.png", "/a\\b.png", "/a\n.png", "", null, undefined, 42]
  for (const p of bad) {
    const out = M.render("<:smile:123>", { emojiPath: () => p })
    assert.equal(out, ":smile:", JSON.stringify(p))
  }
  const thrown = M.render("<:smile:123>", { emojiPath: () => { throw new Error("boom") } })
  assert.equal(thrown, ":smile:")
  assert.equal(M.render("<:smile:123>", { emojiPath: "not a function" }), ":smile:")
  assert.equal(M.render("<:smile:123>", { emojiPath: () => "/a b/c.png" }),
    "<img src=\"file:///a b/c.png\" width=\"17\" height=\"17\" alt=\":smile:\">")
  assert.equal(M.render("<:<b>:123>", { emojiPath: () => "/a.png" }), "&lt;:&lt;b&gt;:123&gt;")
})

test("timestamps", () => {
  assert.equal(r("<t:1700000000:R>"), "<u>1 hour ago</u>")
  assert.equal(r("<t:1700007200:R>"), "<u>in 1 hour</u>")
  assert.match(r("<t:1700000000:t>"), /^<u>\d\d:\d\d<\/u>$/)
  assert.match(r("<t:1700000000:F>"), /^<u>\w+day, \d+ November 2023 \d\d:\d\d<\/u>$/)
  assert.match(r("<t:1700000000>"), /^<u>\d+ November 2023 \d\d:\d\d<\/u>$/)
  assert.equal(r("<t:abc>"), "&lt;t:abc&gt;")
})

test("links", () => {
  assert.equal(r("see https://ex.com/a_b?x=1&y=2."),
    "see <a href=\"https://ex.com/a_b?x=1&amp;y=2\" style=\"color:#l\">https://ex.com/a_b?x=1&amp;y=2</a>.")
  assert.equal(r("[t](https://x.y/z_(1))"), "<a href=\"https://x.y/z_(1)\" style=\"color:#l\">t</a>")
  assert.equal(p("[t](https://x.y) https://a.b/"), "t https://a.b/")
  assert.equal(r("[x](javascript:alert(1))"), "[x](javascript:alert(1))")
})

test("html escaping everywhere", () => {
  assert.equal(r("<script>x</script> & \"q\""), "&lt;script&gt;x&lt;/script&gt; &amp; &quot;q&quot;")
  assert.equal(r("**<b>**"), "<b>&lt;b&gt;</b>")
  assert.equal(r("[<i>x</i>](https://a.b)"), "<a href=\"https://a.b\" style=\"color:#l\">&lt;i&gt;x&lt;/i&gt;</a>")
  assert.equal(r("<@1> <b>"), "<span style=\"color:#m;background-color:#mb;\"><b>@ada</b></span> &lt;b&gt;")
  assert.equal(M.render("<@1>", { users: { "1": "<img>" } }),
    "<b>@&lt;img&gt;</b>")
  assert.equal(p("<b>&amp;</b>"), "<b>&amp;</b>")
  assert.equal(r("\u00010\u0002"), "0")
  assert.equal(r("a \u0001 1 \u0002 b"), "a  1  b")
})

test("never throws on garbage", () => {
  const garbage = [null, undefined, 0, 1e9, true, {}, [], () => {}, "", "```", "``", "**", "*", "||", "__",
    "<@", "<@>", "<#", "<t:>", "<t:99999999999999999999:R>", "[x](", "[](https://)", "> ", ">>> ",
    "#", "- ", "\n\n\n", "```a```b```", "*".repeat(5000), "`".repeat(5001), "<".repeat(1000),
    "\u0000\u0001\u0002\uffff", "😀".repeat(300), "[".repeat(2000) + "](h".repeat(100)]
  for (const g of garbage) {
    assert.equal(typeof M.render(g, ctx), "string")
    assert.equal(typeof M.render(g, null), "string")
    assert.equal(typeof M.plainText(g, ctx), "string")
    assert.equal(typeof M.firstLink(g), "string")
  }
  assert.equal(typeof M.render("x", { users: null, now: "bad", fontSize: "nope" }), "string")
})

test("firstLink", () => {
  assert.equal(M.firstLink({ content: "a [m](https://m.x) https://b.x" }), "https://m.x")
  assert.equal(M.firstLink({ content: "https://b.x, then [m](https://m.x)" }), "https://b.x")
  assert.equal(M.firstLink({ content: "nothing", attachments: [{ url: "https://att" }] }), "https://att")
  assert.equal(M.firstLink({ content: "", attachments: [], embeds: [{ url: "https://emb" }] }), "https://emb")
  assert.equal(M.firstLink({ content: "none" }), "")
  assert.equal(M.firstLink(null), "")
})

test("formatSize", () => {
  assert.equal(M.formatSize(512), "512 B")
  assert.equal(M.formatSize(348211), "340.0 KB")
  assert.equal(M.formatSize(5 * 1024 * 1024), "5.0 MB")
  assert.equal(M.formatSize("x"), "0 B")
})
