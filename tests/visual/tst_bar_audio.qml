import QtQuick
import QtTest
import "../../components" as Components

TestCase {
 id:test;name:"BarCombinedAudio";when:windowShown
 property var replies:[]
 property var requests:[]
 QtObject {
  id:service
  property bool ready:true
  property var voice:({status:"connected",channelId:"1",muted:false,deafened:false})
  property var backend:service
  function sendCommand(name,fields,callback){test.requests.push(fields);test.replies.push(callback)}
  function fail(error){}
 }
 Components.BarAudioControl {id:control;service:service}
 function init(){control.reset();service.voice={status:"connected",channelId:"1",muted:false,deafened:false};test.replies=[];test.requests=[]}
 function test_reply_before_voice_does_not_unlock_stale_state(){
  verify(control.toggleSilence());verify(control.busy);verify(!control.toggleSilence())
  replies.shift()(true,{},"");verify(control.busy)
  service.voice={status:"connected",channelId:"1",muted:true,deafened:true};tryCompare(control,"busy",false)
  verify(!control.toggleMicrophone());verify(control.toggleSilence());compare(requests[1].muted,false);compare(requests[1].deafened,false)
  service.voice={status:"connected",channelId:"1",muted:false,deafened:false};verify(control.busy)
  replies.shift()(true,{},"");tryCompare(control,"busy",false);verify(!control.hasSavedMute)
 }
 function test_muted_restore_and_external_change(){
  service.voice={status:"connected",channelId:"1",muted:true,deafened:false}
  verify(control.toggleSilence());service.voice={status:"connected",channelId:"1",muted:true,deafened:true};replies.shift()(true,{},"")
  verify(control.toggleSilence());compare(requests[1].muted,true)
  service.voice={status:"connected",channelId:"1",muted:true,deafened:false};replies.shift()(true,{},"");verify(!control.hasSavedMute)
 }
 function test_disconnect_cancels_late_result(){
  verify(control.toggleSilence());verify(control.busy)
  service.voice={status:"idle",channelId:"",muted:false,deafened:false};tryCompare(control,"busy",false);verify(!control.hasSavedMute)
  replies.shift()(true,{},"");verify(!control.busy);verify(!control.toggleSilence())
 }
}
