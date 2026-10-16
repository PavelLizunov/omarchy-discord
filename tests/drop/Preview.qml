import QtQuick
import "../visual" as Visual
Rectangle {
 id:root
 width:1040;height:680
 readonly property int sendCalls:preview.model.callCount("sendMessage")+preview.model.callCount("upload")
 property int stagedCount:0
 property string lastPath:""
 readonly property bool ready:preview.ready
 property real dropX:700
 property real dropY:300
 property alias currentChannel:preview.model.currentChannelId
 Visual.Preview {id:preview;anchors.fill:parent}
 property QtObject stager:QtObject {
  property bool busy:false
  function stage(id,urls) {
   root.stagedCount++
   root.lastPath=decodeURIComponent(String(urls[0]).replace(/^file:\/\//,""))
   var next=Object.assign({},preview.model.staged);next[id]=[{path:root.lastPath,filename:"Fixture file",size:14}];preview.model.staged=next
   return true
  }
 }
 Component.onCompleted:{preview.client.fileStager=stager;preview.client.open("{}");preview.client.setCompactMode(false)}
}
