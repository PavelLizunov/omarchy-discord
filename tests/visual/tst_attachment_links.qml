import QtQuick
import QtTest
import "../../components" as Components
TestCase {
 id:test
 name:"AttachmentLinks"
 visible:true;width:640;height:400;when:windowShown
 property string opened:""
 Components.MessageRow {
  id:row;width:600
  message:({id:"100",author:{id:"10",username:"Fixture"},attachments:[{filename:"fixture.mp4",url:"https://cdn.discordapp.com/attachments/fixture.mp4",size:1234}]})
  onLinkActivated:function(url){test.opened=url}
 }
 function test_attachment_pointer_opens_url() {
  wait(30)
  var action=findChild(row,"attachment-open-fixture.mp4");verify(action!==null)
  mouseClick(action,action.width/2,action.height/2)
  tryCompare(test,"opened","https://cdn.discordapp.com/attachments/fixture.mp4",500)
  test.opened="";action.forceActiveFocus();keyClick(Qt.Key_Return)
  tryCompare(test,"opened","https://cdn.discordapp.com/attachments/fixture.mp4",500)
 }
}
