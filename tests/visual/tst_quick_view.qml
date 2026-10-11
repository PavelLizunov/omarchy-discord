import QtQuick
import QtTest
import "../../components" as Components
import "../../components/harness" as Harness

TestCase {
  id:test
  name:"CompactQuickView"
  width:460;height:540;visible:true;when:windowShown
  property int closes:0
  property var requests:[]
  Harness.MockService {
    id:model
    readonly property var backend:model
    function sendCommand(command,fields,callback) {
      note(command,JSON.stringify(fields))
      var row={volume:100,muted:!!fields.muted}
      callback(true,{"200":row},"")
    }
  }
  Components.QuickView {
    id:view;anchors.fill:parent;service:model
    onCloseRequested:test.closes++
    onFullRequested:function(payload){test.requests.push(payload)}
  }
  function init() {
    test.width=460;test.closes=0;test.requests=[];view.opened=false
    model.guilds=[{id:"1",name:"Fixture server"}]
    model.dms=[{id:"42",name:"Fixture DM"}]
    model.channelsByGuild={"1":[{id:"9",name:"Voice room",type:"voice"},{id:"8",name:"Text room",type:"text"}]}
    model.channelNames={"9":"Voice room"}
    model.voiceMembers={"1":[{channel_id:"9",users:[{id:"100",display_name:"Me",status:"online"},{id:"200",display_name:"Ada",status:"idle"}]}]}
    model.voice={status:"connected",guildId:"1",channelId:"9",muted:false,deafened:false}
    model.reset();view.opened=true;wait(30)
  }
  function browse() { var b=findChild(view,"quick-browse");mouseClick(b,b.width/2,b.height/2);wait(20) }
  function test_default_voice_room_single_click_local_mute_and_escape() {
    verify(view.callView);verify(!findChild(view,"quick-search").visible)
    var call=findChild(view,"quick-call");compare(view.focusItem,call.roster)
    verify(!findChild(view,"voice-device-details").visible)
    var row=findChild(view,"voice-row-200");verify(row.visible);mouseClick(row,5,5);wait(20)
    compare(call.roster.audioUserId,"200");compare(model.callCount("voice_user_audio"),1)
    var mute=findChild(view,"participant-mute");compare(mute.iconName,"speaker")
    mouseClick(mute,mute.width/2,mute.height/2);compare(model.callCount("voice_user_set"),1)
    compare(JSON.parse(model.lastCall("voice_user_set")).muted,true);compare(mute.iconName,"speakerOff")
    mute.forceActiveFocus();keyClick(Qt.Key_Escape);compare(call.roster.audioUserId,"");compare(closes,0)
    call.roster.forceActiveFocus();verify(call.roster.activeFocus);keyClick(Qt.Key_Escape);compare(closes,1)
    compare(model.callCount("voiceJoin"),0);compare(model.callCount("voiceLeave"),0);compare(model.callCount("voice_diagnostics"),0)
  }
  function test_browse_and_explicit_window_no_voice_or_ack() {
    browse();verify(!view.callView)
    var server=findChild(view,"quick-row-1");mouseClick(server,server.width/2,server.height/2);wait(20)
    compare(view.guildId,"1");compare(model.callCount("loadChannels"),1)
    test.width=320;wait(20)
    var close=findChild(view,"quick-close");verify(close.mapToItem(view,close.width,0).x<=view.width+1)
    var voice=findChild(view,"quick-row-9");verify(voice);mouseClick(voice,voice.width/2,voice.height/2)
    compare(requests.length,1);compare(requests[0].channel_id,"9")
    compare(model.callCount("voiceJoin"),0);compare(model.callCount("markChannelRead"),0)
    compare(model.callCount("showChannel"),0)
  }
  function test_mute_close_keyboard_and_geometry() {
    var mute=findChild(view,"voice-mute"),close=findChild(view,"quick-close"),full=findChild(view,"quick-full")
    verify(mute.visible);mouseClick(mute,mute.width/2,mute.height/2);compare(model.callCount("toggleMute"),1)
    model.voice={status:"connected",guildId:"1",channelId:"9",muted:true};wait(10);compare(mute.iconName,"microphoneOff")
    test.width=320;wait(20);verify(close.mapToItem(view,close.width,0).x<=view.width+1)
    verify(full.mapToItem(view,full.width,0).x<=view.width+1)
    close.forceActiveFocus();keyClick(Qt.Key_Return);compare(closes,1)
    view.focusSearch();keyClick(Qt.Key_Escape);compare(closes,2)
    compare(model.callCount("voiceLeave"),0);compare(model.callCount("logout"),0)
  }
  function test_search_back_and_idle_fallback() {
    browse();var search=findChild(view,"quick-search");view.focusSearch();keyClick(Qt.Key_D);keyClick(Qt.Key_M);compare(view.rows.length,1)
    keyClick(Qt.Key_Return);compare(requests[0].channel_id,"42")
    search.text="";view.activate({id:"1",kind:"server"});wait(20);keyClick(Qt.Key_Escape)
    verify(view.callView);compare(view.guildId,"");compare(closes,0);compare(model.voice.status,"connected")
    model.voice={status:"idle"};wait(20);verify(!view.callView);verify(search.visible)
    view.opened=false;view.opened=true;wait(20);verify(!view.browsing)
  }
}
