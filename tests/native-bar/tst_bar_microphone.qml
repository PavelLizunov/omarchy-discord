import QtQuick
import QtTest

Item {
 width:320;height:110
 BarIconPreview {id:preview;anchors.fill:parent}
 TestCase {
  name:"NativeBarMicrophone";when:windowShown
  function test_native_button_and_keyboard(){
   tryCompare(preview,"auditReady",true)
   var mic=preview.find(preview,"bar-quick-mute")
   verify(mic!==null);verify(mic.enabled);verify(mic.visible)
   mic.forceActiveFocus();verify(mic.activeFocus)
   keyClick(Qt.Key_Return);verify(mic.active)
   keyClick(Qt.Key_Space);verify(!mic.active)
   var main=preview.find(preview,"bar-compact-toggle")
   verify(mic.x>=main.width)
   verify(preview.find(preview,"bar-primary-icon").iconStatus===Image.Ready)
  }
 }
}
