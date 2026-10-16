import QtQuick
import QtQuick.Controls as Controls
import QtMultimedia
import "../ui" as PluginTheme
import "../ui"

Column {
  id: root
  objectName: "attachment-inline-video"
  property url sourceUrl: ""
  readonly property alias player: player
  readonly property alias output: output
  signal closeRequested()
  width: parent ? parent.width : 480
  spacing: Style.spacing.xxs
  function clock(ms) {
    var seconds = Math.floor(Math.max(0, ms) / 1000)
    return Math.floor(seconds / 60) + ":" + (seconds % 60 < 10 ? "0" : "") + seconds % 60
  }
  MediaPlayer {
    id: player
    objectName: "attachment-video-player"
    source: root.sourceUrl
    autoPlay: true
    videoOutput: output
    audioOutput: AudioOutput { id: audio; volume: 0.7; muted: true }
  }
  Rectangle {
    width: root.width
    height: Math.min(Style.space(270), width * 9 / 16)
    color: PluginTheme.Color.background
    VideoOutput {
      id: output
      objectName: "attachment-video-output"
      anchors.fill: parent
      fillMode: VideoOutput.PreserveAspectFit
    }
  }
  Text {
    objectName: "attachment-video-status"
    width: parent.width
    visible: player.error !== MediaPlayer.NoError || player.mediaStatus === MediaPlayer.LoadingMedia || player.mediaStatus === MediaPlayer.StalledMedia
    text: player.error !== MediaPlayer.NoError ? "Video could not play. Open it in your browser instead."
      : player.mediaStatus === MediaPlayer.StalledMedia ? "Buffering video" : "Loading video"
    color: player.error !== MediaPlayer.NoError ? PluginTheme.Color.urgent : PluginTheme.Color.foreground
    wrapMode: Text.Wrap
    font.family: Style.font.family
    font.pixelSize: Style.font.bodySmall
  }
  Controls.Slider {
    objectName: "attachment-video-seek"
    width: parent.width
    from: 0
    to: Math.max(1, player.duration)
    value: player.position
    enabled: player.seekable && player.duration > 0
    Accessible.name: "Video position"
    onMoved: player.setPosition(value)
  }
  Flow {
    width: parent.width
    spacing: Style.spacing.xxs
    Button {
      objectName: "attachment-video-toggle"
      text: player.playbackState === MediaPlayer.PlayingState ? "Pause" : "Play"
      iconName: player.playbackState === MediaPlayer.PlayingState ? "pause" : "play"
      focusable: true
      enabled: player.error === MediaPlayer.NoError
      onClicked: player.playbackState === MediaPlayer.PlayingState ? player.pause() : player.play()
    }
    Button {
      objectName: "attachment-video-stop"
      text: "Stop"
      iconName: "close"
      focusable: true
      onClicked: { player.stop(); root.closeRequested() }
    }
    Button {
      objectName: "attachment-video-mute"
      text: audio.muted ? "Unmute" : "Mute"
      iconName: audio.muted ? "speakerOff" : "speaker"
      focusable: true
      onClicked: audio.muted = !audio.muted
    }
    Text {
      text: root.clock(player.position) + " / " + root.clock(player.duration)
      color: PluginTheme.Color.foreground
      font.family: Style.font.family
      font.pixelSize: Style.font.bodySmall
      height: Style.space(32)
      verticalAlignment: Text.AlignVCenter
    }
  }
  Component.onDestruction: player.stop()
}
