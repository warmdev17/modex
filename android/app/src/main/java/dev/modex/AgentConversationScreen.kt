package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import kotlinx.coroutines.flow.distinctUntilChangedBy
import java.io.File
import kotlinx.coroutines.launch

@Composable
internal fun AgentConversationScreen(
    host: String, conversation: AgentConversation, agent: AgentTimeline,
    onHistory: (Boolean) -> Unit, loadAttachment: suspend (Attachment) -> File,
) {
    var preview by rememberSaveable(host, agent.id, saver = AttachmentPreviewStateSaver) { mutableStateOf<Attachment?>(null) }
    val timeline = remember(conversation.messages) {
        thinkingTimeline(conversation.messages).filterNot { it.role == "agent_activity" }
    }
    Dialog(onDismissRequest = conversation::close,
        properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Surface(Modifier.fillMaxSize(), color = Mocha.Base) {
            Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                Row(Modifier.fillMaxWidth().heightIn(min = 56.dp).padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    TextButton(onClick = conversation::close) { Text("Back", Modifier.semantics { contentDescription = "Back to agents" }) }
                    Column(Modifier.weight(1f).padding(horizontal = 8.dp)) {
                        Text(agent.label, Modifier.semantics { heading() }, style = MaterialTheme.typography.titleMedium,
                            maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(agentStatusLabel(if (conversation.busy) "running" else agent.status),
                            style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
                    }
                    Text("Read only", Modifier.padding(end = 8.dp), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
                }
                HorizontalDivider(color = Mocha.Surface)
                val scroll = rememberLazyListState()
                val scope = rememberCoroutineScope()
                var openAtLatest by remember(agent.id) { mutableStateOf(true) }
                var following by remember(agent.id) { mutableStateOf(true) }
                var autoScrolling by remember(agent.id) { mutableStateOf(false) }
                LaunchedEffect(agent.id) {
                    snapshotFlow { if (autoScrolling) null else scroll.isScrollInProgress to scroll.canScrollForward }.collect {
                        if (it?.first == true) following = !it.second
                    }
                }
                LaunchedEffect(agent.id) {
                    snapshotFlow { Triple(scroll.layoutInfo, following, scroll.isScrollInProgress) }
                        .distinctUntilChangedBy { (layout, follow, scrolling) ->
                            Triple(layout.totalItemsCount to layout.viewportSize,
                                layout.visibleItemsInfo.map { Triple(it.index, it.offset, it.size) },
                                follow to scrolling)
                        }
                        .collect { (layout, follow, scrolling) ->
                            if ((openAtLatest || follow) && !scrolling && layout.totalItemsCount > 0 &&
                                (openAtLatest || scroll.canScrollForward)) {
                            autoScrolling = true
                            try {
                                scroll.scrollToItem(layout.totalItemsCount - 1)
                                openAtLatest = false
                            }
                            finally { autoScrolling = false }
                        }
                    }
                }
                if (timeline.isEmpty() && conversation.loading) {
                    Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
                    }
                } else Box(Modifier.weight(1f).fillMaxWidth()) {
                    LazyColumn(state = scroll, modifier = Modifier.fillMaxSize().semantics { contentDescription = "Agent conversation history" },
                        contentPadding = PaddingValues(horizontal = 20.dp, vertical = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(16.dp)) {
                        if (conversation.cursor.isNotBlank()) item(key = "agent-older") {
                            TextButton(onClick = { following = false; onHistory(true) }, enabled = !conversation.loading) {
                                Text(if (conversation.loading) "Loading earlier activity…" else "Load earlier activity")
                            }
                        }
                        if (timeline.isEmpty()) item(key = "agent-empty") {
                            Text("No activity yet", color = Mocha.Subtext)
                        }
                        itemsIndexed(timeline, key = { index, entry ->
                            if (entry.id.isBlank()) "legacy-$index" else "${entry.turnId}/${entry.role}/${entry.id}"
                        }) { _, entry ->
                            if (entry.role == "reasoning") {
                                val summary = reasoningLabel(entry.summaryParts.lastOrNull { it.isNotBlank() } ?: entry.text)
                                if (summary.isNotBlank()) Text(summary, color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                            } else ChatTranscriptEntry(entry, showWork = true, loadAttachment, { preview = it })
                        }
                        item(key = "agent-end") { Spacer(Modifier.height(1.dp)) }
                    }
                    ScrollToLatestButton(!openAtLatest && !autoScrolling && scroll.canScrollForward) {
                        scope.launch {
                            if (autoScrolling) return@launch
                            autoScrolling = true
                            try {
                                scroll.returnToLatest()
                                following = true
                            } finally { autoScrolling = false }
                        }
                    }
                }
                if (conversation.error) {
                    Row(Modifier.fillMaxWidth().padding(horizontal = 20.dp), verticalAlignment = Alignment.CenterVertically) {
                        Text("Couldn’t load agent history", Modifier.weight(1f), color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                        TextButton(onClick = { onHistory(conversation.cursor.isNotBlank()) }) { Text("Retry") }
                    }
                }
            }
        }
    }
    preview?.let { AttachmentViewer(it, loadAttachment, host) { preview = null } }
}
