import QtQuick
import QtTest
import "../../" as Discord
import "../../ui"

TestCase {
  name: "HostTheme"
  when: windowShown
  QtObject {
    id: hostColor
    property color foreground: "#242424"
    property color background: "#f5f4f0"
    property color accent: "#235a81"
    property color urgent: "#a02030"
    property color muted: "#565656"
    property var shellValues: ({})
  }
  QtObject {
    id: hostStyle
    property int cornerRadius: 0
    property int gapsOut: 5
    property string resolvedFontFamily: "Adwaita Sans"
    property var font: ({family: resolvedFontFamily})
  }
  Discord.ThemeSync { hostColor: hostColor; hostStyle: hostStyle }
  function test_host_palette_changes_without_panel() {
    compare(String(Color.background), "#f5f4f0")
    compare(String(Color.popups.text), "#242424")
    hostColor.background="#101315"; hostColor.foreground="#cacccc"
    compare(String(Color.background), "#101315")
    compare(String(Color.popups.text), "#cacccc")
    hostColor.shellValues={"popups.background":"#202428"}
    compare(String(Color.popups.background),"#202428")
    hostStyle.resolvedFontFamily="monospace"
    compare(Style.font.family,"monospace")
  }
}
