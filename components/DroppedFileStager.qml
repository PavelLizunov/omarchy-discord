import QtQuick
import Quickshell
import Quickshell.Io

Item {
  id: root
  visible: false
  property var service: null
  readonly property bool busy: copyProcess.running
  property string targetChannel: ""
  property double generation: -1
  signal staged(string channelId)
  Component.onDestruction: if(copyProcess.running)copyProcess.signal(15)

  function stage(channelId, urls) {
    if (busy || !service || !service.ready || !service.isOpen(channelId) || !service.stagedDir) return false
    targetChannel=channelId
    generation=service.lastGeneration
    copyProcess.command=["/usr/bin/python3",service.daemon.pluginDir + "/scripts/stage-dropped-files.py",service.stagedDir].concat(urls)
    copyProcess.running=true
    return true
  }
  Process {
    id: copyProcess
    stdout: StdioCollector { id: output; waitForEnd: true }
    onStarted: copyTimeout.start()
    onExited: function(code) {
      copyTimeout.stop()
      var result
      try { result=JSON.parse(output.text) } catch(e) { result={error:"Could not copy dropped files."} }
      if (code !== 0 || !Array.isArray(result.files)) { if(root.service)root.service.fail(result.error || "Could not copy dropped files."); return }
      var files=result.files
      if (!root.service || !root.service.ready || root.service.lastGeneration !== root.generation || !root.service.isOpen(root.targetChannel)
          || root.service.stagedFor(root.targetChannel).length + files.length > 10) {
        Quickshell.execDetached(["/usr/bin/rm","-f","--"].concat(files.map(function(file){return file.path})))
        return
      }
      for(var i=0;i<files.length;i++)root.service.stageFile(root.targetChannel,files[i].path,files[i].size)
      root.staged(root.targetChannel)
    }
  }
  Timer {
    id: copyTimeout
    interval: 22000
    onTriggered: if(copyProcess.running)copyProcess.signal(9)
  }
}
