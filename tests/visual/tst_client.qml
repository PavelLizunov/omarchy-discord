import QtQuick
import QtTest
import "../../" as Discord
import "../../components/harness" as Harness
import "../../ui"

TestCase {
  id: tests
  name: "TextClient"
  width: 1040
  height: 680
  when: windowShown
  Preview { id: preview; anchors.fill: parent }
  function press(key, text, modifiers) {
    var event = {key:key,text:text || "",modifiers:modifiers || Qt.NoModifier,accepted:false}
    preview.client.dispatchKey(event)
    return event.accepted
  }
  function initTestCase() { tryCompare(preview, "ready", true) }
  function init() {
    preview.model.reset()
    preview.model.currentChannelId = "9000"
    preview.client.zone = "timeline"
    preview.client.timeline.focusNewest()
    preview.client.focusZone()
  }
  function test_ack_requires_visible_active_window() {
    preview.client.mapped = true
    preview.client.windowActive = true
    preview.client.timeline.reachedBottom()
    wait(600)
    compare(preview.model.callCount("markChannelRead"), 1)
    preview.model.reset()
    preview.client.windowActive = false
    preview.client.timeline.reachedBottom()
    wait(600)
    compare(preview.model.callCount("markChannelRead"), 0)
    preview.client.windowActive = true
    preview.client.mapped = false
    preview.client.timeline.reachedBottom()
    wait(600)
    compare(preview.model.callCount("markChannelRead"), 0)
    preview.client.mapped = true
    preview.client.timeline.reachedBottom()
    preview.client.windowActive = false
    wait(600)
    compare(preview.model.callCount("markChannelRead"), 0)
    preview.client.windowActive = true
    wait(600)
    compare(preview.model.callCount("markChannelRead"), 1)
  }
  function test_copy_and_navigation() {
    verify(press(0,"Y"))
    verify(preview.copiedText.length > 0)
    var before = preview.client.timeline.cursorMessageId
    verify(press(Qt.Key_Up,""))
    verify(preview.client.timeline.cursorMessageId !== before)
    verify(press(Qt.Key_Escape,""))
    compare(preview.client.zone,"sidebar")
    compare(preview.client.column,"channels")
    verify(press(Qt.Key_Escape,""))
    compare(preview.client.column,"rail")
    verify(preview.client.opened)
  }
  function test_reply_edit_send() {
    preview.client.composer.startReply("1067","Ada")
    verify(preview.client.composer.replying)
    preview.client.composer.setText("Fixture reply")
    verify(preview.client.composer.submit())
    compare(preview.model.lastCall("sendMessage"),"Fixture reply")
    compare(preview.client.composer.text,"")
    verify(!preview.client.composer.replying)
    verify(preview.client.composer.startEditLast())
    preview.client.composer.setText("Fixture edit")
    verify(preview.client.composer.submit())
    compare(preview.model.lastCall("editMessage"),"Fixture edit")
    verify(!preview.client.composer.editing)
  }
  function test_voice_chords() {
    verify(press(Qt.Key_M,"",Qt.ControlModifier | Qt.ShiftModifier))
    compare(preview.model.callCount("toggleMute"),1)
    verify(press(Qt.Key_D,"",Qt.ControlModifier | Qt.ShiftModifier))
    compare(preview.model.callCount("toggleDeafen"),1)
    verify(press(Qt.Key_H,"",Qt.ControlModifier | Qt.ShiftModifier))
    compare(preview.model.callCount("voiceLeave"),1)
  }
  function test_members_and_close() {
    preview.client.toggleMembers()
    verify(preview.model.membersWanted)
    preview.client.toggleMembers()
    verify(!preview.model.membersWanted)
    preview.client.requestClose()
    verify(!preview.client.opened)
    preview.client.open("{}")
    verify(preview.client.opened)
  }
  function test_login_actions() {
    preview.model.showStructure = false
    preview.model.lifecycle = "logged_out"
    preview.client.startQrLogin()
    compare(preview.model.callCount("startQrLogin"),1)
    preview.model.qr = {stage:"code",expiresAt:Date.now()+120000}
    preview.model.lifecycle = "qr_pending"
    preview.client.leaveQr()
    compare(preview.model.callCount("cancelQrLogin"),1)
    preview.model.qr = null
    preview.model.lifecycle = "ready"
    preview.model.showStructure = true
  }
  function test_bounded_geometry() {
    tests.width = 640
    tests.height = 420
    wait(30)
    verify(preview.client.timeline.width > 200)
    verify(preview.client.timeline.height > 100)
    verify(preview.client.composer.height > 0)
    tests.width = 1040
    tests.height = 680
  }
  function test_spoiler_and_attachment_link() {
    var timeline = preview.client.timeline
    var rows = preview.model.channelData["9000"].messages
    var old = rows
    preview.model.channelData = {"9000":{channel:preview.model.channelsByGuild["1"][0],messages:[{id:"99",content:"",author:{id:"200",username:"fixture"},timestamp:"2026-09-30T12:00:00Z",attachments:[{url:"https://cdn.discordapp.com/fixture.png",filename:"fixture.png",size:1234,spoiler:true,content_type:"image/png"}]}],loading:false}}
    wait(30)
    timeline.focusNewest()
    timeline.activateCursor()
    verify(timeline.revealed["99"])
    timeline.copyCursorLink()
    compare(preview.copiedText,"https://cdn.discordapp.com/fixture.png")
    timeline.openCursorLink()
    compare(preview.openedLink,"https://cdn.discordapp.com/fixture.png")
    preview.model.channelData = {"9000":{channel:preview.model.channelsByGuild["1"][0],messages:old,loading:false}}
  }
}
