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
    property bool deferSearch:false
    property var requests:[]
    function quickSwitch(query, callback) {
      if(deferSearch){requests=requests.concat([{query:query,callback:callback}]);return}
      callback([{channel:{id:"9000",guild_id:"1",name:"general",type:"text"},guild_name:"Fixture server",last_message_preview:"hello"}],"")
    }
    function openPanel(payload) { note("openPanel",payload.channel_id) }
  }
  Discord.SwitcherView { id: view; anchors.fill: parent; service:model }
  function init(){view.close();view.searchFilter="all";view.entries=[];model.channelsByGuild={};model.guilds=[];model.deferSearch=false;model.requests=[];model.reset()}
  function test_search_and_activate() {
    view.open()
    compare(view.rows.length,1)
    view.setQuery("general")
    tryCompare(view,"busy",false)
    view.activate(0)
    compare(model.lastCall("openPanel"),"9000")
    verify(!view.opened)
  }
  function test_server_result_only_browses_channels() {
    model.guilds=[{id:"1",name:"Fixture server"}]
    model.currentChannelId="9000"
    view.open();view.setQuery("Fixture");view.searchFilter="servers"
    compare(view.rows.length,1)
    model.reset();view.activate(0)
    compare(model.selectedGuildId,"1");compare(model.currentChannelId,"")
    compare(model.callCount("showChannel"),0);compare(model.callCount("voiceJoin"),0)
    compare(model.callCount("openPanel"),1)
    model.guilds=[];view.searchFilter="all"
  }
  function test_voice_result_reads_without_joining() {
    view.open();view.query="";view.searchFilter="voice"
    view.entries=[{channel:{id:"9002",guild_id:"1",name:"Voice lounge",type:"voice"},guild_name:"Fixture server"}]
    compare(view.rows.length,1);model.voice={status:"connected",guildId:"2",channelId:"8000"}
    model.reset();view.activate(0)
    compare(model.lastCall("openPanel"),"9002")
    compare(model.callCount("voiceJoin"),0);compare(model.callCount("voiceLeave"),0)
    compare(model.voice.channelId,"8000");view.searchFilter="all";model.voice={status:"idle"}
  }
  function test_cached_voice_search_describes_reading_not_joining() {
    model.channelsByGuild={"1":[{id:"9002",guild_id:"1",name:"Voice lounge",type:"voice"}]}
    view.open();view.query="lounge";view.searchFilter="voice";view.entries=[]
    compare(view.rows.length,1);compare(view.rows[0].preview,"Open voice text chat")
    model.reset();view.activate(0);compare(model.lastCall("openPanel"),"9002")
    compare(model.callCount("voiceJoin"),0);compare(model.callCount("voiceLeave"),0)
    model.channelsByGuild={};view.searchFilter="all"
  }
  function test_uncached_servers_filter_and_delayed_results(){
    model.deferSearch=true;model.guilds=[{id:"1",name:"Uncached community"}]
    view.open();view.setQuery("community");view.searchFilter="servers"
    compare(view.rows.length,1);compare(view.rows[0].kind,"server");compare(view.emptyText,"")
    view.searchFilter="dms";compare(view.rows.length,0);compare(view.emptyText,"Searching…")
    tryVerify(function(){return model.requests.length===2})
    model.requests[0].callback([{channel:{id:"old",name:"stale",type:"text"}}],"")
    verify(view.busy);compare(view.entries.length,0)
    model.requests[1].callback([{channel:{id:"42",name:"Contact",type:"dm"}}],"")
    compare(view.rows.length,1);compare(view.rows[0].id,"42");verify(!view.busy)
    view.searchFilter="channels";compare(view.rows.length,0);verify(view.emptyText.indexOf("No results")===0)
  }
  function test_reopen_discards_previous_search_callback(){
    model.deferSearch=true;view.open();compare(model.requests.length,1)
    view.close();view.open();compare(model.requests.length,2)
    model.requests[1].callback([{channel:{id:"new",name:"current result",type:"text"}}],"")
    model.requests[0].callback([{channel:{id:"old",name:"old result",type:"text"}}],"")
    compare(view.rows[0].id,"new")
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
