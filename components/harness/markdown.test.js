// Unit test for Markdown.js. Run: node --test components/harness/markdown.test.js
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
  assert.equal(r("```js\nlet a = 1;\n```"), "<pre style=\"font-family:'mono';background-color:#c;\">let a = 1;</pre>")
  assert.equal(r("```\n<b>\n```"), "<pre style=\"font-family:'mono';background-color:#c;\">&lt;b&gt;</pre>")
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
  // placeholder control characters in input cannot address the stash
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
