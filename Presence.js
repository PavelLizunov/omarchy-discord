.pragma library

function label(status) {
  switch(String(status || "")) {
    case "online":return "Online"
    case "idle":return "Away"
    case "dnd":return "Do not disturb"
    case "offline":return "Offline · may be invisible"
    default:return "Status unknown"
  }
}
function observed(since,now) {
  if (!since || !isFinite(since) || now<since) return ""
  var minutes=Math.floor((now-since)/60000)
  return minutes<1 ? "observed <1 min" : minutes<60 ? "observed "+minutes+" min" : "observed "+Math.floor(minutes/60)+" h "+(minutes%60)+" min"
}
function summary(row,now,detail) {
  var text=label(row.status)
  if(detail && row.status && row.status!=="unknown") {var age=observed(row.since,now);if(age)text+=" · "+age}
  if(row.activity) {text+=" · "+row.activity;if(detail){var age=observed(row.activitySince,now);if(age)text+=" ("+age+")"}}
  return text
}
