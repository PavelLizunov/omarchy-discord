import QtQuick
import QtTest
import "../../" as Plugin
import "../../components" as Components
import "../../components/harness" as Harness

TestCase {
 id:test;name:"ExplicitPresence";width:200;height:420;visible:true;when:windowShown
 Harness.MockService {id:model;readonly property var backend:model;signal eventReceived(string name,var message);function sendCommand(name,fields,callback){if(callback)callback(true,{},"")}}
 Components.CallBar {id:call;width:200;height:420;expanded:true;service:model}
 Plugin.DiscordIcon {id:mark;iconSize:18}
 function init(){model.voice={status:"connected",guildId:"1",channelId:"9"};model.voiceMembers={};wait(20)}
 function test_states_are_text_not_only_color(){
  for(var status of ["online","idle","dnd","offline","unknown"]){
   model.voiceMembers={"1":[{channel_id:"9",users:[{id:"200",display_name:"Ada",status:status}]}]};wait(20)
   var label=findChild(call,"presence-200");verify(label);compare(label.text,call.roster.statusLabel(status));verify(label.visible)
  }
 }
 function test_departed_users_are_removed_without_inventing_presence(){
  model.voiceMembers={"1":[{channel_id:"9",users:[{id:"200",display_name:"Ada",status:"online"}]}]};wait(20)
  compare(call.roster.rows.length,1)
  model.voiceMembers={"1":[]};wait(20);compare(call.roster.rows.length,0)
  verify(findChild(call,"voice-departure")===null)
 }
 function test_primary_icon_theme_and_small_size(){tryCompare(mark,"iconStatus",Image.Ready);compare(mark.width,18);mark.color="#f5f4f0";tryCompare(mark,"iconStatus",Image.Ready);mark.iconSize=14;compare(mark.width,14)}
}
