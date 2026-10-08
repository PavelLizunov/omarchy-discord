import QtQuick
import QtTest

TestCase {
  id: tests
  name: "Usability"
  visible: true
  width: 1040
  height: 680
  when: windowShown
  Preview { id: preview; anchors.fill: parent }
  function click(item) { wait(50); mouseClick(item, item.width / 2, item.height / 2); wait(50) }
  function control(text) {
    if (text === "Help") {
      preview.client.optionsMenu.show(); wait(40)
      return preview.client.helpButton
    }
    if (text === "Log out") {
      preview.client.optionsMenu.show(); wait(40)
      return preview.client.logoutButton
    }
    var children = preview.client.controls.children
    for (var i=0;i<children.length;i++) if (children[i].text === text) return children[i]
    fail("Missing control: " + text)
  }
  function action(text) {
    var children = preview.client.timeline.messageActions.children
    for (var i=0;i<children.length;i++) if (children[i].text === text) return children[i]
    fail("Missing message action: " + text)
  }
  function initTestCase() { tryCompare(preview, "ready", true) }
  function init() {
    tests.width=1040; tests.height=680
    preview.model.selectedGuildId="1";preview.model.currentChannelId="9000"
    preview.model.reset(); preview.model.staged=({}); preview.model.membersWanted=false
    preview.client.logoutConfirmation.shown=false
    preview.client.cheatsheet.shown=false; preview.client.picker.shown=false
    preview.client.composer.cancelEdit(); preview.client.composer.cancelReply()
    preview.client.composer.setText("")
    preview.client.compactMode=false;preview.client.navigationShown=false
    preview.client.open("{}"); preview.client.zone="timeline"
    preview.model.membersWanted=false
    preview.client.timeline.focusNewest(); preview.client.focusZone()
    wait(80)
  }
  function test_attachments_bounded_and_focus_scrolled() {
    tests.width=640; tests.height=420; wait(40)
    var controls=preview.client.controls
    var controlsX=controls.mapToItem(preview.client,0,0).x
    verify(controlsX>=0)
    verify(controlsX+controls.width<=preview.client.width)
    var files=[]
    for(var i=0;i<10;i++) files.push({path:"/inert/"+i,filename:"fixture-"+i+".png",size:12345})
    preview.model.staged={"9000":files}; wait(60)
    verify(preview.client.timeline.height>=140)
    verify(preview.client.composer.attachmentViewport.height<=80)
    var hint=findChild(preview.client,"shortcut-hint")
    verify(!hint.visible)
    preview.client.zone="composer"; preview.client.composer.focusInput(); wait(30)
    compare(hint.text,"Enter to send · Shift+Enter for a new line")
    verify(hint.contentWidth<=hint.width)
    preview.client.composer.focusChip(9); wait(60)
    verify(preview.client.composer.attachmentViewport.contentY>0)
    verify(preview.client.composer.attachmentViewport.contentY+preview.client.composer.attachmentViewport.height
      >= preview.client.composer.attachmentViewport.contentHeight-1)
  }
  function test_servers_display_names_and_keep_selection() {
    var label=findChild(preview.client,"server-name-1")
    verify(label!==null)
    compare(label.text,"Fixture server")
    var dm=findChild(preview.client,"server-name-dms")
    verify(dm!==null)
    compare(dm.text,"Direct Messages")
    mouseClick(dm,dm.width/2,dm.height/2);wait(40)
    compare(preview.model.selectedGuildId,"dms")
    preview.client.leaveChannels();preview.client.setGuildCursor(1)
    preview.client.enterChannels();wait(40)
    compare(preview.model.selectedGuildId,"1")
    tests.width=640;tests.height=420;wait(40)
    verify(preview.client.timeline.width>200)
    verify(findChild(preview.client,"server-rail").width>=140)
    preview.model.guilds=[{id:"1",name:"Community with a deliberately long server name",unread:"mentioned",mention_count:12}]
    wait(40)
    label=findChild(preview.client,"server-name-1")
    compare(label.text,"Community with a deliberately long server name")
    verify(label.height<=48)
  }
  function test_send_edit_and_disabled_controls() {
    verify(!preview.client.composer.sendControl.enabled)
    preview.client.composer.setText("Inert click send")
    click(preview.client.composer.sendControl)
    compare(preview.model.lastCall("sendMessage"),"Inert click send")
    compare(preview.client.composer.text,"")
    verify(preview.client.composer.startEditLast())
    compare(preview.client.composer.sendControl.text,"Save")
    preview.client.composer.setText("Inert click edit")
    click(preview.client.composer.sendControl)
    compare(preview.model.lastCall("editMessage"),"Inert click edit")
    verify(!preview.client.composer.editing)
    preview.model.staged={"9000":[{uploading:true,path:"/inert/1"}]}
    verify(!preview.client.composer.sendControl.enabled)
  }
  function test_search_help_and_focus() {
    click(control("Search"));compare(preview.model.callCount("openSwitcher"),1)
    preview.client.zone="composer";preview.client.focusZone()
    preview.client.composer.setText("Draft remains")
    click(control("Help"));tryCompare(preview.client.cheatsheet,"shown",true)
    keyClick(Qt.Key_Escape);tryCompare(preview.client.cheatsheet,"shown",false)
    compare(preview.client.composer.text,"Draft remains")
    verify(preview.client.composer.inputFocused)
  }
  function test_narrow_members_beside_chat_and_dismiss() {
    tests.width=640;tests.height=420;wait(40)
    var before=preview.client.timeline.width
    click(control("Members"));tryCompare(preview.model,"membersWanted",true)
    verify(preview.client.timeline.width<before)
    verify(preview.client.members.width>=120)
    var chat=findChild(preview.client,"chat-column")
    verify(chat.x+chat.width<=preview.client.members.x)
    compare(preview.client.zone,"members")
    keyClick(Qt.Key_Escape);tryCompare(preview.model,"membersWanted",false)
    compare(preview.client.zone,"composer")
    tests.width=1040;wait(40)
    before=preview.client.timeline.width
    click(control("Members"));verify(preview.client.timeline.width<before)
    click(control("Members"));tryCompare(preview.client.timeline,"width",before)
  }
  function test_logout_requires_confirmation_and_cancel_default() {
    click(control("Log out"));tryCompare(preview.client.logoutConfirmation,"shown",true)
    compare(preview.model.callCount("logout"),0)
    keyClick(Qt.Key_Return);tryCompare(preview.client.logoutConfirmation,"shown",false)
    compare(preview.model.callCount("logout"),0)
    click(control("Log out"));wait(20)
    keyClick(Qt.Key_Tab);keyClick(Qt.Key_Return)
    tryCompare(preview.client.logoutConfirmation,"shown",false)
    compare(preview.model.callCount("logout"),1)
    click(control("Log out"));keyClick(Qt.Key_Escape)
    verify(!preview.client.logoutConfirmation.shown)
    compare(preview.model.callCount("logout"),1)
  }
  function test_message_mouse_actions_and_guarded_delete() {
    click(action("Reply"));verify(preview.client.composer.replying)
    preview.client.composer.cancelReply();preview.client.zone="timeline";preview.client.focusZone()
    click(action("React"));tryCompare(preview.client.picker,"shown",true)
    keyClick(Qt.Key_Escape);wait(30)
    click(action("Copy"));verify(preview.copiedText.length>0)
    preview.client.timeline.cursorMessageId=preview.model.lastOwnMessageId("9000");wait(30)
    click(action("Edit"));verify(preview.client.composer.editing)
    preview.client.composer.cancelEdit();preview.client.zone="timeline";preview.client.focusZone()
    click(action("Delete"));compare(preview.model.callCount("deleteMessage"),0)
    click(action("Confirm delete"));compare(preview.model.callCount("deleteMessage"),1)
    preview.model.reset()
    action("Delete").forceActiveFocus();wait(20)
    keyClick(Qt.Key_Return);compare(preview.model.callCount("deleteMessage"),0)
    keyClick(Qt.Key_Return);compare(preview.model.callCount("deleteMessage"),1)
  }
  function test_tab_visits_message_actions_and_send() {
    keyClick(Qt.Key_Tab);verify(action("Reply").activeFocus)
    keyClick(Qt.Key_Return);verify(preview.client.composer.replying)
    preview.client.composer.cancelReply();preview.client.composer.setText("Keep draft")
    preview.client.composer.focusInput();wait(30);keyClick(Qt.Key_Tab)
    verify(preview.client.composer.sendControl.activeFocus)
    keyClick(Qt.Key_Backtab);verify(preview.client.composer.inputFocused)
    compare(preview.client.composer.text,"Keep draft")
    preview.client.composer.setText("");preview.client.composer.focusInput();wait(20)
    keyClick(Qt.Key_Tab);verify(control("Search").activeFocus)
  }
}
