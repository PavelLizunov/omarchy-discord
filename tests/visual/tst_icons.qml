import QtQuick
import QtTest
import "../../ui"
import "../../ui/Icons.js" as Icons
import "../../" as Plugin

TestCase {
  name: "ActionIcons"
  when: windowShown
  width: 400
  height: 100
  Button { id: button; text: "Search"; iconName: "search"; focusable: true }
  function test_svg_loading_theme_and_activation() {
    var names = Object.keys(Icons.paths)
    for (var i = 0; i < names.length; i++) {
      button.iconName = names[i]
      tryCompare(button, "iconStatus", Image.Ready)
    }
    button.foreground = "#f5f4f0"
    tryCompare(button, "iconStatus", Image.Ready)
    verify(Icons.svg("search", button.foreground).indexOf('stroke="rgb(245,244,240)"') >= 0)
    button.foreground = "#242424"
    tryCompare(button, "iconStatus", Image.Ready)
    verify(Icons.svg("search", button.foreground).indexOf('stroke="rgb(36,36,36)"') >= 0)
    button.forceActiveFocus()
    spy.clear(); keyClick(Qt.Key_Return); compare(spy.count, 1)
    compare(button.text, "Search")
    verify(button.implicitWidth > button.iconSize)
    compare(Icons.source("invalid", button.foreground), "")
  }
  function test_disabled_button_rejects_activation() {
    button.enabled=false;button.forceActiveFocus();spy.clear()
    keyClick(Qt.Key_Return);keyClick(Qt.Key_Space);mouseClick(button,button.width/2,button.height/2)
    compare(spy.count,0);button.enabled=true
  }
  Plugin.DiscordIcon {id:mark;iconSize:16}
  function test_solid_stepped_mark_and_decode_size(){
    verify(Icons.svg("omacord","#d8dee9",32).indexOf('viewBox="0 0 16 16"')>=0)
    verify(Icons.svg("omacord","#d8dee9",32).indexOf('fill-rule="evenodd"')>=0)
    verify(Icons.svg("barMicrophone","#d8dee9",32)!==Icons.svg("barMicrophoneOff","#d8dee9",32))
    tryCompare(mark,"iconStatus",Image.Ready)
    compare(mark.children[0].implicitWidth,Math.ceil(16*Screen.devicePixelRatio))
    mark.iconSize=64;tryCompare(mark,"iconStatus",Image.Ready)
    compare(mark.children[0].implicitWidth,Math.ceil(64*Screen.devicePixelRatio))
  }
  SignalSpy { id: spy; target: button; signalName: "clicked" }
}
