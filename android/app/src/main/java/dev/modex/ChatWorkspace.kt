package dev.modex

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch

internal enum class ChatIcon { Menu, New, Send, More, Close }

@Composable
internal fun ChatGlyph(icon: ChatIcon, description: String? = null) {
    val color = LocalContentColor.current
    Canvas(Modifier.size(22.dp).semantics { if (description != null) contentDescription = description }) {
        val u = size.width / 24
        fun line(x: Float, y: Float, x2: Float, y2: Float) = drawLine(color, Offset(x*u,y*u), Offset(x2*u,y2*u), 1.8f*u, StrokeCap.Round)
        when (icon) {
            ChatIcon.Menu -> { line(4f,6f,20f,6f); line(4f,12f,16f,12f); line(4f,18f,20f,18f) }
            ChatIcon.New -> { line(5f,19f,9f,18f); line(9f,18f,20f,7f); line(20f,7f,17f,4f); line(17f,4f,6f,15f); line(6f,15f,5f,19f); line(14f,7f,17f,10f) }
            ChatIcon.Send -> { line(12f,19f,12f,5f); line(6f,11f,12f,5f); line(12f,5f,18f,11f) }
            ChatIcon.Close -> { line(6f,6f,18f,18f); line(18f,6f,6f,18f) }
            ChatIcon.More -> for (y in listOf(5f,12f,19f)) drawCircle(color,1.4f*u,Offset(12*u,y*u))
        }
    }
}

@Composable
internal fun ChatWorkspace(
    host: String, connected: Boolean, connecting: Boolean, selected: String,
    threads: List<String>, titles: Map<String,String>, messages: List<Pair<String,String>>,
    busy: Boolean, historyLoading: Boolean, hasMore: Boolean, error: String,
    onSelect: (String) -> Unit, onRefresh: () -> Unit, onMore: () -> Unit,
    onCreate: (String) -> Unit, onSend: (String) -> Boolean,
    onConnection: () -> Unit, onDisconnect: () -> Unit
) {
    val drawer = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    BackHandler(drawer.isOpen) { scope.launch { drawer.close() } }
    var search by rememberSaveable { mutableStateOf("") }
    var drafts by rememberSaveable { mutableStateOf(hashMapOf<String,String>()) }
    val prompt = drafts[selected].orEmpty()
    fun updatePrompt(value: String) { drafts = HashMap(drafts).apply { put(selected, value) } }
    var newChat by rememberSaveable { mutableStateOf(false) }
    var model by rememberSaveable { mutableStateOf("") }
    var menu by remember { mutableStateOf(false) }
    val canSwitch = connected && !busy && !historyLoading
    val status = when { connecting -> "Connecting…"; !connected -> "Disconnected"; historyLoading -> "Loading history…"; busy -> "Working…"; else -> "Connected · SSH" }
    ModalNavigationDrawer(
        drawerState = drawer,
        drawerContent = {
            ModalDrawerSheet(drawerContainerColor = Mocha.Mantle, drawerShape = RoundedCornerShape(0.dp), modifier = Modifier.widthIn(max = 320.dp).fillMaxHeight()) {
                Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 8.dp, top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("modex", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                    IconButton(onClick = { scope.launch { drawer.close() } }) { ChatGlyph(ChatIcon.Close,"Close sessions") }
                }
                FilledTonalButton(onClick = { newChat = true }, enabled = canSwitch, shape = RoundedCornerShape(10.dp), colors = ButtonDefaults.filledTonalButtonColors(containerColor = MaterialTheme.colorScheme.primaryContainer, contentColor = Mocha.Blue), modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp), contentPadding = PaddingValues(14.dp)) {
                    ChatGlyph(ChatIcon.New); Spacer(Modifier.width(12.dp)); Text("New chat"); Spacer(Modifier.weight(1f))
                }
                OutlinedTextField(search, { search = it }, placeholder = { Text("Search loaded sessions") }, singleLine = true, modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp), shape = RoundedCornerShape(10.dp))
                Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 8.dp, top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("Conversations", style = MaterialTheme.typography.labelLarge, color = Mocha.Subtext, modifier = Modifier.weight(1f))
                    TextButton(onClick = onRefresh, enabled = connected) { Text("Refresh") }
                }
                val visible = threads.filter { (titles[it] ?: it).contains(search, ignoreCase = true) || it.contains(search,ignoreCase = true) }
                LazyColumn(Modifier.weight(1f), contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    if (visible.isEmpty()) item { Text(if (search.isBlank()) "No conversations yet" else "No matching conversations", Modifier.padding(12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium) }
                    items(visible, key = { it }) { id ->
                        Surface(onClick = { onSelect(id); scope.launch { drawer.close() } }, enabled = canSwitch, color = if (id == selected) MaterialTheme.colorScheme.primaryContainer else Mocha.Mantle, shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth()) {
                            Column(Modifier.padding(horizontal = 12.dp, vertical = 14.dp)) {
                                Text(titles[id] ?: "Untitled session", maxLines = 2, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyMedium, color = if (id == selected) Mocha.Blue else Mocha.Text)
                                Text(id.take(8), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext, modifier = Modifier.padding(top = 4.dp))
                            }
                        }
                    }
                    if (hasMore) item { TextButton(onClick = onMore, enabled = connected) { Text("Load more") } }
                }
                HorizontalDivider(color = Mocha.Surface)
                TextButton(onClick = { scope.launch { drawer.close() }; onConnection() }, modifier = Modifier.fillMaxWidth().padding(8.dp)) {
                    Column(Modifier.fillMaxWidth().padding(4.dp)) { Text(host.ifBlank { "Connection settings" }, maxLines = 1, overflow = TextOverflow.Ellipsis); Text(status, style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) }
                }
            }
        }
    ) {
        Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding()) {
            Row(Modifier.fillMaxWidth().heightIn(min = 64.dp).padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = { scope.launch { drawer.open() }; if (connected) onRefresh() }) { ChatGlyph(ChatIcon.Menu,"Open sessions") }
                Column(Modifier.weight(1f).padding(horizontal = 8.dp)) {
                    Text(if (selected.isBlank()) "modex" else titles[selected] ?: "Conversation", style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, fontWeight = FontWeight.SemiBold)
                    Text(status, style = MaterialTheme.typography.labelSmall, color = if (connected) Mocha.Blue else Mocha.Subtext)
                }
                Box {
                    IconButton(onClick = { menu = true }) { ChatGlyph(ChatIcon.More,"Connection menu") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(text = { Text("Connection settings") }, onClick = { menu = false; onConnection() })
                        if (connected || connecting) DropdownMenuItem(text = { Text("Disconnect") }, onClick = { menu = false; onDisconnect() })
                    }
                }
            }
            if (error.isNotBlank()) Surface(color = MaterialTheme.colorScheme.errorContainer, shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)) {
                SelectionContainer { Text(error, Modifier.padding(12.dp), style = MaterialTheme.typography.bodySmall) }
            }
            if (historyLoading || connecting) LinearProgressIndicator(Modifier.fillMaxWidth(), color = Mocha.Blue, trackColor = Mocha.Surface)
            val scroll = rememberLazyListState()
            LaunchedEffect(selected, messages.lastOrNull()?.second) {
                val nearBottom = scroll.layoutInfo.visibleItemsInfo.lastOrNull()?.index?.let { it >= messages.lastIndex - 2 } ?: true
                if (messages.isNotEmpty() && nearBottom) scroll.scrollToItem(messages.lastIndex)
            }
            if (messages.isEmpty()) {
                Column(Modifier.weight(1f).fillMaxWidth().padding(32.dp), verticalArrangement = Arrangement.Center) {
                    Text("Your workspace.\nWithin reach.", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Medium)
                    Text(if (selected.isBlank()) "Open a conversation from the sidebar, or start a new chat." else "Ask Codex to explore, build or fix something.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.padding(top = 16.dp))
                    if (selected.isBlank()) TextButton(onClick = { newChat = true }, enabled = canSwitch, modifier = Modifier.padding(top = 12.dp)) { Text("New chat") }
                }
            } else LazyColumn(state = scroll, modifier = Modifier.weight(1f).fillMaxWidth(), contentPadding = PaddingValues(horizontal = 20.dp, vertical = 20.dp), verticalArrangement = Arrangement.spacedBy(24.dp)) {
                items(messages) { (role, body) ->
                    if (role == "You") Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                        Surface(color = Mocha.Surface, shape = RoundedCornerShape(14.dp), modifier = Modifier.fillMaxWidth(0.9f)) {
                            SelectionContainer { Text(body, Modifier.padding(14.dp), style = MaterialTheme.typography.bodyLarge) }
                        }
                    } else MarkdownMessage(body)
                }
            }
            PromptComposer(prompt, { updatePrompt(it) }, busy, connected && !historyLoading && selected.isNotBlank() && !busy) {
                if (onSend(prompt)) updatePrompt("")
            }
        }
    }
    if (newChat) AlertDialog(
        onDismissRequest = { newChat = false }, title = { Text("New conversation") },
        text = { Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Uses your host’s provider and workspace.", color = Mocha.Subtext)
            OutlinedTextField(model, { model = it }, label = { Text("Model (optional)") }, placeholder = { Text("Host default") }, singleLine = true)
        } },
        confirmButton = { TextButton(enabled = canSwitch, onClick = { onCreate(model.trim()); newChat = false; scope.launch { drawer.close() } }) { Text("Create chat") } },
        dismissButton = { TextButton(onClick = { newChat = false }) { Text("Cancel") } }
    )
}

@Composable
private fun PromptComposer(prompt: String, onChange: (String) -> Unit, busy: Boolean, enabled: Boolean, onSend: () -> Unit) {
    Surface(shape = RoundedCornerShape(12.dp), color = Mocha.Mantle, border = BorderStroke(1.dp, Mocha.Surface), modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 8.dp, bottom = 12.dp)) {
        Column(Modifier.padding(12.dp)) {
            BasicTextField(prompt, onChange, textStyle = MaterialTheme.typography.bodyLarge.copy(color = Mocha.Text), cursorBrush = SolidColor(Mocha.Blue), maxLines = 6,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).semantics { contentDescription = "Message Codex" },
                decorationBox = { inner -> Box(Modifier.padding(vertical = 8.dp)) { if (prompt.isEmpty()) Text("Message Codex…", color = Mocha.Subtext, style = MaterialTheme.typography.bodyLarge); inner() } })
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(if (busy) "Codex is working" else "Codex", style = MaterialTheme.typography.labelMedium, color = Mocha.Subtext, modifier = Modifier.weight(1f))
                FilledIconButton(onClick = onSend, enabled = enabled && prompt.isNotBlank(), shape = RoundedCornerShape(10.dp), modifier = Modifier.size(48.dp)) {
                    if (busy) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = Mocha.Blue) else ChatGlyph(ChatIcon.Send,"Send message")
                }
            }
        }
    }
}
