import QtQuick
import Quickshell
import "." as Components
ShellRoot {
 id: root
 property bool failureExpected: false
 property int errors: 0
 property QtObject fixture: QtObject {function fail(text){root.errors++}}
 property Components.ExternalLink opener: Components.ExternalLink {
  service: root.fixture
  onFinished: function(ok) {
    if(ok===root.failureExpected || (root.failureExpected && root.errors!==1)){console.error("FAIL: handoff result");Qt.exit(1);return}
    console.log("PASS: handoff status",status,"exit",exitCode)
    Qt.quit()
  }
 }
 property Timer start: Timer {interval:20;running:true;onTriggered:{if(root.opener.open("file:///tmp/rejected")){Qt.exit(2);return};root.errors=0;root.failureExpected=Quickshell.env("EXPECT_FAILURE")==="1";root.opener.open("https://example.invalid/test")}}
}
