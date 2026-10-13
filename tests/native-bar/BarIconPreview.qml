import QtQuick
import Quickshell
import qs.Commons as Host
import "../../" as Plugin
import "../../components/harness" as Harness

Rectangle {
 id:root
 width:320;height:110
 property bool muted:false
 property bool idle:false
 property bool light:false
 property bool auditReady:false
 property int mentions:0
 property bool unread:false
 property color ink:light?"#242424":"#d8dee9"
 color:light?"#fafafa":"#2e3440"
 Harness.MockService {
  id:service
  property bool ready:true
  property int totalMentionCount:root.mentions
  property bool anyUnread:root.unread
  function applySettings(values){}
  function toggleMute(){note("toggleMute");voice=Object.assign({},voice,{muted:!voice.muted});return true}
 }
 QtObject {
  id:host
  property bool vertical:false
  property int barSize:26
  property string position:"top"
  property color foreground:root.ink
  property color barForeground:root.ink
  property color urgent:root.light?"#9b303d":"#bf616a"
  property string fontFamily:Host.Style.font.family
  property bool foregroundAnimationEnabled:false
  property var clickTargets:[]
  property var activePopout:null
  property var shell:QtObject {function serviceFor(id){return service}}
  function hideTooltip(item){}
  function showTooltip(item,text){}
  function registerClickTarget(item){clickTargets=clickTargets.concat([item])}
  function unregisterClickTarget(item){clickTargets=clickTargets.filter(function(x){return x!==item})}
  function requestPopout(item){activePopout=item}
  function releasePopout(item){if(activePopout===item)activePopout=null}
 }
 Plugin.BarWidget {id:widget;bar:host;x:14;y:20}
 Text {x:14;y:68;text:root.idle?"No call · microphone disabled":root.muted?"Microphone muted":"Microphone on";color:root.ink;font.pixelSize:13}
 function find(item,name){if(item.objectName===name)return item;for(var i=0;i<item.children.length;i++){var r=find(item.children[i],name);if(r)return r}return null}
 function check(){
  service.voice={status:root.idle?"idle":"connected",guildId:"1",channelId:root.idle?"":"2",muted:root.muted,deafened:false}
  var mic=find(widget,"bar-quick-mute")
  if(!mic || !mic.visible)throw Error("microphone missing")
  if(mic.enabled===root.idle)throw Error("microphone admission incorrect")
  var before=service.callCount("toggleMute")
  mic.triggerPress(Qt.LeftButton)
  if(root.idle){if(service.callCount("toggleMute")!==before)throw Error("idle microphone changed flags")}
  else {
   if(service.callCount("toggleMute")!==before+1 || service.voice.muted===root.muted)throw Error("mute did not toggle")
   if(widget.opened || service.voice.deafened || service.voice.channelId!=="2")throw Error("microphone changed unrelated state")
   mic.triggerPress(Qt.LeftButton)
   if(service.voice.muted!==root.muted)throw Error("second click did not restore")
  }
  mic.triggerPress(Qt.RightButton)
  if(service.callCount("toggleMute")!==before+(root.idle?0:2))throw Error("right click toggled microphone")
  readyTimer.start()
 }
 Timer{id:start;interval:30;running:true;onTriggered:root.check()}
 Timer{id:readyTimer;interval:20;repeat:true;property int ticks:0;onTriggered:{
  var a=root.find(widget,"bar-primary-icon"),b=root.find(widget,"bar-mute-icon")
  if(a&&b&&a.iconStatus===Image.Ready&&b.status===Image.Ready){
   var mic=root.find(widget,"bar-quick-mute")
   if(b.width!==Math.round(a.width*0.75)||b.height!==b.width)throw Error("microphone hierarchy incorrect")
   if(mic.width!==Host.Style.bar.statusSlot||mic.height!==host.barSize)throw Error("microphone click target shrunk")
   var main=root.find(widget,"bar-compact-toggle")
   if(mic.x<main.x+main.width)throw Error("click targets overlap")
   if(root.mentions===0){
    var gap=b.mapToItem(widget,0,0).x-a.mapToItem(widget,a.width,0).x
    if(Math.abs(gap-Host.Style.space(1))>0.5)throw Error("bar icons must have one pixel canvas gap")
   }
   var dot=root.find(widget,"bar-unread-dot")
   if(dot.visible){
    var d=dot.mapToItem(widget,dot.width,dot.height),m=b.mapToItem(widget,0,0)
    if(d.x>m.x && d.y>m.y)throw Error("unread dot overlaps microphone")
   }
   root.auditReady=true;stop()
  }
  else if(++ticks>60)throw Error("icons not ready")
 }}
}
