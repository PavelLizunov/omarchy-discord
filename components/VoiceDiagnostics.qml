pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls as Controls
import "../ui"

Item {
 id:root
 property var service:null
 property bool connected:false
 property bool muted:false
 property bool deafened:false
 property bool details:false
 signal reconnectRequested()
 property color foreground:Color.foreground
 property string fontFamily:Style.font.family
 property bool rejoining:false
 property bool deviceDetails:false
 readonly property bool testFocused:testButton.activeFocus || reconnectButton.activeFocus || deviceToggle.activeFocus
 readonly property var statusRows:[
  {label:"Microphone",value:!fresh?"Unavailable":sample.input_blocked===true?"System muted":muted?"Muted":sample.input_age_ms<0||sample.input_age_ms>1500?"No capture":sample.input_level>0?"Signal detected":"Quiet"},
  {label:"Sending",value:!fresh?"Unavailable":muted?"Muted":!sample.encryption_ready?"Securing call…":sample.sent_age_ms>=0&&sample.sent_age_ms<1500?"Packets sent":"No recent audio"},
  {label:"Receiving",value:!fresh?"Unavailable":sample.received_age_ms>=0&&sample.received_age_ms<1500?"Packets arriving":"No recent audio"},
  {label:"Speakers",value:!fresh?"Unavailable":deafened?"Deafened":sample.output_blocked===true?"System muted":sample.output_age_ms>=0&&sample.output_age_ms<1500?"Samples supplied":"No recent audio"}
 ]
 property var sample:null
 property string error:""
 property bool busy:false
 property int generation:0
 property bool testBusy:false
 property int testGeneration:0
 property string callIdentity:""
 property double sampledAt:0
 property double clock:Date.now()
 onCallIdentityChanged:{reset();poll()}
 property string testFeedback:""
 readonly property bool fresh:sample!==null && sample.active===true && !error && clock-sampledAt<2500
 readonly property string micText:!fresh ? "Microphone: unavailable" : sample.input_blocked===true ? "Microphone: system muted" : sample.input_age_ms<0 || sample.input_age_ms>1500 ? "Microphone: no capture" : muted ? "Microphone: muted in call" : sample.input_level>0 ? "Microphone: signal detected" : "Microphone: no signal"
 readonly property string sendText:!fresh ? "Send: unavailable" : muted ? "Send: muted" : !sample.encryption_ready ? "Send: encryption not ready" : sample.sent_age_ms>=0 && sample.sent_age_ms<1500 ? "Send: packets sent" : "Send: no recent audio"
 readonly property string receiveText:!fresh ? "Receive: unavailable" : sample.received_age_ms>=0 && sample.received_age_ms<1500 ? "Receive: packets arriving" : "Receive: no recent audio"
 readonly property string outputText:!fresh ? "Output: unavailable" : deafened ? "Output: deafened" : sample.output_blocked===true ? "Output: system muted" : sample.output_age_ms>=0 && sample.output_age_ms<1500 ? "Output: samples supplied" : "Output: no recent audio"
 function focusControl(button){button.forceActiveFocus();viewport.contentY=Math.max(0,Math.min(button.y,viewport.contentHeight-viewport.height))}
 function focusTest(){focusControl(testButton)}
 function focusControls(){return [testButton,reconnectButton,deviceToggle]}
 function focusNext(delta){
  var buttons=focusControls(),current=-1
  for(var i=0;i<buttons.length;i++)if(buttons[i].activeFocus)current=i
  var next=current<0?(delta<0?buttons.length-1:0):current+delta
  if(next<0||next>=buttons.length)return false
  focusControl(buttons[next]);return true
 }
 function reset(){generation++;testGeneration++;deadline.stop();testDeadline.stop();sample=null;error="";busy=false;testBusy=false;testFeedback="";details=false;deviceDetails=false}
 function poll(){
  if(!connected || !visible || busy || !service || !service.backend)return
  busy=true;var token=generation
  deadline.restart()
  service.backend.sendCommand("voice_diagnostics",{},function(ok,result,reason){
   if(token!==root.generation)return
   root.busy=false;deadline.stop()
   root.error=ok?"":String(reason||"Audio diagnostics unavailable")
   root.sample=ok?result:null
   root.sampledAt=Date.now();root.clock=root.sampledAt
  })
 }
 function testOutput(){
  if(!connected||!service||!service.backend||testBusy)return
  testBusy=true;testDeadline.restart();var token=++testGeneration
  service.backend.sendCommand("voice_test_output",{},function(ok,result,reason){
   if(token!==root.testGeneration)return
   root.testBusy=false;testDeadline.stop()
   root.testFeedback=ok?"A short tone was requested. Did you hear it?":String(reason||"Output test failed")
  })
 }
 onConnectedChanged:{reset();if(connected)poll()}
 onVisibleChanged:{if(visible)poll();else reset()}
 Timer{interval:1000;repeat:true;running:root.connected&&root.visible;onTriggered:{root.clock=Date.now();root.poll()}}
 Timer{id:testDeadline;interval:7000;onTriggered:{root.testGeneration++;root.testBusy=false;root.testFeedback="Output test timed out"}}
 Timer{id:deadline;interval:7000;onTriggered:{root.generation++;root.busy=false;root.sample=null;root.error="Audio diagnostics timed out"}}
 implicitHeight:details?body.implicitHeight:0
 Flickable {
  id:viewport
  anchors.fill:parent;visible:root.details;clip:true;contentHeight:body.implicitHeight;boundsBehavior:Flickable.StopAtBounds
  Column {
   id:body;width:parent.width;spacing:Style.spacing.sm
   Button{id:reconnectButton;objectName:"voice-reconnect-audio";width:parent.width;text:root.rejoining?"Reconnecting…":"Reconnect audio";leftAlign:true;foreground:root.foreground;fontFamily:root.fontFamily;iconName:"reconnect";tooltipText:"Briefly disconnect and rejoin this call.";Accessible.name:text;Accessible.description:tooltipText;focusable:true;enabled:root.connected&&!root.rejoining;onClicked:root.reconnectRequested()}
   Button{id:testButton;objectName:"voice-test-output";width:parent.width;text:"Test speakers";leftAlign:true;foreground:root.foreground;fontFamily:root.fontFamily;Accessible.name:text;Accessible.description:tooltipText;tooltipText:"Play a half-second tone locally through this call's output. Nothing is sent to Discord.";focusable:true;enabled:root.connected&&!root.testBusy;onClicked:root.testOutput()}
   Text{width:parent.width;visible:!!root.testFeedback;text:root.testFeedback;wrapMode:Text.WordWrap;color:root.foreground;font.family:root.fontFamily;font.pixelSize:Style.font.bodySmall}
   Repeater {
    model:root.statusRows
    Column {
     required property var modelData
     width:body.width;spacing:Style.spacing.xxs
     Text {width:parent.width;text:modelData.label;color:root.foreground;font.family:root.fontFamily;font.pixelSize:Style.font.bodySmall}
     Text {width:parent.width;text:modelData.value;wrapMode:Text.WordWrap;color:root.foreground;font.family:root.fontFamily;font.pixelSize:Style.font.body;Accessible.name:modelData.label+": "+text}
    }
   }
   Rectangle{width:parent.width;height:Style.space(5);color:Util.alpha(Color.foreground,0.15)
    Rectangle{height:parent.height;width:parent.width*(root.fresh?Math.max(0,Math.min(100,root.sample.input_level))/100:0);color:Color.accent}
    Accessible.name:"Microphone level";Accessible.description:root.micText
    MouseArea {id:readingHover;anchors.fill:parent;hoverEnabled:true;acceptedButtons:Qt.NoButton}
    PanelToolTip {visible:readingHover.containsMouse;text:"Sent packets do not confirm remote hearing. No received audio can mean silence or a receive fault. Supplied samples do not confirm audible speakers.";delay:1000}
   }
   Repeater {
    model:[root.error,root.fresh?String(root.sample.device_error||""):""]
    Text{required property string modelData;visible:!!modelData;width:body.width;text:modelData;textFormat:Text.PlainText;wrapMode:Text.WordWrap;color:Color.urgent;font.family:root.fontFamily;font.pixelSize:Style.font.bodySmall}
   }
   Button {id:deviceToggle;objectName:"voice-device-details";width:parent.width;text:root.deviceDetails?"Hide devices":"Devices and errors";leftAlign:true;foreground:root.foreground;fontFamily:root.fontFamily;focusable:true;onClicked:root.deviceDetails=!root.deviceDetails}
   Column {
    visible:root.deviceDetails;width:parent.width;spacing:Style.spacing.xs
    Repeater {
     model:[{label:"Input device",value:root.fresh&&root.sample.input_device?root.sample.input_device:"Unknown"},{label:"Output device",value:root.fresh&&root.sample.output_device?root.sample.output_device:"Unknown"}]
     Column {
      required property var modelData
      width:body.width;spacing:Style.spacing.xxs
      Text {width:parent.width;text:modelData.label;color:root.foreground;font.family:root.fontFamily;font.pixelSize:Style.font.bodySmall}
      Text {
       width:parent.width;text:modelData.value;textFormat:Text.PlainText;elide:Text.ElideRight;color:root.foreground;font.family:root.fontFamily;font.pixelSize:Style.font.bodySmall
       Accessible.description:text
       MouseArea {id:deviceHover;anchors.fill:parent;hoverEnabled:true;acceptedButtons:Qt.NoButton}
       PanelToolTip {visible:deviceHover.containsMouse;text:parent.text;delay:1000}
      }
     }
    }
    Text {width:parent.width;text:root.fresh?"Send errors: "+root.sample.send_errors+"\nReceive errors: "+root.sample.receive_errors+"\nDecode errors: "+root.sample.decode_errors:"Errors unavailable";wrapMode:Text.WordWrap;color:root.foreground;font.family:root.fontFamily;font.pixelSize:Style.font.bodySmall}
   }

  }
  Controls.ScrollBar.vertical:Controls.ScrollBar{policy:Controls.ScrollBar.AsNeeded}
 }
}
