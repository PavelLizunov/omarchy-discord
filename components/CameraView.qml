pragma ComponentBehavior: Bound
import QtQuick
import "../ui" as PluginTheme
import QtQuick.Controls as Controls
import "../ui"
import "../Api.js" as Api

Item {
 id: root
 property var service: null
 property bool shown: false
 readonly property var voice: service && service.voice ? service.voice : ({})
 readonly property string identity: String(voice.channelId || "") + ":" + String(voice.status || "idle")
 property var sample: ({streams:[], user_id:"", image:"", age_ms:-1})
 property var streamChoices: []
 property var pendingWatch: null
 function applyChoices(choices) {var next=choices || [];if(JSON.stringify(next)!==JSON.stringify(streamChoices))streamChoices=next}
 function drainActions() {
  if(busy || !shown)return
  if(pendingWatch){var next=pendingWatch;pendingWatch=null;startWatch(next.user,next.kind);return}
  if(pendingSettings){var fields=pendingSettings;pendingSettings=null;command("video_settings",fields)}
 }
 property string error: ""
 property string actionError: ""
 property bool busy: false
 property int generation: 0
 property bool selecting: false
 property string requestedUser: ""
 property string requestedKind: "camera"
 property string imageSource: ""
 property int frontImage: 0
 readonly property var displayedFrame: frontImage===0 ? frame : pendingFrame
 readonly property var loadingFrame: frontImage===0 ? pendingFrame : frame
 readonly property bool imageReady: displayedFrame.status === Image.Ready
 function clearImage() {imageSource="";frame.source="";pendingFrame.source="";frontImage=0}
 function displayImage(image) {
  var source="data:image/jpeg;base64,"+image
  if(source===String(displayedFrame.source) || source===String(loadingFrame.source))return
  loadingFrame.source=source
  commitImage(loadingFrame)
 }
 function commitImage(loaded) {
  if(loaded!==loadingFrame || loaded.status!==Image.Ready || !shown)return
  imageSource=String(loaded.source)
  frontImage=frontImage===0 ? 1 : 0
 }
 property double selectedAt: 0
 property double clock: Date.now()
 property double requestRevision: 0
 property string pendingCommand: ""
 function revision() {requestRevision=Math.max(Date.now()*1000,requestRevision+1);return requestRevision}
 signal closed()
 signal moveRequested()
 signal resizeRequested()
 property var pendingSettings: null
 function settings(fields) {
  actionError=""
  if(busy || pendingWatch){pendingSettings=Object.assign({},pendingSettings || {},fields);return}
  command("video_settings",fields)
 }
 visible: shown
 function label(id) {
  var users = service && voice.channelId ? service.voiceUsers(String(voice.guildId || ""), String(voice.channelId)) : []
  for (var i=0;i<users.length;i++) if(String(users[i].id)===id) return Api.userLabel(users[i],service.knownUsers)
  return id
 }
 function reset() {pendingWatch=null;pendingSettings=null;streamChoices=[];generation++;deadline.stop();busy=false;selecting=false;requestedUser="";selectedAt=0;sample=({streams:[],user_id:"",image:"",age_ms:-1});clearImage();error="";actionError=""}
 function open() {if(shown)return;reset();shown=true;poll();closeButton.forceActiveFocus()}
 function hide() {
  var stop=!!(requestedUser || sample.user_id)
  reset();shown=false
  if(stop && service && service.backend) service.backend.sendCommand("video_watch",{kind:"stop",user_id:"",revision:revision()},function(){})
  closed()
 }
 function command(name,params) {
  if(!service || !service.backend || busy)return
  busy=true;pendingCommand=name;var token=generation;deadline.restart()
  service.backend.sendCommand(name,params,function(ok,result,reason) {
   if(token!==root.generation)return
   root.busy=false;root.selecting=false;deadline.stop()
   if(!ok && (name==="video_watch" || name==="video_settings"))root.actionError=String(reason || "Video request failed")
   root.error=ok?"":String(reason || "Video request failed")
   if(ok) {
    root.sample=result || ({streams:[]})
    root.applyChoices(root.sample.choices)
    root.error=String(root.sample.error || root.actionError || "")
    if(root.sample.image)root.displayImage(root.sample.image)
    if(!root.sample.user_id && root.requestedUser && !root.error)root.error="Stream ended or voice reconnected. Select a stream to resume."
    if(name==="video_watch")root.selectedAt=Date.now()
   }
   root.drainActions()
  })
 }
 function poll() {if(shown && voice.status==="connected"){drainActions();if(!busy)command("video_state",{})}}
 function watch(id,kind) {
  pendingSettings=null
  if(busy){pendingWatch={user:id,kind:kind || "camera"};return}
  startWatch(id,kind)
 }
 function startWatch(id,kind) {actionError="";requestedUser=id;requestedKind=kind || "camera";selectedAt=Date.now();clearImage();selecting=true;command("video_watch",{kind:requestedKind,user_id:id,revision:revision()})}
 onIdentityChanged: {reset();if(shown)poll()}
 onVisibleChanged: if(!visible && (requestedUser || sample.user_id)) hide()
 Component.onDestruction: {
  if((requestedUser || sample.user_id) && service && service.backend) service.backend.sendCommand("video_watch",{kind:"stop",user_id:"",revision:revision()},function(){})
 }
 Timer {interval:33;repeat:true;running:root.visible && root.voice.status==="connected";onTriggered:{root.clock=Date.now();root.poll()}}
 Timer {id:deadline;interval:18000;onTriggered:{root.generation++;root.busy=false;root.selecting=false;root.error="Video request timed out";if(root.pendingCommand==="video_watch" && root.service && root.service.backend)root.service.backend.sendCommand("video_watch",{kind:"stop",revision:root.revision()},function(){})}}
 Keys.onEscapePressed: function(event) {root.hide();event.accepted=true}
 BorderSurface {
  anchors.fill:parent;color:PluginTheme.Color.popups.background;radius:Style.cornerRadius;borderSpec:Border.none()
  Column {
   id:body;anchors.fill:parent;anchors.margins:Style.spacing.sm;spacing:Style.spacing.sm
   Item {
    width:parent.width;height:Style.spacing.controlHeight
    Text {anchors.left:parent.left;anchors.right:closeButton.left;anchors.verticalCenter:parent.verticalCenter;text:"Video · Camera and screen";elide:Text.ElideRight;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.body;font.bold:true}
    MouseArea {anchors.left:parent.left;anchors.right:closeButton.left;anchors.top:parent.top;anchors.bottom:parent.bottom;cursorShape:Qt.SizeAllCursor;onPressed:root.moveRequested()}
    Button {id:closeButton;objectName:"camera-close";anchors.right:parent.right;text:"Close";focusable:true;tooltipText:"Stop this video and its audio; keep voice connected";onClicked:root.hide()}
   }
   Controls.ScrollView {
    id:picker;width:parent.width;height:Math.min(Style.space(100),choices.implicitHeight)
    contentWidth:availableWidth;clip:true
    Column {
     id:choices;width:picker.availableWidth;spacing:Style.spacing.xxs
     Repeater {
      model:root.streamChoices
      Button {required property var modelData;objectName:"video-choice-"+modelData.kind+"-"+modelData.user_id;width:choices.width;text:root.label(String(modelData.user_id))+" · "+(modelData.kind==="screen" ? "Screen" : "Camera");tooltipText:modelData.kind==="screen" ? "Watch this participant's screen on a separate connection" : "Watch this participant's camera";focusable:true;enabled:root.voice.status==="connected";selected:String(modelData.user_id)===String(root.sample.user_id || "") && modelData.kind===(root.sample.kind || "camera");onClicked:root.watch(String(modelData.user_id),modelData.kind)}
     }
    }
   }
   Row {
    objectName:"video-settings";width:parent.width;spacing:Style.spacing.sm;height:Style.spacing.controlHeight
    Text {text:"Quality";anchors.verticalCenter:parent.verticalCenter;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.bodySmall}
    Controls.ComboBox {palette.button:PluginTheme.Color.popups.background;palette.buttonText:PluginTheme.Color.foreground;palette.base:PluginTheme.Color.popups.background;palette.text:PluginTheme.Color.foreground;palette.highlight:PluginTheme.Color.accent;id:quality;objectName:"video-quality";width:Style.space(98);model:["360p","720p","1080p"];currentIndex:0;enabled:!!root.sample.user_id&&!root.selecting;Accessible.name:"Video output size";onActivated:root.settings({quality:[360,720,1080][currentIndex]})
     function sync(){if(!popup.visible&&!root.pendingSettings&&!root.busy)currentIndex=root.sample.quality===1080?2:root.sample.quality===720?1:0}
     Connections {target:root;function onSampleChanged(){quality.sync()}function onBusyChanged(){quality.sync()}}
     Connections {target:quality.popup;function onVisibleChanged(){if(!quality.popup.visible)quality.sync()}}
    }
    Text {text:"Volume";visible:root.sample.kind==="screen";anchors.verticalCenter:parent.verticalCenter;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.bodySmall}
    Controls.Slider {id:volume;objectName:"video-volume";visible:root.sample.kind==="screen";width:Math.max(40,parent.width-quality.width-Style.space(190));from:0;to:200;stepSize:5;value:root.sample.screen?root.sample.screen.audio_volume:100;enabled:!!root.sample.user_id&&!root.selecting;Accessible.name:"Screen audio volume";onMoved:if(!pressed)root.settings({volume:Math.round(value)});onPressedChanged:if(!pressed&&enabled)root.settings({volume:Math.round(value)})
     implicitHeight:Style.spacing.controlHeight
     background:Rectangle {x:volume.leftPadding;y:volume.topPadding+(volume.availableHeight-height)/2;width:volume.availableWidth;height:Style.space(4);color:PluginTheme.Color.muted;Rectangle{width:volume.visualPosition*parent.width;height:parent.height;color:PluginTheme.Color.accent}}
     handle:BorderSurface {x:volume.leftPadding+volume.visualPosition*(volume.availableWidth-width);y:volume.topPadding+(volume.availableHeight-height)/2;width:Style.space(16);height:width;radius:width/2;color:PluginTheme.Color.foreground;borderSpec:volume.activeFocus?Border.controlSpec("focus",PluginTheme.Color.foreground,PluginTheme.Color.accent):Border.none()}
    }
    Text {text:Math.round(volume.value)+"%";visible:volume.visible;anchors.verticalCenter:parent.verticalCenter;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.bodySmall}
   }
   Item {
    id:picture;objectName:"camera-picture";width:parent.width;height:Math.max(0,body.height-y-footer.height-body.spacing);clip:true
    Rectangle {anchors.fill:parent;color:"black"}
    Image {id:frame;objectName:root.frontImage===0?"camera-frame":"camera-pending-frame";anchors.fill:parent;visible:root.frontImage===0;fillMode:Image.PreserveAspectFit;cache:false;asynchronous:true;onStatusChanged:root.commitImage(frame);Accessible.name:(root.sample.kind==="screen" ? "Screen of " : "Camera of ")+root.label(String(root.sample.user_id || ""))}
    Image {id:pendingFrame;objectName:root.frontImage===1?"camera-frame":"camera-pending-frame";anchors.fill:parent;visible:root.frontImage===1;fillMode:Image.PreserveAspectFit;cache:false;asynchronous:true;onStatusChanged:root.commitImage(pendingFrame);Accessible.name:frame.Accessible.name}
    Text {
     anchors.centerIn:parent;width:Math.max(0,parent.width-Style.spacing.sm*2);horizontalAlignment:Text.AlignHCenter;wrapMode:Text.WordWrap;visible:!root.imageSource
     text:root.error || (root.voice.status!=="connected" ? "Join voice to watch video" : (root.sample.user_id || root.requestedUser) ? (root.clock-root.selectedAt>10000 ? "No video frames received. H.264 only. Select again to retry." : "Waiting for video…") : (root.sample.choices || []).length ? "Choose a camera or screen above" : "No cameras or screen shares announced in this call")
     color:"white";font.family:Style.font.family;font.pixelSize:Style.font.bodySmall
    }
   }
   Text {id:footer;width:parent.width;text:root.error && root.imageSource ? root.error : "H.264 · Drag the title to move. Main window can close; this video stays open.";wrapMode:Text.WordWrap;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.bodySmall}
  }
  MouseArea {objectName:"video-resize";width:Style.space(16);height:width;anchors.right:parent.right;anchors.bottom:parent.bottom;cursorShape:Qt.SizeFDiagCursor;onPressed:root.resizeRequested()}
 }
}
