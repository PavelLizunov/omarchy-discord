import QtQuick
import "../../" as Discord
import "../../ui"
import "../../components/harness" as Harness

Rectangle {
  id: root
  width: 900
  height: 600
  color: Color.background
  property bool ready: false
  property bool lightTheme: false
  Harness.MockService {
    id: model
    property bool ready: true
    property bool loggedOut: false
    function quickSwitch(query, callback) {
      callback([{channel:{id:"9000",guild_id:"1",name:"general",type:"text",unread:"mentioned",mention_count:2},guild_name:"Fixture server",last_message_preview:"Text-only switcher fixture"}], "")
    }
    function openPanel(payload) { note("openPanel", payload.channel_id) }
  }
  Discord.SwitcherView { id: switcher; anchors.fill: parent; service: model }
  Component.onCompleted: {
    if (lightTheme) {
      Color.background = "#f5f4f0"; Color.foreground = "#242424"
      Color.muted = "#565656"; Color.accent = "#235a81"; Color.urgent = "#a02030"
    }
    switcher.open(); settle.start()
  }
  Timer { id: settle; interval: 120; onTriggered: root.ready = true }
}
