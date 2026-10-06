import QtQuick
import QtTest
import "../../ui"

TestCase {
  name: "ThemeAliases"
  function cleanup() { Color.shellValues = ({}) }
  function test_alias_cycle_falls_back() {
    Color.shellValues = {"popups.background":"menu.background", "menu.background":"popups.background"}
    compare(Color.flatColor("popups.background", "#123456"), "#123456")
  }
  function test_alias_chain_and_gradient() {
    Color.shellValues = {"popups.background":"menu.background", "menu.background":"#123456"}
    compare(Color.flatColor("popups.background", "#abcdef"), "#123456")
    compare(Color.flatColor("90deg #123456 #abcdef", "#ffffff"), "#123456")
    compare(Color.flatColor("not-a-color", "#abcdef"), "#abcdef")
  }
}
