import QtQuick
import Quickshell
import "../../components" as Components

ShellRoot {
 id: root
 property int phase:0
 property int count:0
 property string copyPath:""
 property QtObject fixture:QtObject {
  property bool ready:true
  property double lastGeneration:1
  property var daemon:({pluginDir:"/src"})
  property string stagedDir:"/tmp/drop-staged"
  function isOpen(id){return id==="fixture"}
  function stagedFor(id){return []}
  function fail(text){console.error("FAIL: "+text);Qt.exit(1)}
  function stageFile(id,path,size){root.copyPath=path;root.count++}
 }
 property Components.DroppedFileStager worker:Components.DroppedFileStager {
  service:root.fixture
  onStaged:function(id){
   if(root.phase!==0||root.count!==1||id!=="fixture"){console.error("FAIL: unexpected staged result");Qt.exit(1);return}
   root.phase=1
   next.start()
  }
 }
 property Timer next:Timer {interval:20;onTriggered:{if(!root.worker.stage("fixture",["file:///tmp/drop-source.txt"])) {console.error("FAIL: second start");Qt.exit(1);return};root.fixture.lastGeneration=2;finish.start()}}
 property Timer begin:Timer {interval:50;running:true;onTriggered:if(!root.worker.stage("fixture",["file:///tmp/drop-source.txt"]))Qt.exit(1)}
 property Timer finish:Timer {
  interval:300
  onTriggered:{
   if(root.worker.busy){console.error("FAIL: worker did not finish");Qt.exit(1);return}
   if(root.count!==1){console.error("FAIL: old generation staged");Qt.exit(1);return}
   console.log("PASS: native stager copied local file and rejected stale generation",root.copyPath)
   Qt.quit()
  }
 }
}
