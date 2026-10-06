import QtQuick
import QtTest

TestCase {
 id: tests
 name: "Servers"
 width:1040; height:680; visible:true; when:windowShown
 Preview { id: preview; anchors.fill:parent; serverListFixture:true }
 function initTestCase(){tryCompare(preview,"ready",true)}
 function init(){
  tests.width=1040;tests.height=680;preview.client.compactMode=false
  preview.client.serverToolsShown=true
  preview.client.serverQuery="";preview.client.serverFilter="all";preview.client.serverSort="position"
  preview.client.serverMenu.shown=false;preview.client.navigationShown=false
  preview.model.voice={status:"idle"};preview.model.membersWanted=true
  preview.model.currentChannelId="9000";preview.model.reset();wait(50)
 }
 function click(item){mouseClick(item,item.width/2,item.height/2);wait(60)}
 function test_compact_voice_does_not_cover_composer(){
  tests.width=520;tests.height=560;preview.client.setCompactMode(true)
  preview.model.voice={status:"connected",guildId:"1",channelId:"9002",muted:false,deafened:false,error:""};wait(60)
  var call=findChild(preview.client,"call-bar")
  var composer=preview.client.composer
  var right=call.mapToItem(preview.client,call.width,0).x
  verify(right<=composer.mapToItem(preview.client,0,0).x)
  preview.model.voice={status:"idle",muted:false,deafened:false,error:""}
 }
 function test_compact_toggle_preserves_draft_and_clicks(){
  preview.client.composer.setText("Keep compact draft")
  click(findChild(preview.client,"layout-mode-button"));verify(preview.client.compactView)
  tests.width=520;tests.height=560;wait(40)
  verify(!findChild(preview.client,"server-rail").visible)
  verify(!findChild(preview.client,"channel-pane").visible)
  verify(preview.client.timeline.width>280)
  verify(preview.client.members.visible)
  verify(preview.client.members.width>=120)
  compare(preview.client.composer.text,"Keep compact draft")
  click(findChild(preview.client,"search-button"));compare(preview.model.callCount("openSwitcher"),1)
  click(findChild(preview.client,"layout-mode-button"));verify(!preview.client.compactMode)
  tests.width=1040;wait(40);verify(findChild(preview.client,"server-rail").visible)
  compare(preview.client.composer.text,"Keep compact draft");preview.client.composer.setText("")
 }
 function test_no_server_tooltip_on_hover_or_cursor(){
  var rail=findChild(preview.client,"server-rail")
  function countTips(node){
   var count=typeof node.delay==="number" && typeof node.text==="string" ? 1 : 0
   var children=node.data||node.children||[]
   for(var i=0;i<children.length;i++)count+=countTips(children[i])
   return count
  }
  preview.client.zone="sidebar";preview.client.column="rail";preview.client.setGuildCursor(0);preview.client.focusZone()
  var label=findChild(preview.client,"server-name-dms")
  mouseMove(label,label.width/2,label.height/2);wait(450)
  compare(countTips(label.parent.parent),0)
 }
 function test_filter_and_sort_mouse_selection(){
  preview.useRealServerActions=true
  var filter=findChild(preview.client,"server-filter")
  click(filter);tryCompare(filter.popup,"visible",true)
  var list=filter.popup.contentItem
  tryVerify(function(){return list.itemAtIndex(1)!==null})
  click(list.itemAtIndex(1));tryCompare(preview.client,"serverFilter","unread")
  verify(!filter.popup.visible)
  var sort=findChild(preview.client,"server-sort")
  click(sort);tryCompare(sort.popup,"visible",true)
  list=sort.popup.contentItem
  tryVerify(function(){return list.itemAtIndex(1)!==null})
  click(list.itemAtIndex(1));tryCompare(preview.client,"serverSort","name")
  verify(!sort.popup.visible)
  var names=preview.client.filteredGuilds.map(function(g){return g.name})
  var sorted=names.slice().sort(function(a,b){return a.localeCompare(b)})
  compare(JSON.stringify(names),JSON.stringify(sorted))
  preview.useRealServerActions=false
 }
 function test_actual_controller_mouse_commands_and_online_sort(){
  preview.useRealServerActions=true
  var before=preview.model.currentChannelId
  preview.client.showServerMenu(preview.model.guilds[0],160,80);wait(40)
  click(findChild(preview.client,"server-settings"))
  compare(preview.client.serverMenu.page,"settings")
  compare(preview.model.callCount("guild_settings"),1)
  click(findChild(preview.client,"server-cancel"));verify(!preview.client.serverMenu.shown)
  preview.client.showServerMenu(preview.model.guilds[0],160,80);wait(40)
  click(findChild(preview.client,"server-mark-read"))
  compare(preview.model.callCount("mark_guild_read"),1);verify(!preview.client.serverMenu.shown)
  var sort=findChild(preview.client,"server-sort")
  click(sort);tryCompare(sort.popup,"visible",true)
  var list=sort.popup.contentItem
  tryVerify(function(){return list.itemAtIndex(3)!==null})
  click(list.itemAtIndex(3));tryCompare(preview.client,"serverSort","online")
  tryCompare(preview.client.serverActions,"guildStatsBusy",false)
  tryVerify(function(){return preview.model.callCount("guild_stats")===5})
  compare(preview.client.filteredGuilds[0].id,"5")
  compare(preview.client.filteredGuilds[4].id,"1")
  compare(preview.model.currentChannelId,before)
  preview.useRealServerActions=false
 }
 function test_compact_people_are_current_voice_not_server_members(){
  tests.width=520;tests.height=560;preview.client.setCompactMode(true)
  preview.model.voice={status:"connected",guildId:"1",channelId:"9002",muted:true,deafened:false}
  preview.model.voiceMembers={"1":[{channel_id:"9002",users:[{id:"201",display_name:"Voice friend"},{id:"202"}]},{channel_id:"9003",users:[{id:"999"}]}]}
  preview.model.membersWanted=false;wait(40)
  var bar=findChild(preview.client,"call-bar")
  var people=bar.roster
  verify(bar.visible);verify(people.visible);verify(people.voiceMode)
  verify(!preview.client.members.visible)
  compare(people.rows.length,2)
  compare(people.rows[0].id,"201")
  compare(people.rows[1].id,"202")
  verify(findChild(preview.client,"voice-person-201")!==null)
  verify(findChild(preview.client,"voice-person-999")===null)
  compare(findChild(people,"people-heading").text,"In voice")
  preview.model.selectedGuildId="another-server";wait(30)
  compare(people.rows[0].id,"201")
  preview.model.selectedGuildId="1"
  preview.model.currentChannelId="";wait(30);verify(bar.visible)
  click(findChild(preview.client,"navigation-button"));verify(preview.client.navigationShown)
  var drawer=findChild(preview.client,"navigation-drawer")
  verify(bar.mapToItem(preview.client,bar.width,0).x<=drawer.mapToItem(preview.client,0,0).x)
  verify(people.visible)
  click(findChild(preview.client,"navigation-button"))
  preview.model.voiceMembers={"1":[{channel_id:"9002",users:[{id:"202"}]}]};wait(30)
  compare(people.rows.length,1)
  preview.model.voice={status:"error",guildId:"1",channelId:"9002",error:"Session no longer valid"}
  wait(30);verify(people.visible);compare(people.headingText,"People in room")
  var retry=findChild(preview.client,"voice-reconnect");verify(retry.visible);verify(retry.enabled)
  compare(preview.model.callCount("voiceJoin"),0)
  click(retry);compare(preview.model.callCount("voiceJoin"),1)
  preview.model.voice={status:"idle"};wait(30);verify(!bar.visible)
  preview.model.currentChannelId="9000"
 }
 function test_voice_panel_survives_full_compact_and_member_toggle(){
  preview.model.voice={status:"connected",guildId:"1",channelId:"9002",muted:true,deafened:false}
  preview.model.voiceMembers={"1":[{channel_id:"9002",users:[{id:"201",display_name:"Voice friend"}]}]};wait(40)
  var bar=findChild(preview.client,"call-bar")
  verify(bar.roster.visible);compare(bar.roster.rows.length,1)
  tests.width=520;tests.height=560;click(findChild(preview.client,"layout-mode-button"))
  verify(bar.roster.visible);verify(bar.expanded)
  preview.client.toggleMembers();wait(30);verify(bar.roster.visible)
  click(findChild(preview.client,"navigation-button"));verify(bar.roster.visible)
  verify(findChild(preview.client,"navigation-drawer").x>=bar.x+bar.width)
  click(findChild(preview.client,"layout-mode-button"));tests.width=1040;wait(40)
  verify(bar.roster.visible);compare(bar.roster.rows.length,1)
  compare(preview.model.voice.status,"connected")
  compare(preview.model.callCount("voiceJoin"),0);compare(preview.model.callCount("voiceLeave"),0)
 }
 function test_disconnected_action_feedback(){
  preview.useRealServerActions=true;preview.model.connected=false
  preview.client.showServerMenu(preview.model.guilds[0],160,80);wait(40)
  click(findChild(preview.client,"server-mark-read"))
  verify(preview.client.serverMenu.shown)
  verify(preview.client.serverActions.actionError.indexOf("Not connected")>=0)
  verify(!preview.client.serverActions.guildActionBusy)
  compare(preview.model.callCount("mark_guild_read"),0)
  click(findChild(preview.client,"server-settings"))
  verify(preview.client.serverActions.settingsError.indexOf("Not connected")>=0)
  compare(preview.model.callCount("guild_settings"),0)
  preview.model.connected=true;preview.useRealServerActions=false
 }
 function test_compact_navigation_and_voice_mouse(){
  tests.width=520;tests.height=560;preview.client.setCompactMode(true);wait(40)
  click(findChild(preview.client,"navigation-button"));verify(preview.client.navigationShown)
  var sort=findChild(preview.client,"server-sort")
  click(sort);tryCompare(sort.popup,"visible",true)
  click(sort.popup.contentItem.itemAtIndex(1));compare(preview.client.serverSort,"name")
  click(findChild(preview.client,"navigation-button"));verify(!preview.client.navigationShown)
  preview.model.voice={status:"connected",guildId:"1",channelId:"9002",muted:true,deafened:true,error:""};wait(40)
  tests.width=420;tests.height=420;wait(40)
  var bar=findChild(preview.client,"call-bar")
  var previousRight=0
  for(var i=0;i<3;i++) {
   var button=findChild(preview.client,["voice-mute","voice-deafen","voice-leave"][i])
   verify(button.iconOnly)
   verify(button.implicitWidth<=button.width)
   var left=button.mapToItem(bar,0,0).x
   verify(left>=previousRight)
   previousRight=left+button.width
   verify(previousRight<=bar.width)
  }
  click(findChild(preview.client,"voice-mute"));compare(preview.model.callCount("toggleMute"),1)
  click(findChild(preview.client,"voice-deafen"));compare(preview.model.callCount("toggleDeafen"),1)
  click(findChild(preview.client,"voice-leave"));compare(preview.model.callCount("voiceLeave"),1)
  preview.model.voice={status:"idle"}
 }
 function test_filter_keeps_active_conversation(){
  var before=preview.model.currentChannelId
  preview.client.serverQuery="Music";wait(40)
  compare(preview.client.guildRows.length,2)
  compare(preview.client.guildRows[1].id,"2")
  compare(preview.model.currentChannelId,before)
  compare(preview.client.selectedGuildName,"Design workshop")
  preview.client.serverQuery="no such server";wait(40)
  compare(preview.client.guildRows.length,1)
  compare(preview.client.guildRows[0].id,"dms")
 }
 function test_right_click_does_not_navigate_and_escape(){
  var label=findChild(preview.client,"server-name-2")
  var before=preview.model.selectedGuildId
  mouseClick(label,label.width/2,label.height/2,Qt.RightButton);wait(60)
  verify(preview.client.serverMenu.shown)
  compare(preview.client.serverMenu.guildId,"2")
  compare(preview.model.selectedGuildId,before)
  verify(findChild(preview.client,"server-settings").activeFocus)
  keyClick(Qt.Key_Escape);wait(30)
  verify(!preview.client.serverMenu.shown)
 }
 function test_leave_requires_explicit_confirm(){
  preview.client.showServerMenu(preview.model.guilds[0],160,100);wait(40)
  click(findChild(preview.client,"server-leave"))
  compare(preview.client.serverMenu.page,"leave")
  verify(findChild(preview.client,"server-cancel").activeFocus)
  compare(preview.model.calls.length,0)
  click(findChild(preview.client,"server-cancel"))
  compare(preview.model.calls.length,0)
  preview.client.showServerMenu(preview.model.guilds[0],160,100);wait(40)
  click(findChild(preview.client,"server-leave"))
  click(findChild(preview.client,"server-leave-confirm"))
  compare(preview.model.callCount("leave_guild"),1)
  verify(!preview.client.serverMenu.shown)
 }
 function test_mark_read_settings_and_compact_bounds(){
  tests.width=640;tests.height=420;wait(40)
  preview.client.showServerMenu(preview.model.guilds[0],620,400);wait(40)
  var card=findChild(preview.client,"server-menu-card")
  verify(card.x>=0&&card.y>=0)
  verify(card.x+card.width<=preview.width&&card.y+card.height<=preview.height)
  click(findChild(preview.client,"server-mark-read"))
  compare(preview.model.callCount("mark_guild_read"),1)
  preview.client.showServerMenu(preview.model.guilds[0],160,80);wait(40)
  click(findChild(preview.client,"server-settings"))
  compare(preview.client.serverMenu.page,"settings")
  click(findChild(preview.client,"server-mute"))
  compare(preview.model.callCount("set_guild_mute"),1)
 }
 function test_tab_reaches_server_controls_and_keyboard_menu(){
  preview.client.zone="sidebar";preview.client.column="rail";preview.client.focusZone();wait(30)
  keyClick(Qt.Key_Tab);verify(findChild(preview.client,"server-tools-button").activeFocus)
  keyClick(Qt.Key_Tab);verify(findChild(preview.client,"server-search").activeFocus)
  keyClick(Qt.Key_Tab);verify(findChild(preview.client,"server-filter").activeFocus)
  keyClick(Qt.Key_Tab);verify(findChild(preview.client,"server-sort").activeFocus)
  keyClick(Qt.Key_Backtab);verify(findChild(preview.client,"server-filter").activeFocus)
  preview.client.serverSort="online";preview.client.focusStop("serverSort",1);wait(30)
  keyClick(Qt.Key_Tab);verify(findChild(preview.client,"server-count-refresh").activeFocus)
  preview.client.setGuildCursor(1);preview.client.focusStop("rail",1);wait(30)
  keyClick(Qt.Key_F10,Qt.ShiftModifier);wait(30);verify(preview.client.serverMenu.shown)
 }
 function test_search_typing_is_not_sidebar_shortcut(){
  var field=findChild(preview.client,"server-search")
  field.forceActiveFocus();keyClick(Qt.Key_M);keyClick(Qt.Key_R);wait(30)
  compare(preview.client.serverQuery,"mr")
  compare(preview.model.calls.length,0)
 }
}
