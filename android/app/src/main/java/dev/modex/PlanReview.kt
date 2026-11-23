package dev.modex

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
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
    val canChoose = enabled && !pending
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = Mocha.Mantle,
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, Mocha.Surface),
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ChatGlyph(ChatIcon.File, color = Mocha.Blue)
                Text("Implement this plan?", Modifier.weight(1f), style = MaterialTheme.typography.titleSmall)
                if (pending) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            }
            if (pending) Text("Checking plan… Do not resend if disconnected.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
            listOf(
                Triple("implement", "Implement in this session", ChatIcon.Send),
                Triple("fresh", "Clear context and implement in new chat", ChatIcon.New),
                Triple("stay", "Stay in Plan mode", ChatIcon.Edit),
            ).forEach { (action, label, icon) ->
                val primary = canChoose && action == "implement"
                Surface(
                    onClick = { onAction(action) },
                    enabled = canChoose,
                    modifier = Modifier.fillMaxWidth().semantics { role = Role.Button },
                    color = if (primary) Mocha.Blue else Mocha.Surface,
                    contentColor = if (primary) Mocha.Base else if (canChoose) Mocha.Text else Mocha.Subtext,
                    shape = MaterialTheme.shapes.small,
                ) {
                    Row(
                        Modifier.fillMaxWidth().heightIn(min = 48.dp).padding(horizontal = 12.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        ChatGlyph(icon)
                        Text(label, Modifier.weight(1f), style = MaterialTheme.typography.bodyMedium)
                    }
                }
            }
        }
    }
}
