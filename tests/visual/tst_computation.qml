import QtQuick
import QtTest
import "../../components" as Components
import "../../Markdown.js" as Markdown
import "Counter.js" as Counter

TestCase {
  id: tests
  name: "MessageComputation"
  width: 700
  height: 600
  when: windowShown
  Components.MessageRow {
    id: row
    width: 680
    height: implicitHeight
    markdownRenderer: function(text, context) { Counter.hit(); return Markdown.render(text,context) }
  }
  function test_reaction_update_does_not_parse_text() {
    row.message={id:"1",content:"**same text**",author:{username:"fixture"},timestamp:"2026-10-01T00:00:00Z",reactions:[]}
    wait(10)
    var before=Counter.count()
    for (var i=0;i<30;i++) row.message=Object.assign({},row.message,{reactions:[{emoji:"x",count:i}]})
    wait(10)
    compare(Counter.count(),before)
    row.message=Object.assign({},row.message,{content:"**edited**"})
    wait(10)
    compare(Counter.count(),before+1)
    verify(row.html.indexOf("edited")>=0)
    var beforeContext=Counter.count()
    row.ctx={users:{"200":"Renamed fixture"},linkColor:"#cacccc"}
    wait(10)
    compare(Counter.count(),beforeContext+1)
  }
}
