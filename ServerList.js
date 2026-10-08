.pragma library

function knownCount(stats, id) {
  var row = stats && stats[String(id)]
  return row && typeof row.online_count === "number" && isFinite(row.online_count)
    && row.online_count >= 0 ? row.online_count : null
}

function hasVoiceUsers(voiceMembersMap, guildId) {
  if (!voiceMembersMap) return false
  var list = voiceMembersMap[String(guildId || "")]
  if (!Array.isArray(list)) return false
  for (var i = 0; i < list.length; i++) {
    if (list[i] && Array.isArray(list[i].users) && list[i].users.length > 0) return true
  }
  return false
}

function rows(guilds, query, filter, sort, stats, archivedMap, voiceMembersMap) {
  var needle = String(query || "").trim().toLocaleLowerCase()
  var isArchiveFilter = filter === "archive"
  return (guilds || []).filter(function(row) {
    if (!row) return false
    var isArchived = !!(archivedMap && archivedMap[String(row.id)])
    if (isArchiveFilter ? !isArchived : isArchived) return false
    if (needle && String(row.name || "").toLocaleLowerCase().indexOf(needle) < 0) return false
    var mentions = Number(row.mention_count) || 0
    if (filter === "mentions") return mentions > 0 || row.unread === "mentioned"
    if (filter === "unread") return mentions > 0 || row.unread === "mentioned" || row.unread === "unread"
    if (filter === "voice") return hasVoiceUsers(voiceMembersMap, row.id)
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

function archivedCount(guilds, archivedMap) {
  if (!archivedMap) return 0
  var count = 0
  var list = guilds || []
  for (var i = 0; i < list.length; i++) {
    if (list[i] && archivedMap[String(list[i].id)]) count++
  }
  return count
}

function unreadCount(guilds, archivedMap) {
  var count = 0
  var list = guilds || []
  for (var i = 0; i < list.length; i++) {
    if (!list[i] || (archivedMap && archivedMap[String(list[i].id)])) continue
    if (list[i].unread === "unread" || list[i].unread === "mentioned" || Number(list[i].mention_count) > 0) count++
  }
  return count
}

function mentionsCount(guilds, archivedMap) {
  var count = 0
  var list = guilds || []
  for (var i = 0; i < list.length; i++) {
    if (!list[i] || (archivedMap && archivedMap[String(list[i].id)])) continue
    if (Number(list[i].mention_count) > 0 || list[i].unread === "mentioned") count++
  }
  return count
}

function voiceGuildsCount(guilds, archivedMap, voiceMembersMap) {
  var count = 0
  var list = guilds || []
  for (var i = 0; i < list.length; i++) {
    if (!list[i] || (archivedMap && archivedMap[String(list[i].id)])) continue
    if (hasVoiceUsers(voiceMembersMap, list[i].id)) count++
  }
  return count
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { knownCount: knownCount, rows: rows, archivedCount: archivedCount,
    unreadCount: unreadCount, mentionsCount: mentionsCount, voiceGuildsCount: voiceGuildsCount,
    hasVoiceUsers: hasVoiceUsers }
}
