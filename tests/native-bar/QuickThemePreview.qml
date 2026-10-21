import QtQuick
import qs.Commons as Host
import "../../" as Plugin
import "../../ui" as PluginTheme
import "../../components" as Components
import "../../components/harness" as Harness

Rectangle {
  id: root
  width: 460; height: 572
  objectName: "quick-theme-preview"
  property bool light: true
  property bool failed: true
  property bool browsing: false
  property bool switchTheme: false
  property bool switched: false
  property bool auditReady: false
  color: Host.Color.popups.background

  function applyTheme() {
    Host.Color.foreground = light ? "#575279" : "#cacccc"
    Host.Color.background = light ? "#faf4ed" : "#101315"
    Host.Color.accent = light ? "#56949f" : "#cacccc"
    Host.Color.muted = light ? "#cecacd" : "#78818a"
    Host.Color.urgent = light ? "#b4637a" : "#bc6868"
    Host.Color.shellValues = ({})
    auditReady = false
    settled.restart()
  }
  property bool initialized: false
  onLightChanged: if (initialized) applyTheme()
  // Let the host's asynchronous initial theme-file reads settle before fixture input.
  Timer { interval: 250; running: true; onTriggered: { root.initialized = true; root.applyTheme() } }

  Harness.MockService {
    id: model
    property bool ready: true
    property int totalMentionCount: 0
    property bool anyUnread: false
    readonly property var backend: model
    function applySettings(values) {}
    function sendCommand(name, fields, callback) { note(name, JSON.stringify(fields)); callback(true, {}, "") }
    channelNames: ({"9":"Gaming"})
    guilds: [{id:"1",name:"Fixture server"}]
    dms: [{id:"42",name:"Fixture direct message"}]
    voiceMembers: ({"1":[{channel_id:"9",users:[{id:"100",display_name:"Fixture user",status:"online"},{id:"200",display_name:"Ada",status:"online"}]}]})
    voice: ({status:root.failed ? "error" : "connected",guildId:"1",channelId:"9",muted:false,deafened:false})
  }
  QtObject {
    id: hostBar
    property bool vertical: false
    property int barSize: 26
    property string position: "top"
    property color foreground: Host.Color.foreground
    property color barForeground: foreground
    property color urgent: Host.Color.urgent
    property string fontFamily: Host.Style.font.family
    property bool foregroundAnimationEnabled: false
    property var activePopout: null
    property var shell: QtObject { function serviceFor(id) { return model } }
    function hideTooltip(item) {}
    function showTooltip(item, text) {}
    function registerClickTarget(item) {}
    function unregisterClickTarget(item) {}
    function requestPopout(item) { activePopout = item }
    function releasePopout(item) { if (activePopout === item) activePopout = null }
  }
  // The bar must synchronize its own module before any standalone Panel exists.
  Plugin.BarWidget { id: widget; bar: hostBar; visible: false }
  Components.QuickView {
    id: view
    objectName: "quick-theme-content"
    anchors.fill: parent
    anchors.margins: 12
    service: model
    opened: true
    browsing: root.browsing
  }
  function find(item, name) {
    if (item.objectName === name) return item
    for (var i = 0; i < item.children.length; i++) {
      var found = find(item.children[i], name)
      if (found) return found
    }
    return null
  }
  Timer {
    id: settled
    interval: 150
    onTriggered: {
      view.browsing = root.browsing
      if (String(Host.Color.background) !== (root.light ? "#faf4ed" : "#101315")) throw Error("Fixture theme was replaced by host startup")
      if (String(PluginTheme.Color.foreground) !== String(Host.Color.foreground)) throw Error("Bar left foreground in a stale palette")
      if (String(PluginTheme.Color.popups.background) !== String(Host.Color.popups.background)) throw Error("Bar left roster background in a stale palette")
      if (String(view.callPanel.roster.foreground) !== String(Host.Color.popups.text)) throw Error("Roster text differs from the host popup")
      var full = root.find(view, "quick-full")
      if (!full || String(full.foreground) !== String(Host.Color.foreground)) throw Error("Compact action text differs from the host")
      if (root.switchTheme && !root.switched) {
        root.switched = true
        root.light = !root.light
        return
      }
      root.auditReady = true
    }
  }
}
