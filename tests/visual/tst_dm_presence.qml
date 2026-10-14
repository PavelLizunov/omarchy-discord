import QtQuick
import QtTest
import "../../components" as Components
import "../../components/harness" as Harness
import "../../Presence.js" as Presence
TestCase {
 id:test;name:"DirectPresence";width:320;height:240;visible:true;when:windowShown
 Harness.MockService {id:model;readonly property var backend:model;property bool connectedSocket:true;signal eventReceived(string name,var message)}
 Components.DMPresence {id:p;service:model;active:true}
 function init(){p.observations=({});model.dms=[{id:"42",recipients:[{id:"200",status:"online",activity:"Working"}]}];p.seed()}
 function test_status_and_custom_text_update_while_chat_closed(){
  var channel=model.dms[0];verify(p.summary(channel,true).indexOf("Online · observed <1 min · Working")===0)
  var before=p.observations["200"].since;model.eventReceived("presence_update",{user_id:"200",status:"online",activity:"Reading"});compare(p.observations["200"].since,before)
  model.eventReceived("presence_update",{user_id:"200",status:"idle",activity:"Reading"});verify(p.summary(channel,false).indexOf("Away · Reading")===0)
  model.eventReceived("presence_update",{user_id:"200",status:"offline",activity:""});compare(p.summary(channel,false),"Offline · may be invisible")
 }
 function test_reconnect_discards_unobserved_time_and_stale_metadata(){
  var ch=model.dms[0];model.eventReceived("presence_update",{user_id:"200",status:"dnd",activity:"Working"});p.seed();verify(p.summary(ch,false).indexOf("Do not disturb")===0)
  model.backend.connectedChanged();compare(p.summary(ch,true),"Status unknown")
  model.dms=model.dms.slice();verify(p.summary(ch,true).indexOf("Online · observed <1 min")===0)
  var since=p.observations["200"].since;model.eventReceived("presence_update",{user_id:"200",status:"online",activity:"Working"});compare(p.observations["200"].since,since)
 }
 function test_duration_is_observation_and_missing_is_not_offline(){compare(Presence.observed(100000,220000),"observed 2 min");compare(Presence.observed(0,220000),"");compare(p.summary({recipients:[{id:"300"}]},true),"Status unknown");compare(p.summary({recipients:[{id:"200"},{id:"300"}]},true),"Group conversation")}
}
