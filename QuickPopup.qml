import QtQuick
import qs.Commons
import qs.Ui
import "components" as Components

PopupCard {
  id: root
  property var service: null
  readonly property alias content: view
  signal fullRequested(var payload)
  grabFocus: true
  contentWidth: fittedContentWidth(Style.space(460))
  contentHeight: fittedContentHeight(Style.space(540))
  Components.QuickView {
    id: view
    anchors.fill: parent
    service: root.service
    opened: root.open
    onCloseRequested: root.close()
    onFullRequested: function(payload) { root.fullRequested(payload) }
  }
}
