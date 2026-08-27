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
    antialiasing: true
    layer.enabled: true
    // Render the mask above avatar resolution and smooth it so the circle
    // edge is crisp, not feathered, once the effect thresholds it.
    layer.smooth: true
    layer.textureSize: Qt.size(width * 2, height * 2)
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
    antialiasing: true
    maskEnabled: true
    maskSource: mask
    // Threshold the mask alpha to a sharp, 1px-antialiased edge instead of
    // the default soft ramp that feathers the circle.
    maskThresholdMin: 0.5
    maskSpreadAtMin: 1.0
    visible: root.path !== "" && image.status === Image.Ready
  }
}
