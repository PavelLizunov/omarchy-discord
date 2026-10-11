pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls as Controls
import "../ui"

Item {
  id: root
  property var service: null
  DMPresence {id:presence;service:root.service;active:root.opened}
  property bool opened: false
  property string guildId: ""
  property bool browsing: false
  readonly property bool callView: !!(service && service.voice && service.voice.status !== "idle") && !browsing
  readonly property var focusItem: callView ? call.roster : search
  readonly property alias callPanel: call
  readonly property bool ready: !!(service && service.showStructure)
  readonly property bool inCall: !!(service && service.voice && service.voice.status !== "idle")
  readonly property bool muted: !!(service && service.voice && service.voice.muted)
  readonly property bool deafened: !!(service && service.voice && service.voice.deafened)
  readonly property var rows: {
    var result = [], query = search.text.trim().toLocaleLowerCase()
    function add(row) { if (!query || row.label.toLocaleLowerCase().indexOf(query) >= 0) result.push(row) }
    if (!ready) return result
    if (guildId) {
      var channels = service.channelsFor(guildId)
      for (var c = 0; c < channels.length; c++) {
        var ch = channels[c]
        if (["text", "voice", "announcement", "forum", "dm", "group_dm"].indexOf(String(ch.type)) < 0) continue
        add({id:String(ch.id), guild:guildId, label:String(ch.name || "Unnamed channel"), kind:"channel", unread:ch.unread, voice:ch.type === "voice"})
      }
    } else {
      var dms = service.dms || [], guilds = service.guilds || []
      for (var d = 0; d < dms.length; d++)
        add({id:String(dms[d].id), guild:"dms", label:String(dms[d].name || "Direct message"), kind:"dm", unread:dms[d].unread, channel:dms[d]})
      for (var g = 0; g < guilds.length; g++)
        add({id:String(guilds[g].id), label:String(guilds[g].name || "Unnamed server"), kind:"server", unread:guilds[g].unread})
    }
    return result
  }
  signal closeRequested()
  signal fullRequested(var payload)
  function focusSearch() { if (callView) call.roster.forceActiveFocus(); else search.forceActiveFocus() }
  function activate(row) {
    if (!row) return
    if (row.kind === "server") {
      guildId = row.id; search.text = ""; service.loadChannels(row.id, false); focusSearch()
    } else fullRequested({channel_id:row.id})
  }
  function dismiss() {
    if (callView && call.dismissAudio()) return
    if (browsing && inCall) { browsing = false; guildId = ""; focusSearch(); return }
    if (guildId) { guildId = ""; search.text = ""; focusSearch() }
    else closeRequested()
  }
  onOpenedChanged: {
    if (service) service.setUiVisible("quick-panel", opened)
    if (opened) { browsing = false; guildId = ""; search.text = ""; Qt.callLater(focusSearch) }
  }
  Component.onDestruction: if (service) service.setUiVisible("quick-panel", false)
  Keys.onEscapePressed: function(event) { root.dismiss(); event.accepted = true }
  onCallViewChanged: if (opened) Qt.callLater(focusSearch)
  Column {
    anchors.fill: parent
    spacing: Style.spacing.sm
    Row {
      width: parent.width
      spacing: Style.spacing.controlGap
      Button {
        id: back; objectName: "quick-back"; visible: !!root.guildId; iconOnly: true; iconName: "back"
        text: "Back to servers and DMs"; tooltipText: text; focusable: true; onClicked: root.dismiss()
      }
      Text {
        width: Math.max(0, parent.width - full.width - close.width - (root.guildId ? back.width : 0) - parent.spacing * (root.guildId ? 3 : 2))
        anchors.verticalCenter: parent.verticalCenter
        text: "Omacord"; color: Color.foreground; font.family: Style.font.family; font.pixelSize: Style.font.body
      }
      Button {id:full; objectName:"quick-full";text:"Open window";iconName:"expand";focusable:true;onClicked:root.fullRequested({})}
      Button {id:close; objectName:"quick-close";text:"Close";iconOnly:true;iconName:"close";tooltipText:"Close compact panel";focusable:true;onClicked:root.closeRequested()}
    }
    Button {
      objectName: "quick-browse"
      visible: root.inCall
      width: parent.width
      text: root.browsing ? "Back to voice room" : "Browse conversations"
      iconName: root.browsing ? "headphones" : "search"
      focusable: true
      onClicked: { root.browsing = !root.browsing; root.guildId = ""; Qt.callLater(root.focusSearch) }
    }
    CallBar {
      id: call
      objectName: "quick-call"
      visible: root.callView
      width: parent.width
      height: Math.max(0, root.height - y)
      service: root.service
      expanded: true
      compactRosterHeader: true
      showDiagnostics: false
      roster.singleClickAudio: true
      channelName: root.service && root.service.channelNames && root.service.voice
        ? String(root.service.channelNames[String(root.service.voice.channelId || "")] || "Voice room") : "Voice room"
      onChatRequested: root.fullRequested({channel_id:String(root.service.voice.channelId || "")})
    }
    Connections {
      target: call.roster
      function onEscapeRequested() { root.dismiss() }
    }
    TextField {
      id:search;objectName:"quick-search";width:parent.width;visible:!root.callView
      placeholderText:root.guildId?"Find a channel":"Find a server or direct message"
      Keys.onDownPressed: function(event) { list.currentIndex=Math.min(list.count-1,list.currentIndex+1);event.accepted=true }
      Keys.onUpPressed: function(event) { list.currentIndex=Math.max(0,list.currentIndex-1);event.accepted=true }
      onAccepted: root.activate(root.rows[list.currentIndex])
    }
    ListView {
      id:list;objectName:"quick-results";width:parent.width;visible:!root.callView
      height:Math.max(0,root.height-y);clip:true;model:root.rows;currentIndex:0
      onCountChanged: currentIndex=0
      Controls.ScrollBar.vertical:Controls.ScrollBar {policy:Controls.ScrollBar.AsNeeded}
      delegate: Button {
        required property var modelData
        required property int index
        width:list.width;height:Style.space(44);leftAlign:true;focusable:true
        objectName:"quick-row-"+modelData.id
        text:modelData.kind==="dm" ? modelData.label+" · "+presence.summary(modelData.channel,false) : modelData.label
        iconName:modelData.kind==="server"?"all":modelData.voice?"headphones":modelData.kind==="dm"?"user":"textChannel"
        active:modelData.unread==="mentioned";hasCursor:list.currentIndex===index && search.activeFocus
        tooltipText:modelData.label+(modelData.kind==="dm" ? "\n"+presence.summary(modelData.channel,true) : "")+(modelData.kind==="server"?" · Browse channels":" · Open conversation in window")
        onClicked:root.activate(modelData)
      }
      Text {
        anchors.centerIn:parent;visible:list.count===0;width:parent.width
        horizontalAlignment:Text.AlignHCenter;wrapMode:Text.WordWrap
        text:!root.ready?"Open the window to sign in":root.guildId&&root.service.isLoadingChannels(root.guildId)?"Loading channels…":"No matching conversations"
        color:Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.body
      }
    }
  }
}
