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
 property string error: ""
 property string actionError: ""
 property bool busy: false
 property int generation: 0
 property bool selecting: false
 property string requestedUser: ""
 property string requestedKind: "camera"
 property string imageSource: ""
 property double selectedAt: 0
 property double clock: Date.now()
 property double requestRevision: 0
 function revision() {requestRevision=Math.max(Date.now()*1000,requestRevision+1);return requestRevision}
 signal closed()
 visible: shown
 function label(id) {
  var users = service && voice.channelId ? service.voiceUsers(String(voice.guildId || ""), String(voice.channelId)) : []
  for (var i=0;i<users.length;i++) if(String(users[i].id)===id) return Api.userLabel(users[i],service.knownUsers)
  return id
 }
 function reset() {generation++;deadline.stop();busy=false;selecting=false;requestedUser="";selectedAt=0;sample=({streams:[],user_id:"",image:"",age_ms:-1});imageSource="";error="";actionError=""}
 function open() {if(shown)return;reset();shown=true;poll();closeButton.forceActiveFocus()}
 function hide() {
  var stop=!!(requestedUser || sample.user_id)
  reset();shown=false
  if(stop && service && service.backend) service.backend.sendCommand("video_watch",{kind:"stop",user_id:"",revision:revision()},function(){})
  closed()
 }
 function command(name,params) {
  if(!service || !service.backend || busy)return
  busy=true;var token=generation;deadline.restart()
  service.backend.sendCommand(name,params,function(ok,result,reason) {
   if(token!==root.generation)return
   root.busy=false;root.selecting=false;deadline.stop()
   if(!ok && name==="video_watch")root.actionError=String(reason || "Video request failed")
   root.error=ok?"":String(reason || "Video request failed")
   if(ok) {
    root.sample=result || ({streams:[]})
    root.error=String(root.sample.error || root.actionError || "")
    root.imageSource=root.sample.image ? "data:image/jpeg;base64,"+root.sample.image : ""
    if(name==="video_watch")root.selectedAt=Date.now()
   }
  })
 }
 function poll() {if(shown && voice.status==="connected")command("video_state",{})}
 function watch(id,kind) {if(busy)return;actionError="";requestedUser=id;requestedKind=kind || "camera";selectedAt=Date.now();imageSource="";selecting=true;command("video_watch",{kind:requestedKind,user_id:id,revision:revision()})}
 onIdentityChanged: {reset();if(shown)poll()}
 onVisibleChanged: if(!visible && (requestedUser || sample.user_id)) hide()
 Component.onDestruction: {
  if((requestedUser || sample.user_id) && service && service.backend) service.backend.sendCommand("video_watch",{kind:"stop",user_id:"",revision:revision()},function(){})
 }
 Timer {interval:200;repeat:true;running:root.visible && root.voice.status==="connected";onTriggered:{root.clock=Date.now();root.poll()}}
 Timer {id:deadline;interval:18000;onTriggered:{root.generation++;root.busy=false;root.selecting=false;root.error="Video request timed out";if(root.service && root.service.backend)root.service.backend.sendCommand("video_watch",{kind:"stop",revision:root.revision()},function(){})}}
 Keys.onEscapePressed: function(event) {root.hide();event.accepted=true}
 BorderSurface {
  anchors.fill:parent;color:PluginTheme.Color.popups.background;radius:Style.cornerRadius;borderSpec:Border.none()
  Column {
   id:body;anchors.fill:parent;anchors.margins:Style.spacing.sm;spacing:Style.spacing.sm
   Item {
    width:parent.width;height:Style.spacing.controlHeight
    Text {anchors.left:parent.left;anchors.right:closeButton.left;anchors.verticalCenter:parent.verticalCenter;text:"Video · Camera and screen";elide:Text.ElideRight;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.body;font.bold:true}
    Button {id:closeButton;objectName:"camera-close";anchors.right:parent.right;text:"Close";focusable:true;tooltipText:"Stop watching and return to the channel";onClicked:root.hide()}
   }
   Controls.ScrollView {
    id:picker;width:parent.width;height:Math.min(Style.space(100),choices.implicitHeight)
    contentWidth:availableWidth;clip:true
    Column {
     id:choices;width:picker.availableWidth;spacing:Style.spacing.xxs
     Repeater {
      model:root.sample.choices || []
      Button {required property var modelData;width:choices.width;text:root.label(String(modelData.user_id))+" · "+(modelData.kind==="screen" ? "Screen" : "Camera");tooltipText:modelData.kind==="screen" ? "Watch this participant's screen on a separate connection" : "Watch this participant's camera";focusable:true;enabled:!root.busy || !root.selecting;selected:String(modelData.user_id)===String(root.sample.user_id || "") && modelData.kind===(root.sample.kind || "camera");onClicked:root.watch(String(modelData.user_id),modelData.kind)}
     }
    }
   }
   Item {
    id:picture;objectName:"camera-picture";width:parent.width;height:Math.max(0,body.height-y-footer.height-body.spacing);clip:true
    Rectangle {anchors.fill:parent;color:"black"}
    Image {id:frame;objectName:"camera-frame";anchors.fill:parent;source:root.imageSource;fillMode:Image.PreserveAspectFit;cache:false;asynchronous:true;Accessible.name:(root.sample.kind==="screen" ? "Screen of " : "Camera of ")+root.label(String(root.sample.user_id || ""))}
    Text {
     anchors.centerIn:parent;width:Math.max(0,parent.width-Style.spacing.sm*2);horizontalAlignment:Text.AlignHCenter;wrapMode:Text.WordWrap;visible:!root.imageSource
     text:root.error || (root.voice.status!=="connected" ? "Join voice to watch video" : (root.sample.user_id || root.requestedUser) ? (root.clock-root.selectedAt>10000 ? "No video frames received. H.264 only. Select again to retry." : "Waiting for video…") : (root.sample.choices || []).length ? "Choose a camera or screen above" : "No cameras or screen shares announced in this call")
     color:"white";font.family:Style.font.family;font.pixelSize:Style.font.bodySmall
    }
   }
   Text {id:footer;width:parent.width;text:root.error && root.imageSource ? root.error : "One video. H.264 only. Close stops watching, not voice.";wrapMode:Text.WordWrap;color:PluginTheme.Color.foreground;font.family:Style.font.family;font.pixelSize:Style.font.bodySmall}
  }
 }
}
