import QtQuick
import QtTest
import "../../components" as Components

TestCase {
 id:tests;name:"ScreenShareLifecycle";width:941;height:508;visible:true;when:windowShown
 Preview { id:preview;anchors.fill:parent }
 QtObject {
  id:model
  property var voice:({status:"connected",guildId:"1",channelId:"9"})
  readonly property var backend:model
  property var calls:[]
  property var replies:[]
  function sendCommand(name,fields,cb){calls=calls.concat([fields]);replies=replies.concat([cb]);return 1}
  function reply(index,status){replies[index](true,{status:status},"")}
 }
 Components.ScreenShare {id:controller;service:model}
 function initTestCase(){tryCompare(preview,"ready",true)}
 function init(){
  model.voice={status:"idle"};model.calls=[];model.replies=[]
  model.voice={status:"connected",guildId:"1",channelId:"9"}
  model.reply(0,"idle");model.calls=[];model.replies=[]
  preview.model.voice={status:"connected",guildId:"1",channelId:"9002"}
  preview.client.open("{}");preview.client.setCompactMode(false)
  preview.client.sharing.sample={status:"idle"};preview.model.shareStatus="idle"
  findChild(preview.client,"call-bar").expanded=false
 }
 function cleanup(){controller.stop();model.voice={status:"idle"};preview.client.camera.hide();preview.model.voice={status:"idle"}}
 function test_stop_supersedes_pending_start(){
  controller.start();verify(controller.active);verify(controller.busy)
  controller.stop();compare(controller.status,"stopping");verify(!controller.busy)
  verify(model.calls[1].revision>model.calls[0].revision)
  model.reply(0,"selecting");compare(controller.status,"stopping")
  model.reply(1,"idle");compare(controller.status,"idle")
 }
 function test_stale_poll_cannot_overwrite_start(){
  controller.poll();controller.start();model.reply(0,"idle")
  compare(controller.status,"selecting");model.reply(1,"selecting")
 }
 function test_leave_discards_late_share_reply(){
  controller.start();model.voice={status:"idle"};model.reply(0,"sharing")
  compare(controller.status,"idle");verify(!controller.active)
 }
 function test_primary_controls_fit_and_remain_reachable_data(){return [{tag:"narrow",w:420,h:420},{tag:"actual",w:941,h:508},{tag:"wide",w:1040,h:680}]}
 function test_primary_controls_fit_and_remain_reachable(data){
  tests.width=data.w;tests.height=data.h;wait(40)
  for(var name of ["voice-mute","voice-deafen","screen-share","voice-watch","voice-leave"]){
   var button=findChild(preview.client,name);verify(button.visible);verify(button.enabled);verify(!button.iconOnly)
   var at=button.mapToItem(preview.client,0,0)
   verify(at.x>=0&&at.y>=0);verify(at.x+button.width<=tests.width);verify(at.y+button.height<=tests.height)
  }
  var share=findChild(preview.client,"screen-share")
  mouseClick(share,share.width/2,share.height/2);compare(preview.client.sharing.status,"selecting")
  compare(share.text,"Stop sharing")
  var leave=findChild(preview.client,"voice-leave")
  preview.model.reset();mouseClick(leave,leave.width/2,leave.height/2);compare(preview.model.callCount("voiceLeave"),1)
 }
 function test_viewer_and_main_close_keep_sharing_controls_state(){
  preview.client.sharing.start();preview.client.openCamera();preview.client.camera.hide()
  verify(preview.client.sharing.active)
  preview.client.close();verify(preview.client.sharing.active)
  preview.client.open("{}");verify(findChild(preview.client,"screen-share").visible)
  preview.client.sharing.stop();verify(!preview.client.sharing.active)
 }
}
