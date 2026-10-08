import QtQuick
import QtTest
import "../../components" as Components

TestCase {
 id: tests
 name: "ArchivePersistence"
 width:520; height:560; visible:true; when:windowShown
 Preview {id:p; anchors.fill:parent; serverListFixture:true;useRealServerActions:true}
 Component {id:controller;Components.ServerActions{}}
 function initTestCase(){tryCompare(p,"ready",true)}
 function init(){p.model.entry={unrelated:"keep",archivedGuilds:"[]"};p.client.serverActions.loadArchived();p.client.serverQuery="";p.client.serverFilter="all";p.client.serverSort="position";p.model.reset()}
 function test_archive_survives_controller_recreation_and_reconnect(){
  var id="2",a=p.client.serverActions,channel=p.model.currentChannelId
  p.model.voice={status:"connected",guildId:"1",channelId:"9002"}
  a.toggleArchive(id,true);compare(p.model.callCount("persistOpaque"),1)
  compare(JSON.stringify(JSON.parse(p.model.entry.archivedGuilds)),JSON.stringify([id]));compare(p.model.entry.unrelated,"keep")
  verify(p.client.filteredGuilds.every(function(g){return g.id!==id}))
  var recreated=controller.createObject(tests,{service:p.model});verify(recreated!==null)
  compare(recreated.archivedCount,1);verify(recreated.isArchived(id))
  p.model.connected=false;wait(20);p.model.connected=true;wait(20);verify(recreated.isArchived(id))
  p.client.serverFilter="archive";wait(20);compare(p.client.filteredGuilds.length,1);compare(p.client.filteredGuilds[0].id,id)
  recreated.toggleArchive(id,false);a.loadArchived();compare(a.archivedCount,0);compare(p.model.entry.archivedGuilds,"[]")
  compare(p.model.entry.unrelated,"keep");compare(p.model.currentChannelId,channel);compare(p.model.voice.channelId,"9002")
  compare(p.model.callCount("voiceJoin"),0);compare(p.model.callCount("voiceLeave"),0);compare(p.model.callCount("sendMessage"),0)
  recreated.destroy();p.model.voice={status:"idle"}
 }
 function test_archive_parsing_and_filter_counts_without_channel_cache(){
  var channels=p.model.channelsByGuild;p.model.channelsByGuild={}
  p.model.entry={archivedGuilds:["2","2",""]};p.client.serverActions.loadArchived();compare(p.client.serverActions.archivedCount,1)
  compare(p.client.unreadServersCount,1);compare(p.client.mentionedServersCount,1)
  p.client.serverFilter="unread";compare(p.client.filteredGuilds.length,1);compare(p.client.filteredGuilds[0].id,"1")
  p.client.serverFilter="mentions";compare(p.client.filteredGuilds.length,1)
  p.client.serverQuery="Music";compare(p.client.filteredGuilds.length,0)
  p.client.serverFilter="archive";compare(p.client.filteredGuilds.length,1);compare(p.client.filteredGuilds[0].id,"2")
  p.model.entry={archivedGuilds:"broken json"};p.client.serverActions.loadArchived();compare(p.client.serverActions.archivedCount,0)
  p.client.serverQuery="";p.client.serverFilter="all";p.model.channelsByGuild=channels
 }
}
