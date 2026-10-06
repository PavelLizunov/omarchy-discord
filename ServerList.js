.pragma library

function knownCount(stats, id) {
  var row = stats && stats[String(id)]
  return row && typeof row.online_count === "number" && isFinite(row.online_count)
    && row.online_count >= 0 ? row.online_count : null
}

function rows(guilds, query, filter, sort, stats) {
  var needle = String(query || "").trim().toLocaleLowerCase()
  return (guilds || []).filter(function(row) {
    if (needle && String(row.name || "").toLocaleLowerCase().indexOf(needle) < 0) return false
    var mentions = Number(row.mention_count) || 0
    if (filter === "mentions") return mentions > 0 || row.unread === "mentioned"
    if (filter === "unread") return mentions > 0 || row.unread === "mentioned" || row.unread === "unread"
    return true
  }).slice().sort(function(a, b) {
    var diff = 0
    if (sort === "name") diff = String(a.name || "").localeCompare(String(b.name || ""))
    else if (sort === "mentions") diff = (Number(b.mention_count) || 0) - (Number(a.mention_count) || 0)
    else if (sort === "online") {
      var ac = knownCount(stats, a.id), bc = knownCount(stats, b.id)
      if (ac === null && bc !== null) return 1
      if (ac !== null && bc === null) return -1
      if (ac !== null && bc !== null) diff = bc - ac
    }
    if (diff) return diff
    diff = (Number(a.position) || 0) - (Number(b.position) || 0)
    return diff || String(a.id).localeCompare(String(b.id))
  })
}
