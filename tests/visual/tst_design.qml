import QtQuick
import QtTest
import "../../" as Discord
import "../../ui"
import "../../Api.js" as Api
import "../../components" as Components

TestCase {
 id: tests; name: "DesignRegressions"; width:520; height:560; visible:true; when:windowShown
 Preview { id:p; anchors.fill:parent }
 Discord.SwitcherView { id:s; anchors.fill:parent; visible:false; service:searchModel }
 QtObject {
  id:searchModel;property bool ready:true;property bool connected:true;property bool loggedOut:false
  property var guilds:[];property var channelsByGuild:({});property var markdownCtx:({})
  function setUiVisible(key,value) {}
  function quickSwitch(query,callback){callback([],"")}
 }
 Components.MessageRow {id:row;visible:false;width:200;message:({content:"link",author:{id:"200"},timestamp:"2026-01-01T00:00:00Z"})}
 property var initialGuilds:[]
 property var initialChannels:({})
 function initTestCase(){tryCompare(p,"ready",true);initialGuilds=p.model.guilds;initialChannels=p.model.channelsByGuild}
 function init(){
  tests.width=520;tests.height=560;p.model.voice={status:"idle"};p.model.membersWanted=false
  p.model.currentChannelId="9000";p.client.setCompactMode(true);p.client.enterComposer();p.model.reset()
  p.model.guilds=initialGuilds;p.model.channelsByGuild=initialChannels;p.model.selectedGuildId="1"
  p.client.serverSort="position";p.client.serverFilter="all";p.client.serverToolsShown=false
  p.client.channelFilter="all";p.client.optionsMenu.shown=false;s.visible=false
 }
 function test_size_toggle_preserves_chat_draft_and_voice(){
  p.model.voice={status:"connected",guildId:"1",channelId:"9002"};p.client.enterComposer();p.client.composer.setText("Keep my draft")
  p.model.reset();p.client.setCompactMode(false);wait(30)
  verify(findChild(p.client,"chat-column").visible);verify(!findChild(p.client,"server-rail").visible)
  compare(p.client.currentChannelId,"9000");compare(p.client.composer.text,"Keep my draft")
  p.client.setCompactMode(true);wait(30);verify(findChild(p.client,"chat-column").visible)
  compare(p.model.voice.channelId,"9002");compare(p.model.callCount("voiceJoin"),0);compare(p.model.callCount("voiceLeave"),0)
  p.client.composer.setText("")
  p.client.compactChatOpen=false;p.client.setCompactMode(false);wait(30)
  verify(findChild(p.client,"channel-pane").visible);verify(!findChild(p.client,"chat-column").visible)
 }
 function test_voice_chat_reserves_left_once(){
  p.model.voice={status:"connected",guildId:"1",channelId:"9002"};wait(30)
  var chat=findChild(p.client,"chat-column"),bar=findChild(p.client,"call-bar")
  verify(chat.width>280);compare(Math.round(chat.mapToItem(p.client,0,0).x-bar.mapToItem(p.client,bar.width,0).x),Style.spacing.panelGap)
  tests.width=420;wait(30);verify(chat.width>200);verify(p.client.composer.width>200)
  p.model.voice={status:"error",guildId:"1",channelId:"9002"};wait(30);verify(chat.width>200);verify(bar.roster.visible)
 }
 function test_voice_hierarchy_preserves_readable_navigation_and_user_card(){
  p.model.voice={status:"connected",guildId:"1",channelId:"9002"};p.client.compactChatOpen=false
  p.model.guilds=[{id:"1",name:"gs.ninitux.com"}];tests.width=597;tests.height=400;wait(40)
  var rail=findChild(p.client,"server-rail"),channels=findChild(p.client,"channel-pane"),content=findChild(p.client,"selected-channel-content"),bar=findChild(p.client,"call-bar")
  verify(rail.width>=160);verify(rail.mapToItem(p.client,rail.width,0).x<channels.mapToItem(p.client,0,0).x)
  verify(channels.mapToItem(p.client,0,channels.height).y<=bar.mapToItem(p.client,0,0).y)
  compare(bar.parent,content);verify(bar.roster.visible);verify(bar.roster.compactHeader)
  tests.width=420;tests.height=420;wait(30);verify(rail.width>=160);verify(bar.roster.height>=50)
  tests.width=597;tests.height=400;wait(30)
  var user=findChild(p.client,"user-bar"),settings=findChild(p.client,"user-bar-settings")
  verify(settings.mapToItem(user,settings.width,0).x<=user.width)
  var label=findChild(p.client,"server-name-1");verify(label.height<30)
  tests.width=1040;tests.height=680;p.client.setCompactMode(false);wait(40)
  verify(channels.mapToItem(p.client,channels.width,0).x<=content.mapToItem(p.client,0,0).x)
  verify(bar.mapToItem(p.client,bar.width,0).x<=findChild(p.client,"chat-column").mapToItem(p.client,0,0).x)
  compare(p.model.callCount("voiceJoin"),0);compare(p.model.callCount("voiceLeave"),0)
  p.model.guilds=[{id:"1",name:"Fixture server",unread:"mentioned",mention_count:2}]
 }
 function test_members_do_not_cover_navigation(){
  p.model.membersWanted=true;p.client.compactChatOpen=false;wait(30)
  verify(findChild(p.client,"channel-pane").visible);verify(!p.client.members.visible)
  p.client.enterComposer();wait(30);verify(p.client.members.visible)
  var chat=findChild(p.client,"chat-column");verify(chat.mapToItem(p.client,chat.width,0).x<=p.client.members.mapToItem(p.client,0,0).x)
 }
 function test_filtered_voice_mouse_and_keyboard_use_visible_id(){
  p.client.compactChatOpen=false;p.client.channelFilter="voice";wait(30)
  compare(p.client.filteredChannelRows[0].id,"9002");p.model.reset()
  p.client.activateChannel(0,"channels");compare(p.model.lastCall("showChannel"),"9002");compare(p.model.callCount("voiceJoin"),0)
  p.client.joinVoice(0);compare(p.model.lastCall("voiceJoin"),"9002")
  p.client.compactChatOpen=false;p.client.zone="sidebar";p.client.column="channels";p.client.channelCursorId="";p.client.moveChannelCursor(1);p.client.focusZone();wait(20)
  compare(p.client.channelCursorId,"9002");p.model.reset();keyClick(Qt.Key_Return)
  compare(p.model.lastCall("showChannel"),"9002");compare(p.model.callCount("voiceJoin"),0)
 }
 function test_voice_pointer_reads_without_switching_existing_call(){
  p.model.voice={status:"connected",guildId:"2",channelId:"8000"};p.client.compactChatOpen=false
  p.client.channelFilter="voice";wait(50)
  var target=findChild(p.client,"open-channel-9002");verify(target.visible)
  p.model.reset();mouseClick(target,target.width/2,target.height/2);wait(30)
  compare(p.model.lastCall("showChannel"),"9002");compare(p.model.callCount("voiceJoin"),0)
  compare(p.model.callCount("voiceLeave"),0);compare(p.model.voice.channelId,"8000")
  var join=findChild(p.client,"join-channel-voice");verify(join.visible)
  verify(join.mapToItem(p.client,0,join.height).y<=p.client.timeline.mapToItem(p.client,0,0).y)
  verify(join.width<=findChild(p.client,"chat-column").width)
  verify(join.bordered);verify(join.backgroundColor.a>0);verify(join.width>=150)
  p.client.zone="composer";p.client.focusZone();var reached=false
  for(var i=0;i<30;i++){p.client.cycleFocus(1);if(join.activeFocus){reached=true;break}}
  verify(reached);keyClick(Qt.Key_Return);compare(p.model.lastCall("voiceJoin"),"9002")
 }
 function test_navigation_links_are_not_server_tiles(){
  p.useRealServerActions=true;p.client.compactChatOpen=false;p.client.serverFilter="all";p.client.serverActions.archivedGuilds={"2":true}
  p.model.guilds=[{id:"1",name:"Fixture server"},{id:"2",name:"Archived fixture"}];wait(50)
  var dm=findChild(p.client,"navigation-entry-dms"),archive=findChild(p.client,"navigation-entry-archive")
  verify(dm!==null);verify(archive!==null);verify(findChild(p.client,"guild-entry-1")!==null)
  var dmTile=findChild(p.client,"guild-tile-dms"),archiveTile=findChild(p.client,"guild-tile-archive")
  compare(dmTile.color.a,0);compare(archiveTile.color.a,0)
  var heading=findChild(p.client,"guild-section-heading");verify(heading!==null);compare(heading.text,"Servers")
  mouseClick(archiveTile,archiveTile.width/2,archiveTile.height/2);wait(30);compare(p.client.serverFilter,"archive")
  verify(findChild(p.client,"navigation-entry-archive_back")!==null)
  p.client.setGuildCursor(0);p.client.enterChannels();wait(30);compare(p.client.serverFilter,"all")
  p.client.serverActions.archivedGuilds=({});p.useRealServerActions=false;p.model.guilds=[{id:"1",name:"Fixture server",unread:"mentioned",mention_count:2}]
 }
 function test_filter_chips_keyboard(){
  p.client.compactChatOpen=false;p.client.zone="sidebar";p.client.column="rail";p.client.focusZone();wait(20)
  var buttons=p.client.filterButtons(findChild(p.client,"server-filter-chips"))
  keyClick(Qt.Key_Tab);verify(buttons[0].activeFocus);keyClick(Qt.Key_Tab);verify(buttons[1].activeFocus)
  keyClick(Qt.Key_Return);compare(p.client.serverFilter,"unread");p.client.serverFilter="all"
  p.client.focusStop("channelChips",1);wait(20);keyClick(Qt.Key_Tab);keyClick(Qt.Key_Tab);keyClick(Qt.Key_Space)
  compare(p.client.channelFilter,"voice")
 }
 function test_search_categories_feedback_and_fit(){
  s.visible=true;s.open();s.searchFilter="servers";searchModel.guilds=[{id:"1",name:"Design workshop",unread:"read"}];s.setQuery("Design");wait(100)
  compare(s.rows.length,1);compare(s.emptyText,"")
  var list=[];for(var i=0;i<20;i++)list.push({channel:{id:String(i),guild_id:"1",name:"long channel "+i,type:"text"},guild_name:"Synthetic server"})
  tests.width=420;tests.height=420;s.searchFilter="all";s.query="";s.entries=list;wait(40)
  var card=findChild(s,"search-card"),results=findChild(s,"search-results"),footer=findChild(s,"search-footer"),categories=findChild(s,"search-categories")
  verify(results.mapToItem(s,0,results.height).y<=footer.mapToItem(s,0,0).y)
  verify(footer.mapToItem(s,0,footer.height).y<=card.y+card.height)
  verify(categories.height>32)
  var input=findChild(s,"search-input");input.forceActiveFocus();wait(20);keyClick(Qt.Key_Tab)
  verify(categories.children[0].activeFocus);keyClick(Qt.Key_Tab);keyClick(Qt.Key_Space);compare(s.searchFilter,"channels")
  keyClick(Qt.Key_Escape);verify(!s.opened)
 }
 function test_long_lists_and_tools_keep_navigation_reachable(){
  tests.width=420;tests.height=420;p.client.compactChatOpen=false;p.client.serverToolsShown=true
  p.client.serverSort="online";p.model.voice={status:"connected",guildId:"1",channelId:"9002"}
  var guilds=[],channels=[]
  for(var i=0;i<40;i++){guilds.push({id:String(i+1),name:"A long server name "+i});channels.push({id:String(9100+i),guild_id:"1",name:"channel "+i,type:"text"})}
  p.model.guilds=guilds;p.model.channelsByGuild={"1":channels};wait(40)
  var servers=findChild(p.client,"server-results"),list=findChild(p.client,"channel-results"),user=findChild(p.client,"user-bar")
  verify(servers.height>=48,"server tools consume all list height: "+servers.height)
  verify(list.height>=48,"channel results height: "+list.height)
  verify(servers.mapToItem(p.client,0,servers.height).y<=user.mapToItem(p.client,0,0).y)
  p.client.zone="sidebar";p.client.column="rail";p.client.setGuildCursor(p.client.guildRows.length-1);p.client.focusZone();wait(30)
  verify(servers.contentY>0);compare(p.client.guildCursorId,String(p.client.guildRows[p.client.guildRows.length-1].id))
  var viewport=findChild(p.client,"server-controls-viewport"),refresh=findChild(p.client,"server-count-refresh")
  refresh.forceActiveFocus();wait(40)
  verify(viewport.contentY>0);verify(refresh.mapToItem(viewport,0,refresh.height).y<=viewport.height+1)
  p.client.serverToolsShown=false;p.client.serverSort="position"
 }
 function test_audio_details_keyboard_and_no_automatic_tone(){
  p.model.voice={status:"connected",guildId:"1",channelId:"9002"};wait(30)
  var bar=findChild(p.client,"call-bar"),toggle=findChild(p.client,"voice-audio-details"),d=findChild(p.client,"voice-audio-diagnostics")
  p.client.focusCallBar();verify(toggle.activeFocus);keyClick(Qt.Key_Return);verify(d.details)
  p.client.cycleFocus(1);verify(findChild(p.client,"voice-test-output").activeFocus)
  keyClick(Qt.Key_Escape);verify(!d.details);verify(toggle.activeFocus)
  toggle.clicked();p.client.cycleFocus(1);verify(findChild(p.client,"voice-test-output").activeFocus)
  p.client.cycleFocus(1);verify(findChild(p.client,"voice-reconnect-audio").activeFocus)
  p.client.cycleFocus(1);verify(findChild(p.client,"voice-device-details").activeFocus)
  p.client.cycleFocus(1);verify(findChild(p.client,"voice-mute").activeFocus)
  toggle.clicked()
  p.model.reset();toggle.clicked();toggle.clicked();compare(p.model.callCount("voice_test_output"),0)
  compare(p.model.callCount("voiceJoin"),0);compare(p.model.callCount("voiceLeave"),0)
 }
 function test_rejoin_is_one_explicit_call_and_preserves_context(){
  p.model.voice={status:"connected",guildId:"1",channelId:"9002",muted:true,deafened:false};wait(20)
  var bar=findChild(p.client,"call-bar"),d=findChild(p.client,"voice-audio-diagnostics")
  bar.rejoining=false;p.model.reset();var channel=p.model.currentChannelId
  bar.rejoin();bar.rejoin();compare(p.model.callCount("voiceJoin"),1);compare(p.model.callCount("voiceLeave"),0)
  compare(p.model.currentChannelId,channel);compare(p.model.voice.muted,true);compare(p.model.voice.deafened,false)
  bar.rejoining=false
 }
 function test_composited_text_contrast(){
  var oldBg=Color.background,oldFg=Color.foreground,oldMuted=Color.muted,oldAccent=Color.accent
  Color.background="#111c18";Color.foreground="#c1c497";Color.muted="#53685b";Color.accent="#509475";row.cursor=true
  verify(Api.contrastRatio(Api.blend(row.linkColor,row.paintedBackground,row.linkColor.a),row.paintedBackground)>=4.5)
  verify(Api.contrastRatio(Api.blend(row.muted,row.paintedBackground,row.muted.a),row.paintedBackground)>=4.5)
  var badge=Api.blend(Color.accent,row.paintedBackground,.2)
  verify(Api.contrastRatio(Api.textColor(Color.accent,Color.foreground,badge),badge)>=4.5)
  Color.background=oldBg;Color.foreground=oldFg;Color.muted=oldMuted;Color.accent=oldAccent
 }
}
