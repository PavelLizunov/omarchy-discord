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
  Harness.MockService {id:model}
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
    model.voice={status:"connected",muted:false,deafened:false}
    model.reset();view.opened=true;wait(30)
  }
  function test_browse_and_explicit_window_no_voice_or_ack() {
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
    var mute=findChild(view,"quick-mute"),close=findChild(view,"quick-close"),full=findChild(view,"quick-full")
    verify(mute.visible);mouseClick(mute,mute.width/2,mute.height/2);compare(model.callCount("toggleMute"),1)
    model.voice={status:"connected",muted:true};wait(10);compare(mute.text,"Unmute microphone")
    test.width=320;wait(20);verify(close.mapToItem(view,close.width,0).x<=view.width+1)
    verify(full.mapToItem(view,full.width,0).x<=view.width+1)
    close.forceActiveFocus();keyClick(Qt.Key_Return);compare(closes,1)
    view.focusSearch();keyClick(Qt.Key_Escape);compare(closes,2)
    compare(model.callCount("voiceLeave"),0);compare(model.callCount("logout"),0)
  }
  function test_search_and_back_retains_voice() {
    var search=findChild(view,"quick-search");view.focusSearch();keyClick(Qt.Key_D);keyClick(Qt.Key_M);compare(view.rows.length,1)
    keyClick(Qt.Key_Return);compare(requests[0].channel_id,"42")
    search.text="";view.activate({id:"1",kind:"server"});wait(20);keyClick(Qt.Key_Escape)
    compare(view.guildId,"");compare(closes,0);compare(model.voice.status,"connected")
  }
}
