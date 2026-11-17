package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

internal fun planKey(entry: ChatEntry): String = "${entry.turnId}:${entry.id}"

internal fun pendingPlan(entries: List<ChatEntry>, busy: Boolean, handled: Set<String>): ChatEntry? {
    if (busy) return null
    val latestTurn = entries.lastOrNull { it.turnId.isNotBlank() }?.turnId ?: return null
    return entries.lastOrNull { it.role == "plan" && it.turnId == latestTurn }
        ?.takeIf { it.id.isNotBlank() && it.text.isNotBlank() && it.status == "completed" && !it.planHandled && planKey(it) !in handled }
}

@Composable
internal fun PlanReviewCard(enabled: Boolean, pending: Boolean, onAction: (String) -> Unit) {
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("Implement this plan?", style = MaterialTheme.typography.titleSmall, color = Mocha.Text)
        if (pending) Text("Checking plan… Do not resend if disconnected.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        listOf("implement" to "Implement in this session", "fresh" to "Clear context and implement in new chat", "stay" to "Stay in Plan mode").forEach { (action, label) ->
            TextButton(enabled = enabled && !pending, onClick = { onAction(action) }, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp)) { Text(label) }
        }
    }
}
