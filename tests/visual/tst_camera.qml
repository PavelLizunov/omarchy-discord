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
  function voiceUsers(g,c){return [{id:"123",display_name:"Ada"}]}
  function sendCommand(name,p,callback){calls=calls.concat([{name:name,user:p.user_id || ""}]);callback(true,{streams:[{user_id:"123",ssrc:42}],user_id:name==="voice_watch_camera"?p.user_id:"",image:"",age_ms:-1},"");return 1}
 }
 Components.CameraView {id:camera;anchors.fill:parent;service:service}
 TestCase {
  name:"CameraMVP";when:windowShown
  function test_choose_and_close(){
   camera.open();compare(camera.shown,true);compare(camera.sample.streams.length,1)
   camera.watch("123");compare(camera.sample.user_id,"123");compare(service.calls[service.calls.length-1].name,"voice_watch_camera")
   camera.hide();compare(camera.shown,false);compare(service.calls[service.calls.length-1].user,"");compare(camera.imageSource,"")
  }
  function test_escape_stops(){camera.open();camera.watch("123");camera.forceActiveFocus();keyClick(Qt.Key_Escape);compare(camera.shown,false);compare(service.calls[service.calls.length-1].user,"")}
 }
}
