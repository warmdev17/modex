package dev.modex

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp

@Composable
internal fun AgentActivityButton(running: Int, onClick: () -> Unit) {
    val color = if (running > 0) Mocha.Blue else Mocha.Subtext
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    Box(
        Modifier.sizeIn(minWidth = 48.dp, minHeight = 48.dp)
            .clickable(interactionSource = interaction, indication = null, role = Role.Button,
                onClickLabel = "Show agent activity", onClick = onClick)
            .semantics { if (running > 0) stateDescription = "$running running" },
        contentAlignment = Alignment.Center,
    ) {
        Surface(shape = CircleShape, color = if (pressed) MaterialTheme.colorScheme.primaryContainer else Mocha.Mantle,
            border = BorderStroke(1.dp, Mocha.Surface)) {
            Row(
                Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                ChatGlyph(ChatIcon.Activity, color = color)
                Text(if (running > 0) "Agents $running" else "Agents", color = color, style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun AgentActivitySheet(
    agents: List<AgentTimeline>, hasOlder: Boolean, loading: Boolean,
    onLoadOlder: () -> Unit, onOpenAgent: ((AgentTimeline) -> Unit)? = null, onDismiss: () -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = Mocha.Mantle,
    ) {
        BoxWithConstraints(Modifier.fillMaxWidth()) {
            val listHeight = maxHeight * 0.72f
            Column(Modifier.padding(horizontal = 20.dp).padding(bottom = 16.dp)) {
                Row(
                    Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Text("Agent activity", style = MaterialTheme.typography.titleMedium, modifier = Modifier.semantics { heading() })
                    IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close agent activity") }
                }
                LazyColumn(Modifier.heightIn(max = listHeight), contentPadding = PaddingValues(bottom = 8.dp)) {
                    items(agents, key = { it.id }) { agent ->
                        HorizontalDivider(color = Mocha.Surface)
                        AgentActivityRow(agent, onOpenAgent?.let { open -> { open(agent) } })
                    }
                    if (hasOlder) item {
                        TextButton(onClick = onLoadOlder, enabled = !loading) {
                            Text(if (loading) "Loading earlier activity…" else "Load earlier activity")
                        }
                    }
                }
            }
        }
    }
}


@Composable
private fun AgentActivityRow(agent: AgentTimeline, onOpen: (() -> Unit)?) {
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val color = when (agent.status) {
        "running" -> Mocha.Blue
        "errored" -> Mocha.Red
        else -> Mocha.Subtext
    }
    val action = if (onOpen == null) Modifier else Modifier
        .clip(RoundedCornerShape(8.dp))
        .clickable(interactionSource = interaction, indication = null, role = Role.Button,
            onClickLabel = "Open agent conversation", onClick = onOpen)
        .background(if (pressed) Mocha.Surface else Color.Transparent)
    Column(Modifier.fillMaxWidth().then(action).padding(horizontal = 8.dp, vertical = 14.dp),
        verticalArrangement = Arrangement.spacedBy(5.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(agent.label, style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
            Text(agentStatusLabel(agent.status), style = MaterialTheme.typography.labelMedium, color = color)
            if (onOpen != null) ChatGlyph(ChatIcon.ChevronRight, color = Mocha.Subtext)
        }
        agent.milestones.forEach { milestone ->
            Text(milestone, color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
        }
    }
}
