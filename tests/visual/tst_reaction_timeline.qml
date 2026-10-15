import QtQuick
import QtTest
import "../../components" as Components

TestCase {
  id: tests
  name: "ReactionTimeline"
  visible: true
  width: 1280
  height: 960
  when: windowShown
  Components.Timeline {
    id: timeline
    width: 1070
    height: 828
    viewing: false
  }
  Components.MessageRow { id: messageRow; x:1080; width:180 }
  function findEditor(node) {
    if (typeof node.getText === "function" && typeof node.deselect === "function") return node
    var children=node.children || []
    for(var i=0;i<children.length;i++) {var found=findEditor(children[i]);if(found)return found}
    return null
  }
  function test_reused_message_format_transition() {
    var editor=findEditor(messageRow)
    verify(editor!==null)
    for(var i=0;i<6;i++) {
      messageRow.message={content:"**Formatted** message",system:false,author:{id:"10",username:"fixture"}}
      wait(10)
      compare(editor.getText(0,editor.length),"Formatted message")
      messageRow.message={content:"Fixture pinned a message. <b>literal</b>",system:true}
      wait(10)
      compare(editor.getText(0,editor.length),"Fixture pinned a message. <b>literal</b>")
      verify(editor.getText(0,editor.length).indexOf("<!DOCTYPE")<0)
    }
  }
  function messages() {
    var rows = []
    for (var i = 0; i < 14; i++) {
      var reactions = []
      for (var j = 0; j < 20; j++)
        reactions.push({emoji:["👍", "❤️", "🔥", "🇧🇷", "custom_fixture:123"][j % 5],count:j + 1,me:false})
      rows.push({id:String(1000 + i),content:"Synthetic announcement " + i + "\n"
          + Array(i * 3 + 2).join("Fixture text. "),
        timestamp:"2026-10-" + String(1 + i).padStart(2,"0") + "T00:00:00Z",
        author:{id:"10",username:"fixture"},reactions:reactions})
    }
    return rows
  }
  function test_reactions_scroll_and_resize_stay_responsive() {
    timeline.channelId = "fixture"
    timeline.messages = messages()
    wait(100)
    compare(timeline.rows.length,14)
    timeline.focusNewest()
    wait(100)
    compare(timeline.cursorMessageId,"1013")
    for (var i = 0; i < 4; i++) {
      timeline.goTop()
      wait(30)
      timeline.focusNewest()
      wait(30)
    }
    timeline.width = 640
    wait(50)
    timeline.focusNewest()
    wait(50)
    timeline.width = 1070
    wait(50)
    compare(timeline.cursorMessageId,"1013")
  }
  function test_pooled_row_releases_selection() {
    var rows = messages()
    for (var i = 14; i < 50; i++) rows.push(Object.assign({},rows[i % 14],{id:String(1000+i)}))
    timeline.messages = rows
    timeline.focusNewest()
    wait(50)
    var list = findChild(timeline,"timeline-list")
    verify(list !== null)
    var row = list.itemAtIndex(49)
    verify(row !== null)
    var body = row.children.filter(function(item) { return typeof item.clearSelection === "function" })[0]
    verify(body !== undefined)
    timeline.noteSelection(body)
    compare(timeline.selectionOwner,body)
    timeline.goTop()
    wait(100)
    compare(timeline.selectionOwner,null)
  }
}
