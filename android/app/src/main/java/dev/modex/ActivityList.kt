package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
internal fun ActivityList(activities: List<HostActivity>, hostNames: Map<String, String>, connectedHosts: Set<String>, onOpen: (String, String) -> Unit) {
    if (activities.isEmpty()) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 40.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            ChatGlyph(ChatIcon.Activity, color = Mocha.Subtext)
            Text("Nothing needs attention", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp))
            Text("Running sessions and questions from your remotes appear here.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 6.dp))
        }
    } else LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp), contentPadding = PaddingValues(bottom = 16.dp)) {
        items(activities, key = { it.key }) { activity ->
            val online = activity.hostId in connectedHosts
            Surface(onClick = { onOpen(activity.hostId, activity.threadId) }, color = Mocha.Mantle, shape = RoundedCornerShape(10.dp)) {
                Row(Modifier.fillMaxWidth().padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                    if (online && activity.waiting > 0) Text("?", color = Mocha.Red, fontWeight = FontWeight.SemiBold, modifier = Modifier.width(24.dp))
                    else if (online && activity.running) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 1.5.dp, color = Mocha.Blue)
                    else ChatGlyph(ChatIcon.Computer, color = Mocha.Subtext)
                    Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                        Text(activity.title, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(hostNames[activity.hostId].orEmpty(), style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(when {
                            !online -> "Reconnect to verify activity"
                            activity.waiting > 0 -> "${activity.waiting} waiting for input"
                            else -> "Running"
                        }, style = MaterialTheme.typography.labelSmall, color = if (online && activity.waiting > 0) Mocha.Red else Mocha.Subtext, modifier = Modifier.padding(top = 3.dp))
                    }
                    ChatGlyph(ChatIcon.ChevronRight, color = Mocha.Subtext)
                }
            }
        }
    }
}
