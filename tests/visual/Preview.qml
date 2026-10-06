import QtQuick
import "../../" as Discord
import "../../ui"
import "../../components/harness" as Harness
import "../../components/harness/Fixtures.js" as Fixtures

Rectangle {
  id: root
  width: 1040
  height: 680
  color: Color.background
  property string state: "chat"
  property bool lightTheme: false
  property int themeRadius: 0
  property bool ready: false
  property int interactionChecks: 0
  property string interactionError: ""
  property string copiedText: ""
  property string openedLink: ""
  property alias client: client
  property alias model: model
  Harness.MockService {
    id: model
    function mediaPath(url, size) { throw new Error("Text view requested remote media") }
    property bool persistentWindow: false
    property var knownUsers: ({"202":"Known fixture contact"})
    readonly property var markdownCtx: ({ users:{"100":"Matt","200":"Ada","300":"Lin"},channels:{"9001":"development"},roles:{"5001":"maintainers"},selfId:selfId,
      linkColor:String(Color.accent),mentionColor:String(Color.accent),mentionBg:Util.alpha(Color.accent,0.18),
      spoilerColor:String(Color.foreground),codeBg:Util.alpha(Color.foreground,0.08),mutedColor:String(Color.muted),monoFamily:Style.font.family,fontSize:Style.font.body })
    property var drafts: ({})
    function draftFor(channel) { return drafts[channel] || "" }
    function setDraft(channel, text) { var next = Object.assign({}, drafts); next[channel] = text; drafts = next }
    function sendMessage(channel, content, reply) { note("sendMessage",content); return true }
    function findMessage(channel, id) {
      var entry = channelData[channel]
      return entry ? entry.messages.find(function(row) { return row.id === id }) || null : null
    }
    function lastOwnMessageId(channel) {
      var entry = channelData[channel]
      if (!entry) return ""
      for (var i=entry.messages.length-1;i>=0;i--) if (entry.messages[i].author.id === selfId) return entry.messages[i].id
      return ""
    }
    function editMessage(channel, id, content, callback) { note("editMessage",content); if (callback) callback(true) }
  }
  Discord.ClientView {
    id: client
    objectName: "client"
    anchors.fill: parent
    service: model
    onCopyRequested: function(text) { root.copiedText = text }
    onLinkRequested: function(url) { root.openedLink = url }
    onCloseRequested: close()
  }
  Component.onCompleted: {
    Style.cornerRadius = themeRadius
    if (lightTheme) {
      Color.background = "#f5f4f0"
      Color.foreground = "#242424"
      Color.muted = "#565656"
      Color.accent = "#235a81"
      Color.urgent = "#a02030"
    }
    model.user = {username:"fixture",display_name:"Fixture user"}
    model.channelNames = {"9000":"general","9001":"development","9002":"Voice lounge"}
    model.guilds = [{id:"1",name:"Fixture server",unread:"mentioned",mention_count:2,icon_url:"https://cdn.discordapp.com/icons/fixture.png"}]
    model.dms = [{id:"42",name:"Fixture contact",type:"dm",unread:"read"}]
    model.selectedGuildId = "1"
    model.currentChannelId = "9000"
    model.channelsByGuild = {"1":[{id:"9000",guild_id:"1",name:"general",type:"text",topic:"Synthetic data · no network or system actions"}, {id:"9001",guild_id:"1",name:"development",type:"text"}, {id:"9002",guild_id:"1",name:"Voice lounge",type:"voice"}]}
    model.channelData = {"9000":{channel:model.channelsByGuild["1"][0],messages:Fixtures.build(14,1080,1790800000000),loading:false,hasMore:false}}
    if (state === "empty" || state === "loading") model.channelData = {"9000":{channel:model.channelsByGuild["1"][0],messages:[],loading:state === "loading",hasMore:false}}
    if (state === "error") model.lastError = "Fixture error: connection unavailable; press r to retry."
    if (state === "login") { model.showStructure = false; model.lifecycle = "logged_out"; model.statusText = "Logged out" }
    if (state === "qr") {
      model.showStructure = false
      model.lifecycle = "qr_pending"
      model.statusText = "Waiting for QR scan"
      model.qr = {stage:"code",imagePath:decodeURIComponent(String(Qt.resolvedUrl("qr-fixture.png")).replace(/^file:\/\//,"")),revision:1,expiresAt:Date.now()+120000}
    }
    if (state === "members") {
      model.membersWanted = true
      model.membersChannelId = "9000"
      model.memberList = {channel_id:"9000",groups:[{id:"online",name:"Online",count:2}],members:[{user:{id:"200",username:"ada",display_name:"Ada"},group_id:"online",status:"online",activity:""}]}
    }
    if (state === "voice") model.voice = {status:"connected",guildId:"1",channelId:"9002",muted:false,deafened:false,error:""}
    if (state === "voice" || state === "voice-unknown") model.voiceMembers = {"1":[{channel_id:"9002",users:[
      {id:"200",display_name:"Ada"},{id:"202"},{id:"123456789012345678"}
    ]}]}
    client.open("{}")
    settle.start()
  }
  Timer {
    id: settle
    interval: 150
    repeat: true
    onTriggered: {
      if (root.state === "qr" && !client.qrImageReady) return
      if (root.state !== "login") {
        client.zone = "timeline"
        client.timeline.focusNewest()
        client.focusZone()
      }
      root.ready = true
      stop()
    }
  }
}
