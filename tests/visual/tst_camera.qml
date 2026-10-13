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
  function voiceUsers(g,c){return [{id:"123",display_name:"Ada"}]}
  function sendCommand(name,p,callback){if(name==="video_watch" && rejectWatch && p.kind!=="stop"){callback(false,null,"Screen handshake timed out");return 1}calls=calls.concat([{name:name,user:p.user_id || "",kind:p.kind || ""}]);callback(true,{streams:[{user_id:"123",ssrc:42}],choices:[{user_id:"123",kind:"camera"},{user_id:"123",kind:"screen"}],kind:p.kind || "camera",user_id:name==="video_watch"?p.user_id:"",image:frameImage,age_ms:-1,error:cameraError},"");return 1}
 }
 Components.CameraView {id:camera;anchors.fill:parent;service:service}
 TestCase {
  name:"CameraMVP";when:windowShown
  function cleanup(){camera.hide();service.cameraError="";service.rejectWatch=false;service.frameImage=""}
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
  function test_escape_stops(){camera.open();camera.watch("123");camera.forceActiveFocus();keyClick(Qt.Key_Escape);compare(camera.shown,false);compare(service.calls[service.calls.length-1].user,"")}
 }
}
