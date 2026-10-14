import QtQuick
import QtTest
import "../../components" as Components
import "../../components/harness" as Harness

TestCase {
 id:test;name:"ParticipantAudio";width:180;height:420;visible:true;when:windowShown
 property var callbacks:[]
 property var levels:({})
 Harness.MockService {
  id:model
  readonly property var backend:model
  function sendCommand(command,fields,callback){note(command,JSON.stringify(fields));test.callbacks.push(callback);return calls.length}
 }
 Components.MemberList {id:people;anchors.fill:parent;service:model;voiceMode:true;compactHeader:true;voiceUsers:[{id:"200",display_name:"Ada"},{id:"300",display_name:"Long participant name"}]}
 function init(){test.height=420;people.closeAudio();model.voice={status:"connected",guildId:"1",channelId:"9"};model.reset();test.callbacks=[];test.levels=({});wait(10)}
 function test_right_click_mute_and_volume_only_local(){
  wait(20);var row=findChild(people,"voice-row-200");verify(row);mouseClick(row,5,5,Qt.RightButton)
  compare(people.audioUserId,"200");compare(model.callCount("voice_user_audio"),1)
  callbacks.shift()(true,{"200":{volume:100,muted:false}},"")
  wait(30);var mute=findChild(people,"participant-mute");verify(mute.visible);verify(mute.enabled);mouseClick(mute,mute.width/2,mute.height/2);compare(model.callCount("voice_user_set"),1)
  compare(JSON.parse(model.lastCall("voice_user_set")).muted,true)
  callbacks.shift()(true,{"200":{volume:100,muted:true}},"")
  var slider=findChild(people,"participant-volume");slider.forceActiveFocus();keyClick(Qt.Key_Left)
  compare(model.callCount("voice_user_set"),2);compare(JSON.parse(model.lastCall("voice_user_set")).volume,95)
  callbacks.shift()(true,{"200":{volume:95,muted:true}},"")
  verify(slider.height>=24);keyClick(Qt.Key_Escape);compare(people.audioUserId,"")
  compare(model.callCount("voiceJoin"),0);compare(model.callCount("voiceLeave"),0);compare(model.callCount("voiceSet"),0)
 }
 function test_small_viewport_focus_reveals_slider_and_back(){
  test.height=90;people.openAudio(people.rows[0]);callbacks.shift()(true,{},"");wait(20)
  var slider=findChild(people,"participant-volume"),back=findChild(people,"participant-audio-close"),viewport=findChild(people,"participant-viewport")
  slider.forceActiveFocus();wait(10);verify(viewport.contentY>0)
  back.forceActiveFocus();wait(10);verify(back.mapToItem(viewport,0,0).y+back.height<=viewport.height+1)
  keyClick(Qt.Key_Return);compare(people.audioUserId,"")
 }
 function test_keyboard_busy_and_stale_reply(){
  people.forceActiveFocus();people.setCursor(0);keyClick(Qt.Key_Return);compare(people.audioUserId,"200")
  var audio=people.audioControls;verify(audio.busy);audio.setAudio({volume:50});compare(model.callCount("voice_user_set"),0)
  var old=callbacks.shift();people.closeAudio();people.openAudio(people.rows[1]);old(true,{"200":{volume:0,muted:true}},"")
  compare(audio.volume,100);verify(!audio.muted)
  callbacks.shift()(false,null,"Fixture failure");compare(audio.errorText,"Fixture failure")
 }
}
