import QtQuick
import QtTest
import "../../components" as Components

TestCase {
 id:test;name:"ServerStatsReconnect";when:windowShown
 property var callbacks:[]
 QtObject {
  id:service
  property bool ready:true
  property var guilds:[{id:"old"}]
  property var backend:service
  function configuredEntry(){return {archivedGuilds:"[]"}}
  function sendCommand(name,fields,callback){test.callbacks.push(callback)}
 }
 Components.ServerActions {id:actions;service:service}
 function test_late_old_reply_cannot_replace_new_snapshot(){
  verify(actions.fetchGuildStats());verify(actions.guildStatsBusy)
  service.ready=false;compare(actions.guildStatsBusy,false)
  service.guilds=[{id:"new"}];service.ready=true
  verify(actions.fetchGuildStats());compare(callbacks.length,2)
  callbacks[1](true,{online_count:2});tryCompare(actions,"guildStatsBusy",false)
  compare(actions.guildStats.new.online_count,2)
  callbacks[0](true,{online_count:1});wait(20)
  compare(actions.guildStats.new.online_count,2);verify(actions.guildStats.old===undefined)
 }
}
