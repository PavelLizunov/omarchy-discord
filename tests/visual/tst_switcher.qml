import QtQuick
import QtTest
import "../../" as Discord
import "../../components/harness" as Harness

TestCase {
  name: "TextSwitcher"
  when: windowShown
  width: 900
  height: 600
  Harness.MockService {
    id: model
    property bool ready: true
    property bool loggedOut: false
    function quickSwitch(query, callback) {
      callback([{channel:{id:"9000",guild_id:"1",name:"general",type:"text"},guild_name:"Fixture server",last_message_preview:"hello"}],"")
    }
    function openPanel(payload) { note("openPanel",payload.channel_id) }
  }
  Discord.SwitcherView { id: view; anchors.fill: parent; service:model }
  function test_search_and_activate() {
    view.open()
    compare(view.rows.length,1)
    view.setQuery("general")
    tryCompare(view,"busy",false)
    view.activate(0)
    compare(model.lastCall("openPanel"),"9000")
    verify(!view.opened)
  }
  function test_escape() {
    view.open()
    view.setQuery("fixture")
    var event={key:Qt.Key_Escape,modifiers:Qt.NoModifier,accepted:false}
    view.handleKey(event)
    compare(view.query,"")
    verify(view.opened)
    view.handleKey(event)
    verify(!view.opened)
  }
}
