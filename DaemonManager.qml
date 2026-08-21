import QtQuick
import Quickshell.Io

import "Api.js" as Api

// Owns the short-lived runtime commands around the backend. The backend itself
// is a static systemd user unit started through scripts/backend-runtime.sh
// (the only place QML touches systemctl); it is never a child of the shell.
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

  function safeError(value) {
    return Api.redact(String(value || ""))
  }

  function runtimeScript(action) {
    return ["/usr/bin/bash", pluginDir + "/scripts/backend-runtime.sh", action]
  }

  function checkRequirements() {
    // pluginDir arrives after the child Processes construct, so commands are
    // assigned here rather than bound declaratively.
    if (!pluginDir || runtimeCheck.running) return
    runtimeCheck.command = runtimeScript("check")
    runtimeCheck.running = true
  }

  // Omarchy runs no install hooks when it clones or updates a plugin, so the
  // enabled service installs its own runtime. `sync` covers both the first load
  // and every later plugin version: it compares the shipped backend against the
  // stamp scripts/setup.sh wrote into the runtime directory, runs setup.sh only
  // when they differ, and restarts a running backend when the binary changed.
  // Its fast path is a handful of stats, so running it once per Service load —
  // and the shell recreates the Service on any write inside the plugin dir —
  // costs nothing.
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
    // 0: already current, 10: installed or updated. See backend-runtime.sh.
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
          : "Discord backend setup could not be completed")
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
      root.serviceActive = false
      if (exitCode === 0) root.stopped()
      else root.lastError = "Could not stop the Discord backend"
    }
  }
}
