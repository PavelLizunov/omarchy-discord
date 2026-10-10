import QtQuick
import QtTest
import "../../components" as Components

TestCase {
 id:tests;name:"VoiceDiagnostics";width:220;height:620;visible:true;when:windowShown
 property var callback:null
 property int reconnects:0
 property var result:({active:true,input_level:18,input_age_ms:10,sent_age_ms:10,received_age_ms:-1,decoded_age_ms:-1,output_age_ms:-1,encryption_ready:true,input_blocked:false,output_blocked:false,input_device:"USB microphone",output_device:"Headphones",send_errors:0,receive_errors:0,decode_errors:0})
 QtObject{id:model;property var backend:model;property var requests:[]
  function sendCommand(name,fields,cb){requests=requests.concat(name);if(name==="voice_diagnostics")tests.callback=cb;else cb(true,{},"")}
 }
 Components.VoiceDiagnostics{id:d;width:tests.width;height:tests.height;details:true;service:model;onReconnectRequested:tests.reconnects++}
 function init(){d.connected=false;d.reset();model.requests=[];callback=null;d.connected=true;verify(callback!==null);callback(true,result,"")}
 function cleanup(){d.connected=false}
 function test_honest_boundaries_and_explicit_output(){
  compare(d.micText,"Microphone: signal detected");compare(d.sendText,"Send: packets sent");compare(d.receiveText,"Receive: no recent audio");compare(d.outputText,"Output: no recent audio")
  compare(model.requests.length,1);var button=findChild(d,"voice-test-output");button.forceActiveFocus();keyClick(Qt.Key_Return)
  compare(model.requests[1],"voice_test_output");verify(d.testFeedback.indexOf("Did you hear it?")>=0)
  d.muted=true;compare(d.sendText,"Send: muted");d.muted=false
  d.sample=Object.assign({},result,{output_blocked:true});compare(d.outputText,"Output: system muted")
 }
 function test_recovery_and_progressive_details(){
  d.details=true;d.width=172;compare(d.deviceDetails,false);compare(d.statusRows.length,4)
  var reconnect=findChild(d,"voice-reconnect-audio");reconnect.forceActiveFocus();keyClick(Qt.Key_Return);compare(reconnects,1)
  compare(model.requests.length,1)
  var toggle=findChild(d,"voice-device-details");toggle.forceActiveFocus();keyClick(Qt.Key_Return);compare(d.deviceDetails,true)
  keyClick(Qt.Key_Return);compare(d.deviceDetails,false)
  d.focusTest();verify(d.testFocused);verify(d.focusNext(1));verify(reconnect.activeFocus);verify(d.focusNext(1));verify(toggle.activeFocus)
  d.reset();compare(d.deviceDetails,false);d.width=tests.width
 }
 function test_stale_sample_and_room_switch(){d.clock=d.sampledAt+3000;compare(d.fresh,false);var old=callback;d.callIdentity="new-room";compare(d.sample,null);old(true,result,"");compare(d.sample,null)}
 function test_stale_callback_cannot_repopulate_detached_call(){var old=callback;d.connected=false;old(true,result,"");compare(d.sample,null);compare(d.fresh,false)}
 function test_error_and_stale_capture_are_not_success(){d.error="synthetic failure";compare(d.sendText,"Send: unavailable");d.error="";d.sample=Object.assign({},result,{input_age_ms:9000});compare(d.micText,"Microphone: no capture")}
}
