package dev.modex

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.horizontalScroll
import androidx.compose.ui.res.painterResource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.first
import org.json.JSONObject

internal enum class ChatIcon { Menu, New, Send, More, Close, Model, Folder, Attach, File, Image, Phone, Computer }

@Composable
internal fun ChatGlyph(icon: ChatIcon, description: String? = null) {
    val color = LocalContentColor.current
    if (icon == ChatIcon.Model) {
        Icon(painterResource(R.drawable.ic_openai), description, Modifier.size(22.dp), tint = color)
        return
    }
    Canvas(Modifier.size(22.dp).semantics { if (description != null) contentDescription = description }) {
        val u = size.width / 24
        fun line(x: Float, y: Float, x2: Float, y2: Float) = drawLine(color, Offset(x*u,y*u), Offset(x2*u,y2*u), 1.8f*u, StrokeCap.Round)
        when (icon) {
            ChatIcon.Menu -> { line(4f,6f,20f,6f); line(4f,12f,16f,12f); line(4f,18f,20f,18f) }
            ChatIcon.New -> { line(5f,19f,9f,18f); line(9f,18f,20f,7f); line(20f,7f,17f,4f); line(17f,4f,6f,15f); line(6f,15f,5f,19f); line(14f,7f,17f,10f) }
            ChatIcon.Send -> { line(12f,19f,12f,5f); line(6f,11f,12f,5f); line(12f,5f,18f,11f) }
            ChatIcon.Close -> { line(6f,6f,18f,18f); line(18f,6f,6f,18f) }
            ChatIcon.More -> for (y in listOf(5f,12f,19f)) drawCircle(color,1.4f*u,Offset(12*u,y*u))
            ChatIcon.Folder -> { line(3f,7f,3f,19f); line(3f,19f,21f,19f); line(21f,19f,21f,8f); line(21f,8f,11f,8f); line(11f,8f,9f,5f); line(9f,5f,3f,5f); line(3f,5f,3f,7f) }
            ChatIcon.Model -> Unit
            ChatIcon.Attach -> { line(5f,12f,19f,12f); line(12f,5f,12f,19f) }
            ChatIcon.File -> { line(5f,3f,14f,3f); line(14f,3f,19f,8f); line(19f,8f,19f,21f); line(19f,21f,5f,21f); line(5f,21f,5f,3f); line(14f,3f,14f,8f); line(14f,8f,19f,8f) }
            ChatIcon.Image -> { line(3f,4f,21f,4f); line(21f,4f,21f,20f); line(21f,20f,3f,20f); line(3f,20f,3f,4f); line(3f,17f,9f,11f); line(9f,11f,15f,17f); line(13f,15f,17f,11f); line(17f,11f,21f,15f); drawCircle(color,1.5f*u,Offset(16*u,8*u)) }
            ChatIcon.Phone -> { line(7f,3f,17f,3f); line(17f,3f,17f,21f); line(17f,21f,7f,21f); line(7f,21f,7f,3f); line(11f,18f,13f,18f) }
            ChatIcon.Computer -> { line(3f,4f,21f,4f); line(21f,4f,21f,16f); line(21f,16f,3f,16f); line(3f,16f,3f,4f); line(12f,16f,12f,21f); line(8f,21f,16f,21f) }
        }
    }
}

@Composable
internal fun ChatWorkspace(
    host: String, connected: Boolean, connecting: Boolean, selected: String,
    threads: List<String>, titles: Map<String,String>, messages: List<ChatEntry>,
    busy: Boolean, historyLoading: Boolean, hasMore: Boolean, error: String, sessionSettings: String,
    sessionModel: String, decisions: List<JSONObject>, answeringDecisionIds: Set<String>,
    onSelect: (String) -> Unit, onRefresh: () -> Unit, onMore: () -> Unit,
    onCreate: (String, String, String) -> Unit, onSend: (String, String, String) -> Boolean, onAnswer: (JSONObject, JSONObject) -> Unit,
    onConnection: () -> Unit, onDisconnect: () -> Unit,
    models: List<ModelOption> = emptyList(), modelCatalogStatus: String = "",
    sessionEffort: String = "", hostModel: String = "", hostEffort: String = "", onModelChoice: (String, String) -> Unit = { _, _ -> },
    threadWorkspaces: Map<String,String> = emptyMap(), threadListLoading: Boolean = false,
    registeredProjects: List<RemoteProject> = emptyList(), projectMembership: Map<String,String> = emptyMap(), projectStatus: String = "",
    attachments: List<Attachment> = emptyList(), attachmentWorking: Boolean = false,
    attachmentProgress: String = "", attachmentError: String = "",
    loadAttachment: suspend (Attachment) -> java.io.File = { error("Reconnect to open attachment") },
    onPhoneFiles: () -> Unit = {}, onRemoteFiles: () -> Unit = {}, onRemoveAttachment: (String) -> Unit = {}
) {
    var preview by remember(selected) { mutableStateOf<Attachment?>(null) }
    preview?.let { file -> AttachmentViewer(file, loadAttachment) { preview = null } }
    val drawer = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    BackHandler(drawer.isOpen) { scope.launch { drawer.close() } }
    var collapsedProjects by rememberSaveable { mutableStateOf(arrayListOf<String>()) }
    var search by rememberSaveable { mutableStateOf("") }
    var drafts by rememberSaveable { mutableStateOf(hashMapOf<String,String>()) }
    val prompt = drafts[selected].orEmpty()
    fun updatePrompt(value: String) { drafts = HashMap(drafts).apply { put(selected, value) } }
    var newChat by rememberSaveable { mutableStateOf(false) }
    var permissionMode by rememberSaveable { mutableStateOf("host-default") }
    LaunchedEffect(newChat) { if (newChat) permissionMode = "host-default" }
    var showSettings by rememberSaveable { mutableStateOf(false) }
    var model by rememberSaveable { mutableStateOf("") }
    var effortInput by rememberSaveable { mutableStateOf("") }
    var newEffort by rememberSaveable { mutableStateOf("") }
    fun defaultEffort(id: String) = if (id == hostModel && hostEffort.isNotBlank()) hostEffort else models.firstOrNull { it.model == id }?.defaultEffort.orEmpty()
    LaunchedEffect(newChat) { if (newChat) { model = hostModel; newEffort = defaultEffort(hostModel) } }
    var showModel by rememberSaveable { mutableStateOf(false) }
    var modelInput by rememberSaveable { mutableStateOf("") }
    val selectedDecisions = decisions.filter { it.optString("thread_id") == selected }
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
                OutlinedTextField(search, { search = it }, placeholder = { Text("Search sessions or projects") }, singleLine = true, modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp), shape = RoundedCornerShape(10.dp))
                Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 8.dp, top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("Sessions", style = MaterialTheme.typography.labelLarge, color = Mocha.Subtext, modifier = Modifier.weight(1f))
                    TextButton(onClick = onRefresh, enabled = connected && !threadListLoading) { Text("Refresh") }
                }
                val visible = threads.filter { (titles[it] ?: it).contains(search, ignoreCase = true) || it.contains(search,ignoreCase = true) || threadWorkspaces[it].orEmpty().contains(search, ignoreCase = true) }
                val projects = projectSessions(registeredProjects, visible, projectMembership)
                LazyColumn(Modifier.weight(1f), contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    if (visible.isEmpty()) item { Text(if (threadListLoading) "Loading sessions…" else if (search.isBlank()) "No conversations yet" else "No matching conversations", Modifier.padding(12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium) }
                    item { Text("Projects", Modifier.padding(horizontal = 12.dp, vertical = 10.dp), color = Mocha.Subtext, style = MaterialTheme.typography.labelLarge) }
                    if (projectStatus.isNotEmpty()) item { Text(projectStatus, Modifier.padding(12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall) }
                    registeredProjects.filter { search.isBlank() || projects[it.id].orEmpty().isNotEmpty() || it.name.contains(search, true) || it.roots.any { root -> root.contains(search,true) } }.forEach { project ->
                        val path = project.id
                        val ids = projects[path].orEmpty()
                        item(key = "folder:$path") {
                            Surface(onClick = { collapsedProjects = ArrayList(collapsedProjects).apply { if (path in this) remove(path) else add(path) } }, color = Mocha.Mantle, shape = RoundedCornerShape(6.dp)) {
                                Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                                    ChatGlyph(ChatIcon.Folder)
                                    Column(Modifier.weight(1f).padding(start = 8.dp)) {
                                        Text(project.name.ifBlank { projectName(project.roots.firstOrNull().orEmpty()) }, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                        if (project.roots.isNotEmpty()) Text(project.roots.joinToString(" · "), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    }
                                    Text(if (path in collapsedProjects) "›" else "⌄", color = Mocha.Subtext)
                                }
                            }
                        }
                        if (path !in collapsedProjects || search.isNotEmpty()) items(ids, key = { "project:$it" }) { id ->
                            SessionSidebarRow(id, titles[id], id == selected, canSwitch, Modifier.padding(start = 20.dp)) { onSelect(id); scope.launch { drawer.close() } }
                        }
                    }
                    if (visible.isNotEmpty()) item { Text("Recents", Modifier.padding(horizontal = 12.dp, vertical = 12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.labelLarge) }
                    items(visible, key = { "recent:$it" }) { id ->
                        SessionSidebarRow(id, titles[id], id == selected, canSwitch) { onSelect(id); scope.launch { drawer.close() } }
                    }
                    if (threadListLoading) item { Text("Loading sessions…", Modifier.padding(12.dp), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) }
                    if (hasMore && !threadListLoading) item { TextButton(onClick = onMore, enabled = connected) { Text("Load more") } }
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
                        DropdownMenuItem(text = { Text("Session settings") }, enabled = selected.isNotBlank(), onClick = { menu = false; showSettings = true })
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
            var openAtLatest by remember(selected) { mutableStateOf(true) }
            LaunchedEffect(selected, historyLoading, messages.size, messages.lastOrNull(), selectedDecisions.map { it.optString("decision_id") }) {
                if (historyLoading) {
                    openAtLatest = true
                    return@LaunchedEffect
                }
                val layout = scroll.layoutInfo
                val nearBottom = layout.visibleItemsInfo.lastOrNull()?.index?.let { it >= layout.totalItemsCount - 2 } ?: true
                if (messages.isNotEmpty() && (openAtLatest || nearBottom)) {
                    snapshotFlow { scroll.layoutInfo.totalItemsCount }.first { it == messages.size + selectedDecisions.size + 1 }
                    // The final spacer anchors the bottom even when the last message is taller than the viewport.
                    scroll.scrollToItem(messages.size)
                    openAtLatest = false
                }
            }
            if (messages.isEmpty() && selectedDecisions.isEmpty()) {
                Column(Modifier.weight(1f).fillMaxWidth().padding(32.dp), verticalArrangement = Arrangement.Center) {
                    Text("Your workspace.\nWithin reach.", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Medium)
                    Text(if (selected.isBlank()) "Open a conversation from the sidebar, or start a new chat." else "Ask Codex to explore, build or fix something.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.padding(top = 16.dp))
                    if (selected.isBlank()) TextButton(onClick = { newChat = true }, enabled = canSwitch, modifier = Modifier.padding(top = 12.dp)) { Text("New chat") }
                }
            } else LazyColumn(state = scroll, modifier = Modifier.weight(1f).fillMaxWidth(), contentPadding = PaddingValues(horizontal = 20.dp, vertical = 20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                items(messages) { entry ->
                    val role = entry.role; val body = entry.text
                    if (role == "You") Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                        Surface(color = Mocha.Surface, shape = RoundedCornerShape(14.dp), modifier = Modifier.fillMaxWidth(0.9f)) {
                            Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                                if (entry.attachments.isNotEmpty()) Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    entry.attachments.forEach { file -> AttachmentTile(file, loadAttachment, { preview = file }) }
                                }
                                if (body.isNotBlank()) SelectionContainer { Text(body, style = MaterialTheme.typography.bodyLarge) }
                            }
                        }
                    } else if (role == "command") CommandCard(entry) else MarkdownMessage(body)
                }
                items(selectedDecisions, key = { "decision-${it.optString("decision_id")}" }) { decision ->
                    InlineDecisionCard(decision, decision.optString("decision_id") in answeringDecisionIds, connected) { onAnswer(decision, it) }
                }
                item { Spacer(Modifier.height(1.dp)) }
            }
            PromptComposer(
                prompt, { updatePrompt(it) }, busy,
                (sessionModel.ifBlank { "Loading model…" } + " · " + sessionEffort.ifBlank { defaultEffort(sessionModel).ifBlank { "Reasoning unavailable" } }), connected && selected.isNotBlank(),
                { modelInput = sessionModel; effortInput = sessionEffort.ifBlank { defaultEffort(sessionModel) }; showModel = true },
                connected && !historyLoading && selected.isNotBlank() && !busy && !attachmentWorking,
                attachments, attachmentWorking, attachmentProgress, attachmentError,
                connected && selected.isNotBlank() && !busy && !attachmentWorking,
                onPhoneFiles, onRemoteFiles, onRemoveAttachment, loadAttachment, { preview = it }
            ) {
                if (onSend(prompt, sessionModel, sessionEffort.ifBlank { defaultEffort(sessionModel) })) updatePrompt("")
            }
        }
    }
    if (newChat) AlertDialog(
        onDismissRequest = { newChat = false }, title = { Text("New conversation") },
        text = { Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Uses your host’s provider. Folder: ${threadWorkspaces[selected].orEmpty().ifBlank { "Host workspace" }}", color = Mocha.Subtext)
            ModelPicker(models, modelCatalogStatus, model, { model = it; newEffort = defaultEffort(it) })
            ReasoningControl(models.firstOrNull { it.model == model }, newEffort) { newEffort = it }
            Column(Modifier.selectableGroup()) {
                permissionModes.forEach { (mode, label) ->
                    Row(Modifier.fillMaxWidth().selectable(selected = permissionMode == mode, role = Role.RadioButton, onClick = { permissionMode = mode }).heightIn(min = 48.dp), verticalAlignment = Alignment.CenterVertically) {
                        RadioButton(selected = permissionMode == mode, onClick = null)
                        Spacer(Modifier.width(12.dp))
                        Text(label)
                    }
                }
            }
            Text(when (permissionMode) {
                "full-access" -> "Unrestricted access with no command approval prompts."
                "host-default" -> "Inherits your host configuration, which may allow full access."
                else -> "Requests user approval when sandbox restrictions need to be lifted. Host policy may restrict this mode."
            }, style = MaterialTheme.typography.bodySmall, color = if (permissionMode == "full-access") MaterialTheme.colorScheme.error else Mocha.Subtext)
        } },
        confirmButton = { TextButton(enabled = canSwitch && model.isNotBlank() && validModelId(model), onClick = { onCreate(model.trim(), permissionMode, newEffort); newChat = false; scope.launch { drawer.close() } }) { Text("Create chat") } },
        dismissButton = { TextButton(onClick = { newChat = false }) { Text("Cancel") } }
    )
    if (showSettings) AlertDialog(
        onDismissRequest = { showSettings = false }, title = { Text("Session settings") },
        text = { SelectionContainer { Text(sessionSettings.ifBlank { "Effective permissions are unknown. Select this session while connected to read Codex's resume settings." } + "\n\nReported at create/resume. Host policy and tool-specific approvals still apply.") } },
        confirmButton = { TextButton(onClick = { showSettings = false }) { Text("Close") } }
    )
    if (showModel) AlertDialog(
        onDismissRequest = { showModel = false },
        title = { Text("Model for next turn") },
        text = { Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Codex applies this model to the next turn and later turns in this session.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
            ModelPicker(models, modelCatalogStatus, modelInput, { modelInput = it; effortInput = defaultEffort(it) })
            ReasoningControl(models.firstOrNull { it.model == modelInput.trim() }, effortInput) { effortInput = it }

        } },
        confirmButton = { TextButton(enabled = modelInput.isNotBlank() && validModelId(modelInput), onClick = {
            onModelChoice(modelInput.trim(), effortInput)
            showModel = false
        }) { Text("Use model") } },
        dismissButton = { TextButton(onClick = { showModel = false }) { Text("Cancel") } }
    )
}

@Composable
private fun PromptComposer(
    prompt: String, onChange: (String) -> Unit, busy: Boolean, model: String, modelEnabled: Boolean, onModel: () -> Unit, enabled: Boolean,
    attachments: List<Attachment>, attachmentWorking: Boolean, attachmentProgress: String, attachmentError: String,
    attachEnabled: Boolean, onPhoneFiles: () -> Unit, onRemoteFiles: () -> Unit, onRemove: (String) -> Unit, loadAttachment: suspend (Attachment) -> java.io.File, onPreview: (Attachment) -> Unit, onSend: () -> Unit
) {
    var attachMenu by remember { mutableStateOf(false) }
    Surface(shape = RoundedCornerShape(12.dp), color = Mocha.Mantle, border = BorderStroke(1.dp, Mocha.Surface), modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 8.dp, bottom = 12.dp)) {
        Column(Modifier.padding(12.dp)) {
            if (attachments.isNotEmpty()) Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                attachments.forEach { file -> AttachmentTile(file, loadAttachment, { onPreview(file) }, if (!busy && !attachmentWorking) ({ onRemove(file.path) }) else null) }
            }
            if (attachmentWorking) Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                Text(attachmentProgress, Modifier.padding(start = 8.dp), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            if (attachmentError.isNotEmpty()) Text(attachmentError, style = MaterialTheme.typography.bodySmall, color = Mocha.Red)
            BasicTextField(prompt, onChange, textStyle = MaterialTheme.typography.bodyLarge.copy(color = Mocha.Text), cursorBrush = SolidColor(Mocha.Blue), maxLines = 6,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp).semantics { contentDescription = "Message Codex" },
                decorationBox = { inner -> Box(Modifier.padding(vertical = 8.dp)) { if (prompt.isEmpty()) Text("Message Codex…", color = Mocha.Subtext, style = MaterialTheme.typography.bodyLarge); inner() } })
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Box {
                    IconButton(onClick = { attachMenu = true }, enabled = attachEnabled) { ChatGlyph(ChatIcon.Attach, "Attach files") }
                    DropdownMenu(expanded = attachMenu, onDismissRequest = { attachMenu = false }) {
                        DropdownMenuItem(text = { Text("From this phone") }, leadingIcon = { ChatGlyph(ChatIcon.Phone) }, enabled = attachEnabled, onClick = { attachMenu = false; onPhoneFiles() })
                        DropdownMenuItem(text = { Text("From connected computer") }, leadingIcon = { ChatGlyph(ChatIcon.Computer) }, enabled = attachEnabled, onClick = { attachMenu = false; onRemoteFiles() })
                    }
                }
                TextButton(
                    onClick = onModel, enabled = modelEnabled, shape = RoundedCornerShape(6.dp),
                    contentPadding = PaddingValues(horizontal = 4.dp), modifier = Modifier.weight(1f).heightIn(min = 48.dp).semantics { contentDescription = "Choose model" }
                ) {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.Start) {
                        ChatGlyph(ChatIcon.Model)
                        Spacer(Modifier.width(6.dp))
                        Text(model, modifier = Modifier.weight(1f, fill = false), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.labelMedium)
                        if (busy) { Spacer(Modifier.width(6.dp)); Text("working", color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall) }
                    }
                }
                FilledIconButton(onClick = onSend, enabled = enabled && (prompt.isNotBlank() || attachments.isNotEmpty()), shape = RoundedCornerShape(10.dp), modifier = Modifier.size(48.dp)) {
                    if (busy) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = Mocha.Blue) else ChatGlyph(ChatIcon.Send,"Send message")
                }
            }
        }
    }
}

@Composable
private fun SessionSidebarRow(id: String, title: String?, selected: Boolean, enabled: Boolean, modifier: Modifier = Modifier, onClick: () -> Unit) {
    Surface(onClick = onClick, enabled = enabled, color = if (selected) Mocha.Blue else Mocha.Mantle, contentColor = if (selected) Mocha.Base else Mocha.Text, shape = RoundedCornerShape(6.dp), modifier = modifier.fillMaxWidth().semantics { this.selected = selected }) {
        Text(title ?: "Untitled session · ${id.take(8)}", maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(horizontal = 12.dp, vertical = 11.dp))
    }
}
