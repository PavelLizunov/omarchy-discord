import QtQuick
import QtTest
import QtMultimedia
import "../../components" as Components

TestCase {
  id: test
  name: "InlineVideo"
  width: 600; height: 600
  visible: true
  when: windowShown
  Components.MessageRow {
    id: row
    width: 560
    message: ({id:"fixture",author:{id:"1",username:"Fixture"},attachments:[{id:"clip",filename:"clip.mp4",content_type:"video/mp4",url:"http://127.0.0.1:9182/clip.mp4",size:12345}]})
  }
  function init() {
    row.mediaActive=true
    row.spoilersRevealed=false
    row.message={id:"fixture",author:{id:"1",username:"Fixture"},attachments:[{id:"clip",filename:"clip.mp4",content_type:"video/mp4",url:"http://127.0.0.1:9182/clip.mp4",size:12345}]}
    wait(20)
  }
  function cleanup() { row.stopMedia();wait(30) }
  function test_click_decodes_pauses_and_stops() {
    compare(findChild(row,"attachment-inline-video"),null)
    var collapsedHeight=row.implicitHeight
    var play=findChild(row,"attachment-play-clip")
    verify(play!==null,"Video play action missing")
    mouseClick(play)
    tryVerify(function(){return findChild(row,"attachment-inline-video")!==null},1000)
    var player=findChild(row,"attachment-inline-video").player
    tryVerify(function(){return player.position>150 && player.hasVideo},5000)
    var output=findChild(row,"attachment-video-output")
    verify(output.videoSink.videoSize.width>0,"Decoded video size missing")
    var picture=grabImage(output)
    verify(picture.width>0)
    verify(picture.red(Math.floor(picture.width/4),Math.floor(picture.height/2))
      !==picture.red(Math.floor(picture.width*3/4),Math.floor(picture.height/2)),"Video output has no painted frame")
    picture.save("/tmp/inline-video-painted.png")
    mouseClick(findChild(row,"attachment-video-toggle"))
    tryCompare(player,"playbackState",MediaPlayer.PausedState,1000)
    var pos=player.position;wait(100);compare(player.position,pos)
    mouseClick(findChild(row,"attachment-video-toggle"))
    tryCompare(player,"playbackState",MediaPlayer.PlayingState,1000)
    mouseClick(findChild(row,"attachment-video-stop"))
    wait(30)
    compare(findChild(row,"attachment-inline-video"),null,"Stop must unload the player")
    tryCompare(row,"implicitHeight",collapsedHeight,1000,"Stop must restore collapsed height")
    mouseClick(findChild(row,"attachment-play-clip"))
    tryVerify(function(){return findChild(row,"attachment-inline-video")!==null},1000)
    row.mediaActive=false
    tryCompare(row,"mediaActive",false)
    wait(50)
    compare(findChild(row,"attachment-inline-video"),null)
    row.mediaActive=true;wait(20)
    compare(findChild(row,"attachment-inline-video"),null,"Reopening must not restart playback")
  }
  function test_spoiler_and_unsafe_url_do_not_start() {
    row.message={id:"spoiler",attachments:[{id:"clip",filename:"clip.mp4",spoiler:true,url:"http://127.0.0.1:9182/clip.mp4"}]}
    wait(20)
    mouseClick(findChild(row,"attachment-play-clip"))
    wait(20)
    compare(findChild(row,"attachment-inline-video"),null)
    row.message={id:"unsafe",attachments:[{id:"clip",filename:"clip.mp4",url:"file:///tmp/clip.mp4"}]}
    wait(20)
    verify(!findChild(row,"attachment-play-clip").enabled)
    compare(findChild(row,"attachment-inline-video"),null)
  }
  function test_error_and_message_reuse() {
    row.message={id:"bad",attachments:[{id:"clip",filename:"clip.mp4",url:"http://127.0.0.1:9182/bad.mp4"}]}
    wait(20);mouseClick(findChild(row,"attachment-play-clip"))
    tryVerify(function(){var p=findChild(row,"attachment-inline-video");return p && p.player.error!==MediaPlayer.NoError},5000)
    verify(findChild(row,"attachment-video-status").visible)
    verify(findChild(row,"attachment-open-clip").visible,"Browser fallback missing")
    row.message={id:"next",attachments:[]}
    wait(50)
    compare(findChild(row,"attachment-inline-video"),null)
  }
}
