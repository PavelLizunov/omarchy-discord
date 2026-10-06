import QtQuick
import Quickshell.Io


Item {
  id: root

  visible: false
  width: 0
  height: 0

  property string pluginDir: ""
  property string unitName: "omarchy-discord.service"

  property bool runtimeAvailable: false
  property bool runtimeChecked: false
  property bool automaticSetupAttempted: false
  property bool serviceActive: false
  property bool setupBusy: false
  property bool busy: false
  property string lastError: ""

  readonly property bool running: serviceActive

  signal started()
  signal stopped()
  signal setupSucceeded()
  signal setupFailed(string reason)

  function runtimeScript(action) {
    return ["/usr/bin/bash", pluginDir + "/scripts/backend-runtime.sh", action]
  }

  function checkRequirements() {
    if (!pluginDir || runtimeCheck.running) return
    runtimeCheck.command = runtimeScript("check")
    runtimeCheck.running = true
  }

  function syncRuntimeIfNeeded() {
    if (automaticSetupAttempted || setupBusy || !pluginDir || !runtimeChecked) return
    automaticSetupAttempted = true
    setupBackend()
  }

  function setupBackend() {
    if (setupBusy || !pluginDir) return
    lastError = ""
    setupBusy = true
    setupCommand.command = runtimeScript("sync")
    setupCommand.running = true
  }

  function refreshStatus() {
    if (!pluginDir || statusCheck.running) return
    statusCheck.command = runtimeScript("status")
    statusCheck.running = true
  }

  function start() {
    if (busy || serviceActive || !pluginDir) return
    if (!runtimeAvailable) {
      lastError = "The Discord backend is not installed yet"
      return
    }
    lastError = ""
    busy = true
    startCommand.command = runtimeScript("start")
    startCommand.running = true
  }

  function stop() {
    if (busy || !pluginDir) return
    lastError = ""
    busy = true
    stopCommand.command = runtimeScript("stop")
    stopCommand.running = true
  }

  onPluginDirChanged: {
    if (!pluginDir) return
    checkRequirements()
    refreshStatus()
  }

  Process {
    id: runtimeCheck
    stdout: StdioCollector { waitForEnd: true }
    stderr: StdioCollector { waitForEnd: true }
    onExited: function(exitCode) {
      root.runtimeAvailable = exitCode === 0
      root.runtimeChecked = true
      root.syncRuntimeIfNeeded()
    }
  }

  Process {
    id: setupCommand
    stdout: StdioCollector { waitForEnd: true }
    stderr: StdioCollector { waitForEnd: true }
    onExited: function(exitCode) {
      root.setupBusy = false
      if (exitCode === 0 || exitCode === 10) {
        root.runtimeAvailable = true
        root.runtimeChecked = true
        root.lastError = ""
        root.refreshStatus()
        if (exitCode === 10) root.setupSucceeded()
        return
      }
      root.lastError = exitCode === 30
        ? "No Discord backend ships for this machine and Go is not installed"
        : (exitCode === 31
          ? "The Discord backend could not be built; run scripts/setup.sh in the plugin directory for the build output"
          : (exitCode === 32
            ? "libopus is missing — install the opus package"
            : "Discord backend setup could not be completed"))
      root.setupFailed(root.lastError)
    }
  }

  Process {
    id: statusCheck
    stdout: StdioCollector { waitForEnd: true }
    stderr: StdioCollector { waitForEnd: true }
    onExited: function(exitCode) { root.serviceActive = exitCode === 0 }
  }

  Process {
    id: startCommand
    stdout: StdioCollector { waitForEnd: true }
    stderr: StdioCollector { waitForEnd: true }
    onExited: function(exitCode) {
      root.busy = false
      if (exitCode === 0) {
        root.serviceActive = true
        root.started()
      } else {
        root.lastError = "Could not start the Discord backend"
      }
    }
  }

  Process {
    id: stopCommand
    stdout: StdioCollector { waitForEnd: true }
    stderr: StdioCollector { waitForEnd: true }
    onExited: function(exitCode) {
      root.busy = false
      if (exitCode === 0) {
        root.serviceActive = false
        root.stopped()
      } else {
        root.lastError = "Could not stop the Discord backend"
      }
    }
  }
}
