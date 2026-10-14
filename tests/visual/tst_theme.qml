import QtQuick
import "../../ui" as PluginTheme
import QtTest
import "../../ui"

TestCase {
  name: "ThemeAliases"
  function cleanup() { PluginTheme.Color.shellValues = ({}) }
  function test_alias_cycle_falls_back() {
    PluginTheme.Color.shellValues = {"popups.background":"menu.background", "menu.background":"popups.background"}
    compare(PluginTheme.Color.flatColor("popups.background", "#123456"), "#123456")
  }
  function test_alias_chain_and_gradient() {
    PluginTheme.Color.shellValues = {"popups.background":"menu.background", "menu.background":"#123456"}
    compare(PluginTheme.Color.flatColor("popups.background", "#abcdef"), "#123456")
    compare(PluginTheme.Color.flatColor("90deg #123456 #abcdef", "#ffffff"), "#123456")
    compare(PluginTheme.Color.flatColor("not-a-color", "#abcdef"), "#abcdef")
  }
}
