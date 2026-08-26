import QtQuick
import QtQuick.Effects
import qs.Commons

import "../Api.js" as Api

// Circular user avatar through the media cache, with the display name's
// initial as the fallback while (or instead of) the image resolves. Shared by
// the member pane and the voice occupant rows; the caller sizes it and may
// add children of its own (the member pane's status dot).
Item {
  id: root

  property var service: null
  property string url: ""
  property string name: ""
  property color foreground: Color.foreground
  property string fontFamily: Style.font.family

  // A miss here does no I/O — mediaPath records the want and the service
  // issues the fetch on the next event-loop turn (CONVENTIONS §2, Media).
  readonly property string path: service && url
    ? String(service.mediaPath(String(url), 64) || "") : ""

  Rectangle {
    anchors.fill: parent
    radius: width / 2
    color: Util.alpha(root.foreground, 0.12)
    visible: !effect.visible

    Text {
      anchors.centerIn: parent
      text: Api.initials(root.name).charAt(0)
      color: root.foreground
      font.family: root.fontFamily
      font.pixelSize: Style.font.caption
      font.bold: true
    }
  }
  Rectangle {
    id: mask
    anchors.fill: parent
    radius: width / 2
    visible: false
    layer.enabled: true
  }
  Image {
    id: image
    anchors.fill: parent
    visible: false
    asynchronous: true
    cache: true
    fillMode: Image.PreserveAspectCrop
    sourceSize.width: root.width * 2
    sourceSize.height: root.height * 2
    source: root.path ? "file://" + root.path : ""
    // A path the backend's LRU has evicted fails here: report it so the
    // service drops the key and re-requests it once.
    onStatusChanged: if (status === Image.Error && root.service) root.service.mediaError(root.path)
  }
  MultiEffect {
    id: effect
    anchors.fill: image
    source: image
    maskEnabled: true
    maskSource: mask
    visible: root.path !== "" && image.status === Image.Ready
  }
}
