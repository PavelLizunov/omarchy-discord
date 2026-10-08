import QtQuick
import QtTest
import "../../ui"

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
  preview.client.optionsMenu.shown=false
  preview.model.voice={status:"idle"};preview.model.membersWanted=true
  preview.model.selectedGuildId="1"
  preview.model.currentChannelId="9000";preview.model.reset();wait(50)
 }
 function click(item){mouseClick(item,item.width/2,item.height/2);wait(60)}
 function test_server_click_browses_only_then_explicit_channel_opens_chat(){
  tests.width=520;tests.height=560;preview.client.setCompactMode(true)
  preview.model.channelsByGuild={"1":[{id:"9000",guild_id:"1",name:"general",type:"text"},{id:"9001",guild_id:"1",name:"development",type:"text"},{id:"9002",guild_id:"1",name:"Voice lounge",type:"voice"}],"2":[{id:"9200",guild_id:"2",name:"general",type:"text"},{id:"9201",guild_id:"2",name:"Voice room",type:"voice"}]}
  preview.model.selectedGuildId="1";preview.model.currentChannelId="9000"
  preview.client.enterComposer();wait(30)
  preview.client.composer.focusInput()
  keyClick(Qt.Key_K);keyClick(Qt.Key_E);keyClick(Qt.Key_E);keyClick(Qt.Key_P)
  preview.model.reset()
  var rail=findChild(preview.client,"server-rail"),channels=findChild(preview.client,"channel-pane"),chat=findChild(preview.client,"chat-column")
  preview.client.compactChatOpen=false;wait(30)
  click(findChild(preview.client,"server-name-2"))
  compare(preview.model.selectedGuildId,"2");compare(preview.model.currentChannelId,"")
  verify(rail.visible);verify(channels.visible);verify(!chat.visible)
  compare(preview.client.channelRows.length,2);compare(preview.model.callCount("showChannel"),0)
  compare(preview.model.drafts["9000"],"keep")
  // A delayed list completion must not take over the screen.
  preview.model.channelsByGuild=Object.assign({},preview.model.channelsByGuild);wait(30)
  verify(!chat.visible)
  preview.client.activateChannel(0,"channels");wait(30)
  compare(preview.model.currentChannelId,"9200");verify(chat.visible)
  click(findChild(preview.client,"back-to-channels"));verify(channels.visible)
  click(findChild(preview.client,"server-name-2"))
  compare(preview.model.currentChannelId,"");verify(!chat.visible)
  click(findChild(preview.client,"server-name-1"))
  compare(preview.model.currentChannelId,"");verify(channels.visible)
  preview.client.activateChannel(0,"channels");wait(30)
  compare(preview.client.composer.text,"keep")
  preview.client.composer.setText("")
 }
 function test_server_browse_during_call_retains_room_draft_and_chat_return(){
  tests.width=520;tests.height=560;preview.client.serverToolsShown=false;preview.client.setCompactMode(true)
  preview.model.voice={status:"connected",guildId:"1",channelId:"9002",muted:true,deafened:false}
  preview.model.channelsByGuild={"1":[{id:"9000",guild_id:"1",name:"general",type:"text"},{id:"9002",guild_id:"1",name:"Voice lounge",type:"voice"}],"2":[{id:"9200",guild_id:"2",name:"other chat",type:"text"}]}
  preview.client.enterComposer();preview.client.composer.focusInput();keyClick(Qt.Key_K);keyClick(Qt.Key_E);keyClick(Qt.Key_E);keyClick(Qt.Key_P)
  preview.client.compactChatOpen=false;wait(30);preview.model.reset()
  click(findChild(preview.client,"server-name-2"));compare(preview.model.currentChannelId,"");compare(preview.model.selectedGuildId,"2")
  compare(preview.model.voice.channelId,"9002");compare(preview.model.voice.muted,true)
  preview.client.activateChannel(0,"channels");wait(30);compare(preview.model.currentChannelId,"9200")
  click(findChild(preview.client,"back-to-channels"));verify(!findChild(preview.client,"chat-column").visible)
  click(findChild(preview.client,"server-name-1"));preview.client.activateChannel(0,"channels");wait(30)
  compare(preview.client.composer.text,"keep")
  compare(preview.model.callCount("voiceJoin"),0);compare(preview.model.callCount("voiceLeave"),0)
  preview.client.composer.setText("")
 }
 function test_compact_reopen_with_existing_channel_starts_at_navigation(){
  tests.width=520;tests.height=560;preview.client.setCompactMode(true)
  preview.model.currentChannelId="9000";preview.client.compactChatOpen=true
  preview.client.close();preview.client.open("{}");wait(30)
  verify(findChild(preview.client,"server-rail").visible)
  verify(findChild(preview.client,"channel-pane").visible)
  verify(!findChild(preview.client,"chat-column").visible)
  preview.client.open('{"channel_id":"9000"}');wait(30)
  verify(findChild(preview.client,"chat-column").visible)
 }
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
  verify(findChild(preview.client,"chat-column").visible)
  preview.client.compactChatOpen = true; wait(40)
  verify(!findChild(preview.client,"server-rail").visible)
  verify(!findChild(preview.client,"channel-pane").visible)
  verify(preview.client.timeline.width>280)
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
  var rail=findChild(preview.client,"server-rail")
  verify(rail.mapToItem(preview.client,rail.width,0).x<=bar.mapToItem(preview.client,0,0).x)
  verify(people.visible)
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
  var channels=findChild(preview.client,"channel-pane")
  verify(channels.mapToItem(preview.client,0,channels.height).y<=bar.mapToItem(preview.client,0,0).y)
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
  var sort=findChild(preview.client,"server-sort")
  click(sort);tryCompare(sort.popup,"visible",true)
  click(sort.popup.contentItem.itemAtIndex(1));compare(preview.client.serverSort,"name")
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
  var chips=findChild(preview.client,"server-filter-chips")
  var buttons=preview.client.filterButtons(chips)
  for(var i=0;i<buttons.length;i++){keyClick(Qt.Key_Tab);verify(buttons[i].activeFocus)}
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
 function test_archive_and_unarchive_servers_via_menu_and_rail(){
  preview.useRealServerActions=true
  compare(preview.client.serverActions.archivedCount,0)
  var initialCount=preview.client.guildRows.length
  preview.client.showServerMenu(preview.model.guilds[1],160,100);wait(40)
  var archiveBtn=findChild(preview.client,"server-archive")
  verify(archiveBtn!==null);compare(archiveBtn.text,"Archive server")
  click(archiveBtn)
  compare(preview.client.serverActions.archivedCount,1)
  verify(!preview.client.serverMenu.shown)
  var hasArchiveRow=false
  for(var i=0;i<preview.client.guildRows.length;i++){
    if(preview.client.guildRows[i].kind==="archive_entry") hasArchiveRow=true
  }
  verify(hasArchiveRow)
  var foundHidden=false
  for(var j=0;j<preview.client.filteredGuilds.length;j++){
    if(preview.client.filteredGuilds[j].id==="2") foundHidden=true
  }
  verify(!foundHidden)
  preview.client.serverFilter="archive";wait(40)
  compare(preview.client.filteredGuilds.length,1)
  compare(preview.client.filteredGuilds[0].id,"2")
  preview.client.showServerMenu(preview.model.guilds[1],160,100);wait(40)
  compare(archiveBtn.text,"Unarchive server")
  click(archiveBtn)
  compare(preview.client.serverActions.archivedCount,0)
  preview.client.serverFilter="all";wait(40)
  compare(preview.client.guildRows.length,initialCount)
  preview.useRealServerActions=false
 }
 function test_voice_chat_opens_without_join_until_explicit_action(){
  preview.client.zone="sidebar";preview.client.column="channels";preview.client.focusZone();wait(30)
  var voiceIndex=-1
  for(var i=0;i<preview.client.channelRows.length;i++){
    if(preview.client.channelRows[i].id==="9002"){voiceIndex=i;break}
  }
  verify(voiceIndex>=0)
  preview.client.activateChannel(voiceIndex,"channels");wait(40)
  compare(preview.client.currentChannelId,"9002")
  compare(preview.model.callCount("voiceJoin"),0)
  var join=findChild(preview.client,"join-channel-voice");verify(join.visible)
  click(join);compare(preview.model.callCount("voiceJoin"),1)
  var chatBtn=findChild(preview.client,"voice-chat-button")
  verify(chatBtn!==null)
  click(chatBtn)
  compare(preview.client.currentChannelId,"9002")
 }
 function test_options_menu_shows_help_and_logout(){
  var opt=findChild(preview.client,"options-button")
  verify(opt!==null)
  click(opt);wait(30)
  verify(preview.client.optionsMenu.shown)
  var help=findChild(preview.client,"menu-help")
  verify(help!==null)
  click(help);wait(30)
  verify(preview.client.cheatsheet.shown)
  verify(!preview.client.optionsMenu.shown)
  keyClick(Qt.Key_Escape);wait(30)
  verify(!preview.client.cheatsheet.shown)
  click(opt);wait(30)
  var logout=findChild(preview.client,"menu-logout")
  verify(logout!==null)
  click(logout)
  tryCompare(preview.client.logoutConfirmation,"shown",true)
  verify(!preview.client.optionsMenu.shown)
  keyClick(Qt.Key_Escape);wait(30)
  tryCompare(preview.client.logoutConfirmation,"shown",false)
 }
 function test_options_menu_settings_persistence_and_preservation(){
  preview.model.entry={dummyUnrelated:"keep_this",otherMeta:42}
  preview.model.settings={notifications:"Mentions and DMs",stayConnected:"On"}
  var opt=findChild(preview.client,"options-button")
  click(opt);wait(30)
  verify(preview.client.optionsMenu.shown)
  var notifs=findChild(preview.client,"menu-notifications")
  verify(notifs!==null)
  compare(notifs.text,"Notifications: Mentions")
  click(notifs);wait(30)
  compare(preview.model.settings.notifications,"Off")
  compare(notifs.text,"Notifications: Off")
  compare(preview.model.entry.dummyUnrelated,"keep_this")
  compare(preview.model.entry.otherMeta,42)
  var stay=findChild(preview.client,"menu-stay-connected")
  verify(stay!==null)
  compare(stay.text,"Stay connected: On")
  click(stay);wait(30)
  compare(preview.model.settings.stayConnected,"Off")
  compare(stay.text,"Stay connected: Off")
  compare(preview.model.entry.dummyUnrelated,"keep_this")
  keyClick(Qt.Key_Escape);wait(30)
  verify(!preview.client.optionsMenu.shown)
 }
 function test_tooltip_suppressed_on_settings_and_themed_on_regular_buttons(){
  var opt=findChild(preview.client,"options-button")
  verify(opt!==null)
  compare(opt.tooltipText,"")
  var userSettings=findChild(preview.client,"user-bar-settings")
  verify(userSettings!==null)
  compare(userSettings.tooltipText,"")
  var mode=findChild(preview.client,"layout-mode-button")
  verify(mode!==null)
  verify(mode.tooltipText!=="")
  verify(mode.tooltipItem!==null && mode.tooltipItem!==undefined)
  compare(String(mode.tooltipItem.panelBackground),String(Color.tooltip.background))
 }
 function test_empty_channel_screen_has_browse_and_find_buttons_in_compact(){
  tests.width=520;tests.height=560;preview.client.setCompactMode(true)
  preview.model.currentChannelId=""
  preview.client.restoreView();wait(30)
  var rail=findChild(preview.client,"server-rail")
  var channels=findChild(preview.client,"channel-pane")
  verify(rail!==null);verify(rail.visible)
  verify(channels!==null);verify(channels.visible)
  preview.model.currentChannelId="9000"
 }
}
