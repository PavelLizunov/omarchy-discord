import QtQuick
import "../Api.js" as Api
import "../Presence.js" as Presence

Item {
  id: root
  objectName: "dm-presence-store"
  visible: false
  property var service: null
  property bool active: false
  property var observations: ({})
  property double clock: Date.now()
  readonly property var dms: service && service.dms ? service.dms : []
  function record(user, now) {
    if (!user || !user.id) return
    var id=String(user.id), previous=observations[id], status=String(user.status || "unknown"), activity=String(user.activity || "")
    if (previous && previous.status===status && previous.activity===activity) return
    var next=Api.shallowCopy(observations)
    if (!previous && Object.keys(next).length>=4096) return
    next[id]={status:status,activity:activity,since:previous && previous.status===status ? previous.since : now,
      activitySince:previous && previous.activity===activity ? previous.activitySince : now}
    observations=next
    clock=now
  }
  function seed() {
    if (service && service.lifecycle !== undefined && service.lifecycle !== "ready") return
    var now=Date.now()
    for (var i=0;i<dms.length;i++) {
      var users=dms[i].recipients || []
      for (var j=0;j<users.length;j++) if (!observations[String(users[j].id)]) record(users[j],now)
    }
  }
  function update(message) {
    if (service && service.lifecycle !== undefined && service.lifecycle !== "ready") return
    var id=String(message.user_id || "")
    if (!observations[id]) return
    record({id:id,status:message.status,activity:message.activity},Date.now())
  }
  function summary(channel, detail) {
    var users=channel && channel.recipients ? channel.recipients : []
    if (users.length!==1) return users.length ? "Group conversation" : "Status unknown"
    if (service && service.connected === false) return "Status unknown · disconnected"
    var user=users[0], row=observations[String(user.id)] || {status:"unknown"}
    return Presence.summary(row,clock,detail)
  }
  onDmsChanged: seed()
  Component.onCompleted: seed()
  Connections {
    target: root.service && root.service.backend ? root.service.backend : null
    ignoreUnknownSignals: true
    function onEventReceived(name,message){if(name==="presence_update")root.update(message)}
    function onConnectedChanged(){root.observations=({})}
  }
  Connections {
    target:root.service
    ignoreUnknownSignals:true
    function onLifecycleChanged(){if(root.service.lifecycle!=="ready")root.observations=({})}
  }
  onActiveChanged: if(active)clock=Date.now()
  Timer {interval:60000;repeat:true;running:root.active;onTriggered:root.clock=Date.now()}
}
