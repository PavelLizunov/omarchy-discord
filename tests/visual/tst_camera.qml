import QtQuick
import QtTest
import "../../components" as Components

Item {
 width:640;height:360
 QtObject {
  id:service
  property var voice:({status:"connected",channelId:"11",guildId:"10"})
  property var knownUsers:({})
  property var backend:service
  property var calls:[]
  property string cameraError:""
  property bool rejectWatch:false
 property string frameImage:""
 property string selectedUser:""
 property string selectedKind:"camera"
 property int selectedQuality:360
 property bool deferReplies:false
 property var replies:[]
 function deliver(){var next=replies[0];replies=replies.slice(1);next.callback(true,next.result,"")}
  function voiceUsers(g,c){return [{id:"123",display_name:"Ada"}]}
  function sendCommand(name,p,callback){if(name==="video_watch" && rejectWatch && p.kind!=="stop"){callback(false,null,"Screen handshake timed out");return 1}calls=calls.concat([{name:name,user:p.user_id || "",kind:p.kind || ""}]);if(name==="video_watch"){selectedUser=p.user_id || "";selectedKind=p.kind || "camera"}if(p.quality!==undefined)selectedQuality=p.quality;var result={streams:[{user_id:"123",ssrc:42}],choices:[{user_id:"123",kind:"camera"},{user_id:"123",kind:"screen"}],quality:selectedQuality,screen:{audio_volume:p.volume===undefined?100:p.volume},kind:selectedKind,user_id:selectedUser,image:frameImage,age_ms:-1,error:cameraError};if(deferReplies)replies=replies.concat([{callback:callback,result:result}]);else callback(true,result,"");return 1}
 }
 Components.CameraView {id:camera;anchors.fill:parent;service:service}
 TestCase {
  name:"CameraMVP";when:windowShown
  function cleanup(){service.deferReplies=false;camera.hide();service.replies=[];service.cameraError="";service.rejectWatch=false;service.frameImage="";service.selectedUser="";service.selectedKind="camera";service.selectedQuality=360}
  function test_backend_error_visible_and_cleared(){
   camera.open();service.cameraError="Camera changed. Select it again.";camera.watch("123")
   compare(camera.error,service.cameraError)
   camera.poll();compare(camera.error,service.cameraError)
   service.cameraError="";camera.poll();compare(camera.error,"")
  }
  function test_choose_and_close(){
   camera.open();compare(camera.shown,true);compare(camera.sample.streams.length,1)
   camera.watch("123");compare(camera.sample.user_id,"123");compare(service.calls[service.calls.length-1].name,"video_watch")
   camera.hide();compare(camera.shown,false);compare(service.calls[service.calls.length-1].user,"");compare(camera.imageSource,"")
  }
  function test_watch_failure_survives_poll(){camera.open();service.rejectWatch=true;camera.watch("123","screen");compare(camera.error,"Screen handshake timed out");camera.poll();compare(camera.error,"Screen handshake timed out");service.rejectWatch=false;camera.watch("123","camera");compare(camera.error,"")}
  function test_screen_select_and_stop(){camera.open();camera.watch("123","screen");compare(camera.sample.kind,"screen");compare(service.calls[service.calls.length-1].kind,"screen");camera.hide();compare(service.calls[service.calls.length-1].kind,"stop")}
  function test_frame_replacement_retains_ready_image(){
   camera.open()
   var image=findChild(camera,"camera-frame")
   service.frameImage="/9j/4AAQSkZJRgABAgAAAQABAAD//gAPTGF2YzYzLjEuMTAyAP/bAEMACAQEBAQEBQUFBQUFBgYGBgYGBgYGBgYGBgcHBwgICAcHBwYGBwcICAgICQkJCAgICAkJCgoKDAwLCw4ODhERFP/EAEwAAQEAAAAAAAAAAAAAAAAAAAAGAQEBAAAAAAAAAAAAAAAAAAAGBxABAAAAAAAAAAAAAAAAAAAAABEBAAAAAAAAAAAAAAAAAAAAAP/AABEIABAAEAMBIgACEQADEQD/2gAMAwEAAhEDEQA/AIsATX9//9k=";camera.poll()
   tryCompare(camera,"imageReady",true)
   image=camera.displayedFrame
   var previous=String(image.source)
   service.frameImage="/9j/4AAQSkZJRgABAgAAAQABAAD//gAPTGF2YzYzLjEuMTAyAP/bAEMACAQEBAQEBQUFBQUFBgYGBgYGBgYGBgYGBgcHBwgICAcHBwYGBwcICAgICQkJCAgICAkJCgoKDAwLCw4ODhERFP/EAEwAAQEAAAAAAAAAAAAAAAAAAAAHAQEBAAAAAAAAAAAAAAAAAAAFBxABAAAAAAAAAAAAAAAAAAAAABEBAAAAAAAAAAAAAAAAAAAAAP/AABEIABAAEAMBIgACEQADEQD/2gAMAwEAAhEDEQA/AI4Av4p//9k=";camera.poll()
   compare(String(image.source),previous)
   compare(image.status,Image.Ready)
   tryVerify(function(){return String(camera.displayedFrame.source)!==previous})
   compare(camera.imageReady,true)
   service.frameImage="";camera.poll();compare(camera.imageReady,true)
   camera.hide();compare(String(camera.displayedFrame.source),"")
  }
  function test_presentation_poll_has_one_request_in_flight(){
   camera.open();service.calls=[];wait(130)
   verify(service.calls.length>=3)
   service.deferReplies=true;camera.poll()
   var pending=service.replies.length;wait(110)
   compare(service.replies.length,pending)
  }
  function test_settings_use_separate_route(){
   camera.open();camera.watch("123","screen")
   camera.settings({volume:35});compare(service.calls[service.calls.length-1].name,"video_settings");compare(camera.sample.screen.audio_volume,35)
   camera.settings({quality:720});compare(camera.sample.quality,720)
  }
  function test_choice_click_during_poll_is_not_lost(){
   camera.open();wait(30);var choice=findChild(camera,"video-choice-screen-123");verify(choice!==null)
   service.deferReplies=true;camera.poll();verify(camera.busy)
   mouseClick(choice,choice.width/2,choice.height/2)
   service.deliver()
   compare(service.calls[service.calls.length-1].name,"video_watch")
   compare(service.calls[service.calls.length-1].kind,"screen")
   service.deliver();compare(camera.sample.kind,"screen")
   compare(camera.sample.user_id,"123")
  }
  function test_choices_are_not_recreated_by_image_polls(){
   camera.open();var choice=findChild(camera,"video-choice-screen-123")
   choice.forceActiveFocus();camera.poll();wait(10)
   compare(findChild(camera,"video-choice-screen-123"),choice);verify(choice.activeFocus)
  }
  function test_quality_keys_during_poll_queue_correct_value(){
   camera.open();camera.watch("123","screen")
   var quality=findChild(camera,"video-quality");quality.forceActiveFocus()
   service.deferReplies=true;camera.poll();verify(quality.enabled)
   keyClick(Qt.Key_Space);tryCompare(quality.popup,"visible",true)
   keyClick(Qt.Key_Down)
   service.deliver()
   keyClick(Qt.Key_Return)
   compare(service.calls[service.calls.length-1].name,"video_settings")
   service.deliver();compare(camera.sample.quality,720)
  }
  function test_escape_stops(){camera.open();camera.watch("123");camera.forceActiveFocus();keyClick(Qt.Key_Escape);compare(camera.shown,false);compare(service.calls[service.calls.length-1].user,"")}
 }
}
