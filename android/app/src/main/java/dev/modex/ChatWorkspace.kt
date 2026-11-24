package dev.modex

import androidx.activity.compose.BackHandler
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.LocalIndication
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.ui.res.painterResource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
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
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.distinctUntilChangedBy
import org.json.JSONObject

internal enum class ChatIcon { Menu, New, Send, More, Close, Model, ModelChoice, Folder, Attach, File, Image, Phone, Computer, Skill, Plugin, ChevronRight, ChevronDown, Down, Stop, Error, Terminal, Settings, Activity, Archive, Restore, Fork, Delete, Edit, Copy, SelectText, Disconnect }

@Composable
internal fun ChatGlyph(icon: ChatIcon, description: String? = null, color: Color = LocalContentColor.current) {
    if (icon == ChatIcon.Model) {
        Icon(painterResource(R.drawable.ic_openai), description, Modifier.size(22.dp), tint = color)
        return
    }
    Canvas(Modifier.size(22.dp).semantics { if (description != null) contentDescription = description }) {
        val u = size.width / 24
        fun line(x: Float, y: Float, x2: Float, y2: Float) = drawLine(color, Offset(x*u,y*u), Offset(x2*u,y2*u), 1.8f*u, StrokeCap.Round)
        when (icon) {
            ChatIcon.Menu -> { line(4f,6f,20f,6f); line(4f,12f,16f,12f); line(4f,18f,20f,18f) }
            ChatIcon.New -> { line(10f,4f,5f,4f); line(5f,4f,4f,5f); line(4f,5f,4f,19f); line(4f,19f,5f,20f); line(5f,20f,19f,20f); line(19f,20f,20f,19f); line(20f,19f,20f,14f); line(10f,14f,14f,13f); line(14f,13f,21f,6f); line(21f,6f,18f,3f); line(18f,3f,11f,10f); line(11f,10f,10f,14f); line(16f,5f,19f,8f) }
            ChatIcon.Send -> { line(12f,19f,12f,5f); line(6f,11f,12f,5f); line(12f,5f,18f,11f) }
            ChatIcon.ChevronRight -> { line(9f,6f,15f,12f); line(15f,12f,9f,18f) }
            ChatIcon.ChevronDown -> { line(6f,9f,12f,15f); line(12f,15f,18f,9f) }
            ChatIcon.Down -> { line(12f,5f,12f,19f); line(6f,13f,12f,19f); line(12f,19f,18f,13f) }
            ChatIcon.Stop -> drawRoundRect(color, Offset(6*u,6*u), Size(12*u,12*u), CornerRadius(u,u))
            ChatIcon.Error -> {
                drawCircle(color, 9.5f * u, style = androidx.compose.ui.graphics.drawscope.Stroke(1.8f * u))
                line(12f, 7f, 12f, 12f)
                drawCircle(color, 1.1f * u, Offset(12f * u, 16f * u))
            }
            ChatIcon.Close -> { line(6f,6f,18f,18f); line(18f,6f,6f,18f) }
            ChatIcon.More -> for (y in listOf(5f,12f,19f)) drawCircle(color,1.4f*u,Offset(12*u,y*u))
            ChatIcon.Folder -> { line(3f,7f,3f,19f); line(3f,19f,21f,19f); line(21f,19f,21f,8f); line(21f,8f,11f,8f); line(11f,8f,9f,5f); line(9f,5f,3f,5f); line(3f,5f,3f,7f) }
            ChatIcon.Plugin -> { line(8f,3f,8f,8f); line(16f,3f,16f,8f); line(5f,8f,19f,8f); line(5f,8f,5f,13f); line(19f,8f,19f,13f); line(5f,13f,9f,17f); line(19f,13f,15f,17f); line(9f,17f,15f,17f); line(12f,17f,12f,22f) }
            ChatIcon.Skill -> { line(4f,4f,10f,4f); line(10f,4f,12f,6f); line(12f,6f,14f,4f); line(14f,4f,20f,4f); line(20f,4f,20f,19f); line(20f,19f,14f,19f); line(14f,19f,12f,21f); line(12f,21f,10f,19f); line(10f,19f,4f,19f); line(4f,19f,4f,4f); line(12f,6f,12f,21f) }
            ChatIcon.Activity -> { line(3f,12f,7f,12f); line(7f,12f,10f,5f); line(10f,5f,14f,19f); line(14f,19f,17f,12f); line(17f,12f,21f,12f) }
            ChatIcon.Terminal -> { line(3f,4f,21f,4f); line(21f,4f,21f,20f); line(21f,20f,3f,20f); line(3f,20f,3f,4f); line(7f,8f,11f,12f); line(11f,12f,7f,16f); line(13f,16f,17f,16f) }
            ChatIcon.Settings -> { line(4f,6f,8f,6f); line(12f,6f,20f,6f); line(4f,12f,14f,12f); line(18f,12f,20f,12f); line(4f,18f,6f,18f); line(10f,18f,20f,18f); for ((x,y) in listOf(10f to 6f,16f to 12f,8f to 18f)) drawCircle(color, 2f*u, Offset(x*u,y*u), style = androidx.compose.ui.graphics.drawscope.Stroke(width = 1.8f*u)) }
            ChatIcon.Model -> Unit
            ChatIcon.ModelChoice -> { line(6f,6f,18f,6f); line(18f,6f,18f,18f); line(18f,18f,6f,18f); line(6f,18f,6f,6f); for (v in listOf(9f,12f,15f)) { line(v,3f,v,6f); line(v,18f,v,21f); line(3f,v,6f,v); line(18f,v,21f,v) } }
            ChatIcon.Attach -> { line(5f,12f,19f,12f); line(12f,5f,12f,19f) }
            ChatIcon.File -> { line(5f,3f,14f,3f); line(14f,3f,19f,8f); line(19f,8f,19f,21f); line(19f,21f,5f,21f); line(5f,21f,5f,3f); line(14f,3f,14f,8f); line(14f,8f,19f,8f) }
            ChatIcon.Image -> { line(3f,4f,21f,4f); line(21f,4f,21f,20f); line(21f,20f,3f,20f); line(3f,20f,3f,4f); line(3f,17f,9f,11f); line(9f,11f,15f,17f); line(13f,15f,17f,11f); line(17f,11f,21f,15f); drawCircle(color,1.5f*u,Offset(16*u,8*u)) }
            ChatIcon.Phone -> { line(7f,3f,17f,3f); line(17f,3f,17f,21f); line(17f,21f,7f,21f); line(7f,21f,7f,3f); line(11f,18f,13f,18f) }
            ChatIcon.Computer -> { line(3f,4f,21f,4f); line(21f,4f,21f,16f); line(21f,16f,3f,16f); line(3f,16f,3f,4f); line(12f,16f,12f,21f); line(8f,21f,16f,21f) }
            ChatIcon.Archive -> { line(4f,7f,20f,7f); line(5f,7f,6f,20f); line(6f,20f,18f,20f); line(18f,20f,19f,7f); line(3f,4f,21f,4f); line(10f,11f,14f,11f) }
            ChatIcon.Restore -> { line(4f,7f,20f,7f); line(5f,7f,6f,20f); line(6f,20f,18f,20f); line(18f,20f,19f,7f); line(3f,4f,21f,4f); line(12f,16f,12f,10f); line(9f,13f,12f,10f); line(12f,10f,15f,13f) }
            ChatIcon.Fork -> { line(7f,4f,7f,9f); line(17f,4f,17f,8f); line(17f,8f,12f,13f); line(12f,13f,12f,20f); line(7f,9f,12f,14f); drawCircle(color, 1.7f*u, Offset(7*u,4*u)); drawCircle(color, 1.7f*u, Offset(17*u,4*u)); drawCircle(color, 1.7f*u, Offset(12*u,20*u)) }
            ChatIcon.Delete -> { line(5f,7f,19f,7f); line(8f,7f,8f,20f); line(8f,20f,16f,20f); line(16f,20f,16f,7f); line(9f,4f,15f,4f); line(11f,10f,11f,17f); line(13f,10f,13f,17f) }
            ChatIcon.Edit -> { line(5f,19f,9f,18f); line(9f,18f,20f,7f); line(20f,7f,17f,4f); line(17f,4f,6f,15f); line(6f,15f,5f,19f); line(15f,6f,18f,9f) }
            ChatIcon.Copy -> { line(8f,7f,20f,7f); line(20f,7f,20f,20f); line(20f,20f,8f,20f); line(8f,20f,8f,7f); line(4f,16f,4f,4f); line(4f,4f,16f,4f) }
            ChatIcon.SelectText -> { line(5f,4f,5f,20f); line(5f,4f,8f,4f); line(5f,20f,8f,20f); line(19f,4f,19f,20f); line(16f,4f,19f,4f); line(16f,20f,19f,20f); line(9f,8f,15f,8f); line(9f,12f,15f,12f); line(9f,16f,15f,16f) }
            ChatIcon.Disconnect -> { line(4f,4f,11f,4f); line(4f,4f,4f,20f); line(4f,20f,11f,20f); line(12f,12f,21f,12f); line(17f,8f,21f,12f); line(21f,12f,17f,16f) }
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
    onCreate: (String, String, String) -> Unit, onSend: (String, String, String, List<PromptReference>) -> Boolean, onAnswer: (JSONObject, JSONObject) -> Unit,
    onConnection: () -> Unit, onDisconnect: () -> Unit, onHome: (() -> Unit)? = null,
    hasOlderHistory: Boolean = false, olderHistoryLoading: Boolean = false, onOlderHistory: () -> Unit = {},
    handledPlans: Set<String> = emptySet(), planResponding: Boolean = false, onPlanAction: (ChatEntry, String) -> Unit = { _, _ -> },
    stopping: Boolean = false, onStop: () -> Unit = {},
    runningThreads: Set<String> = emptySet(),
    archived: Boolean = false, onArchived: (Boolean) -> Unit = {}, actionThreads: Set<String> = emptySet(),
    onSessionAction: (String, String, Boolean) -> Unit = { _, _, _ -> },
    takeoverSession: String = "", onTakeover: () -> Unit = {},
    models: List<ModelOption> = emptyList(), modelCatalogStatus: String = "",
    referenceCatalog: ReferenceCatalog = ReferenceCatalog(), referencesLoading: Boolean = false,
    referenceStatus: String = "", onRefreshReferences: () -> Unit = {},
    sessionEffort: String = "", hostModel: String = "", hostEffort: String = "", onModelChoice: (String, String) -> Unit = { _, _ -> },
    sessionStatus: SessionStatus? = null, sessionStatusLoading: Boolean = false, sessionStatusError: String = "", modeChanging: Boolean = false, modeUncertain: Boolean = false,
    onSessionStatus: () -> Unit = {}, onSessionMode: (String) -> Unit = {},
    permissionProfiles: List<SessionPermissionProfile> = emptyList(), permissionsWorking: Boolean = false,
    permissionsMessage: String = "", onSessionPermissions: (String) -> Unit = {},
    usageWarning: String = "",
    threadWorkspaces: Map<String,String> = emptyMap(), threadListLoading: Boolean = false,
    registeredProjects: List<RemoteProject> = emptyList(), projectMembership: Map<String,String> = emptyMap(), projectStatus: String = "",
    attachments: List<Attachment> = emptyList(), attachmentWorking: Boolean = false,
    attachmentProgress: String = "", attachmentError: String = "",
    loadAttachment: suspend (Attachment) -> java.io.File = { error("Reconnect to open attachment") },
    onNewProject: () -> Unit = {},
    onCreateInProject: (String, String, String, String, String) -> Unit = { model, mode, effort, _, _ -> onCreate(model, mode, effort) },
    onPhoneFiles: () -> Unit = {}, onRemoteFiles: () -> Unit = {}, onRemoveAttachment: (String) -> Unit = {},
    promptDrafts: PromptDrafts? = null, activeTurnId: String = "",
    agentConversation: AgentConversation? = null, onOpenAgent: (AgentTimeline) -> Unit = {},
    onAgentHistory: (Boolean) -> Unit = {},
) {
    var deleting by remember(host, archived) { mutableStateOf("") }
    if (deleting.isNotBlank()) AlertDialog(
        onDismissRequest = { deleting = "" }, title = { Text("Delete conversation?") },
        text = { Text("Permanently delete “${titles[deleting] ?: deleting}” and its history? This cannot be undone. Conversations required by a fork cannot be deleted.") },
        confirmButton = { TextButton(enabled = connected && deleting !in actionThreads, onClick = { onSessionAction("delete", deleting, true); deleting = "" }) { Text("Delete permanently", color = Mocha.Red) } },
        dismissButton = { TextButton(onClick = { deleting = "" }) { Text("Cancel") } }
    )
    fun sessionAction(action: String, id: String) {
        if (action == "delete") deleting = id else onSessionAction(action, id, false)
    }
    var showTakeover by remember(takeoverSession) { mutableStateOf(false) }
    if (showTakeover && takeoverSession.isNotBlank()) AlertDialog(
        onDismissRequest = { showTakeover = false },
        title = { Text("Take over on phone?") },
        text = { Text("Close the registered CLI for “${titles[takeoverSession] ?: takeoverSession}” on your computer. Running work can be interrupted; background commands may continue. Modex reopens the same session without resending prompts.") },
        confirmButton = { TextButton(onClick = { showTakeover = false; onTakeover() }) { Text("Close CLI and take over") } },
        dismissButton = { TextButton(onClick = { showTakeover = false }) { Text("Cancel") } }
    )
    var preview by remember(selected) { mutableStateOf<Attachment?>(null) }
    preview?.let { file -> AttachmentViewer(file, loadAttachment) { preview = null } }
    val drawer = rememberDrawerState(DrawerValue.Closed)
    val scope = rememberCoroutineScope()
    BackHandler(drawer.isOpen) { scope.launch { drawer.close() } }
    var collapsedProjects by rememberSaveable { mutableStateOf(arrayListOf<String>()) }
    var search by rememberSaveable { mutableStateOf("") }
    val localDrafts = remember(host) { mutableStateMapOf<String, PromptDraft>() }
    val focus = remember { FocusRequester() }
    val keyboard = LocalSoftwareKeyboardController.current
    val context = LocalContext.current
    fun openSessions() {
        keyboard?.hide()
        scope.launch { drawer.open() }
        if (connected) onRefresh()
    }
    var editing by remember(selected) { mutableStateOf<String?>(null) }
    val draft = (promptDrafts?.get(selected) ?: localDrafts[selected]) ?: PromptDraft()
    val prompt = draft.text
    val chosenReferences = draft.references
    fun updatePrompt(text: String, added: PromptReference? = null) {
        val current = (promptDrafts?.get(selected) ?: localDrafts[selected])?.references.orEmpty()
        val references = if (added == null) current else {
            current.filterNot { it.kind == added.kind && it.name == added.name } + added
        }
        val next = PromptDraft(text, referencesInText(text, references.take(8)))
        if (promptDrafts != null) promptDrafts.update(selected, next)
        else if (text.isEmpty()) localDrafts.remove(selected) else localDrafts[selected] = next
    }
    fun applyEdit(text: String) { updatePrompt(text); scope.launch { focus.requestFocus(); keyboard?.show() } }
    editing?.let { text -> AlertDialog(onDismissRequest = { editing = null }, title = { Text("Replace draft?") }, text = { Text("Your current draft will be replaced. The message will be sent as a new turn only when you press Send.") }, confirmButton = { TextButton(onClick = { applyEdit(text); editing = null }) { Text("Replace") } }, dismissButton = { TextButton(onClick = { editing = null }) { Text("Cancel") } }) }
    var newChat by rememberSaveable { mutableStateOf(false) }
    var newWorkspace by rememberSaveable { mutableStateOf("") }
    var newProjectId by rememberSaveable { mutableStateOf("") }
    var projectMenu by remember { mutableStateOf(false) }
    fun openNewChat(workspace: String = "", projectId: String = "") {
        newWorkspace = workspace; newProjectId = projectId; newChat = true
    }
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
    fun openModel() {
        modelInput = sessionModel; effortInput = sessionEffort.ifBlank { defaultEffort(sessionModel) }; showModel = true
    }
    var showSessionStatus by remember(selected) { mutableStateOf(false) }
    val reasoningTimeline = remember(messages) { thinkingTimeline(messages) }
    val timeline = remember(reasoningTimeline) { reasoningTimeline.filterNot { it.role == "reasoning" || it.role == "agent_activity" } }
    val agents = remember(messages, activeTurnId, connected, historyLoading) {
        agentTimeline(messages, activeTurnId, connected && !historyLoading)
    }
    var showAgentActivity by remember(host, selected) { mutableStateOf(false) }
    val openedAgent = agentConversation?.agent
    if (openedAgent != null) {
        AgentConversationScreen(agentConversation, agents.firstOrNull { it.id == openedAgent.id } ?: openedAgent,
            onAgentHistory, loadAttachment)
    } else if (showAgentActivity && agents.isNotEmpty()) {
        AgentActivitySheet(agents, hasOlderHistory, olderHistoryLoading, onOlderHistory,
            onOpenAgent.takeIf { agentConversation != null }) { showAgentActivity = false }
    }
    val latestPromptIndex = remember(reasoningTimeline) { reasoningTimeline.indexOfLast { it.role == "You" } }
    val latestPrompt = reasoningTimeline.getOrNull(latestPromptIndex)
    val showWorkStatus = !historyLoading && selected.isNotBlank()
    val completedWork = latestPrompt?.takeIf { showWorkStatus && !busy && it.status != "inProgress" }
    val selectedDecisions = decisions.filter { selected.isNotBlank() && it.optString("thread_id") == selected }
    LaunchedEffect(selected) { if (selected.isBlank()) { showSettings = false; showModel = false } }
    var menu by remember { mutableStateOf(false) }
    val canSwitch = connected && !historyLoading
    val modeActionEnabled = canSwitch && selected.isNotBlank() && selectedDecisions.isEmpty() && !modeChanging && !sessionStatusLoading && (!busy || modeUncertain)
    fun openStatus() { if (selected.isNotBlank()) { showSessionStatus = true; onSessionStatus() } }
    val status = when { connecting -> "Connecting…"; !connected -> "Disconnected"; historyLoading -> "Loading history…"; busy -> "Working…"; else -> "Connected · SSH" }
    ModalNavigationDrawer(
        drawerState = drawer,
        drawerContent = {
            ModalDrawerSheet(drawerContainerColor = Mocha.Mantle, drawerShape = RoundedCornerShape(0.dp), modifier = Modifier.widthIn(max = 320.dp).fillMaxHeight()) {
                Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 8.dp, top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("modex", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                    IconButton(onClick = { scope.launch { drawer.close() } }) { ChatGlyph(ChatIcon.Close,"Close sessions") }
                }
                FilledTonalButton(onClick = { openNewChat() }, enabled = canSwitch, shape = RoundedCornerShape(10.dp), colors = ButtonDefaults.filledTonalButtonColors(containerColor = Mocha.Blue, contentColor = Mocha.Base), modifier = Modifier.fillMaxWidth().padding(start = 16.dp, top = 12.dp, end = 16.dp, bottom = 6.dp), contentPadding = PaddingValues(14.dp)) {
                    ChatGlyph(ChatIcon.New); Spacer(Modifier.width(12.dp)); Text("New chat"); Spacer(Modifier.weight(1f))
                }
                TextButton(onClick = { scope.launch { drawer.close(); onNewProject() } }, enabled = canSwitch && !attachmentWorking, shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 16.dp, bottom = 6.dp), contentPadding = PaddingValues(14.dp)) {
                    ChatGlyph(ChatIcon.Folder); Spacer(Modifier.width(12.dp)); Text("New project"); Spacer(Modifier.weight(1f))
                }
                OutlinedTextField(search, { search = it }, placeholder = { Text("Search sessions or projects") }, singleLine = true, modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp), shape = RoundedCornerShape(10.dp))
                Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 8.dp, top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(if (archived) "Archived" else "Sessions", style = MaterialTheme.typography.labelLarge, color = Mocha.Subtext, modifier = Modifier.weight(1f))
                    TextButton(onClick = onRefresh, enabled = connected && !threadListLoading) { Text("Refresh") }
                }
                TextButton(onClick = { search = ""; onArchived(!archived) }, enabled = connected, modifier = Modifier.padding(horizontal = 12.dp)) { Text(if (archived) "Back to sessions" else "Archived conversations") }
                if (error.isNotBlank()) Text(error, Modifier.padding(horizontal = 20.dp, vertical = 8.dp), color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
                val visible = threads.filter { (titles[it] ?: it).contains(search, ignoreCase = true) || it.contains(search,ignoreCase = true) || threadWorkspaces[it].orEmpty().contains(search, ignoreCase = true) }
                val projects = projectSessions(registeredProjects, visible, projectMembership, threadWorkspaces)
                LazyColumn(Modifier.weight(1f), contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    if (visible.isEmpty()) item { Text(if (threadListLoading) "Loading sessions…" else if (search.isBlank()) "No conversations yet" else "No matching conversations", Modifier.padding(12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium) }
                    if (!archived) item { Text("Projects", Modifier.padding(horizontal = 12.dp, vertical = 10.dp), color = Mocha.Subtext, style = MaterialTheme.typography.labelLarge) }
                    if (!archived && projectStatus.isNotEmpty()) item { Text(projectStatus, Modifier.padding(12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall) }
                    (if (archived) emptyList() else registeredProjects).filter { search.isBlank() || projects[it.id].orEmpty().isNotEmpty() || it.name.contains(search, true) || it.roots.any { root -> root.contains(search,true) } }.forEach { project ->
                        val path = project.id
                        val ids = projects[path].orEmpty()
                        val expanded = path !in collapsedProjects || search.isNotEmpty()
                        item(key = "folder:$path") {
                            Surface(onClick = { collapsedProjects = ArrayList(collapsedProjects).apply { if (path in this) remove(path) else add(path) } }, color = Mocha.Mantle, shape = RoundedCornerShape(6.dp), modifier = Modifier.semantics {
                                contentDescription = "${if (expanded) "Collapse" else "Expand"} project ${project.name}"
                                stateDescription = if (expanded) "expanded" else "collapsed"
                            }) {
                                Row(Modifier.fillMaxWidth().heightIn(min = 48.dp).padding(horizontal = 12.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                                    ChatGlyph(ChatIcon.Folder)
                                    Column(Modifier.weight(1f).padding(start = 8.dp)) {
                                        Text(project.name.ifBlank { projectName(project.roots.firstOrNull().orEmpty()) }, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                        if (project.roots.isNotEmpty()) Text(project.roots.joinToString(" · "), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    }
                                    IconButton(onClick = { openNewChat(project.roots.firstOrNull().orEmpty(), project.id) }, enabled = canSwitch && project.roots.isNotEmpty()) { ChatGlyph(ChatIcon.New, "New chat in ${project.name}", color = Mocha.Subtext) }
                                    ChatGlyph(if (expanded) ChatIcon.ChevronDown else ChatIcon.ChevronRight, color = Mocha.Subtext)
                                }
                            }
                        }
                        if (path !in collapsedProjects || search.isNotEmpty()) items(ids, key = { "project:$it" }) { id ->
                            SessionSidebarRow(id, titles[id], id == selected, canSwitch, id in runningThreads, Modifier.padding(start = 20.dp), archived, id in actionThreads, decisions.none { it.optString("thread_id") == id }, ::sessionAction) { onSelect(id); scope.launch { drawer.close() } }
                        }
                    }
                    if (visible.isNotEmpty()) item { Text(if (archived) "Archived conversations" else "Recents", Modifier.padding(horizontal = 12.dp, vertical = 12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.labelLarge) }
                    items(visible, key = { "recent:$it" }) { id ->
                        SessionSidebarRow(id, titles[id], id == selected, canSwitch, id in runningThreads, archived = archived, working = id in actionThreads, canFork = decisions.none { it.optString("thread_id") == id }, onAction = ::sessionAction) { onSelect(id); scope.launch { drawer.close() } }
                    }
                    if (threadListLoading) item { Text("Loading sessions…", Modifier.padding(12.dp), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) }
                    if (hasMore && !threadListLoading) item { TextButton(onClick = onMore, enabled = connected) { Text("Load more") } }
                }
                HorizontalDivider(color = Mocha.Surface)
                if (onHome != null) TextButton(onClick = onHome, modifier = Modifier.fillMaxWidth()) { Text("All hosts") }
                TextButton(onClick = { scope.launch { drawer.close() }; onConnection() }, modifier = Modifier.fillMaxWidth().padding(8.dp)) {
                    Column(Modifier.fillMaxWidth().padding(4.dp)) { Text(host.ifBlank { "Connection settings" }, maxLines = 1, overflow = TextOverflow.Ellipsis); Text(status, style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) }
                }
            }
        }
    ) {
        Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding()) {
            val scroll = rememberLazyListState()
            var openAtLatest by remember(selected) { mutableStateOf(true) }
            var followLatest by remember(selected) { mutableStateOf(true) }
            var autoScrolling by remember(selected) { mutableStateOf(false) }
            Row(Modifier.fillMaxWidth().heightIn(min = 64.dp).padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = ::openSessions) { ChatGlyph(ChatIcon.Menu,"Open sessions") }
                Column(Modifier.weight(1f).padding(horizontal = 8.dp).clickable(enabled = selected.isNotBlank(), onClick = ::openStatus).semantics { if (selected.isNotBlank()) contentDescription = "Open session status" }) {
                    Text(if (selected.isBlank()) "modex" else titles[selected] ?: "Conversation", style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, fontWeight = FontWeight.SemiBold)
                    Text(if (onHome != null) "$host · $status" else status, maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.labelSmall, color = if (connected) Mocha.Blue else Mocha.Subtext)
                    if (selected.isNotBlank()) Text(contextSummary(sessionStatus), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.labelSmall, color = if (sessionStatus?.contextTokens != null) Mocha.Text else Mocha.Subtext)
                }
                Box {
                    IconButton(onClick = { menu = true }) { ChatGlyph(ChatIcon.More,"Connection menu") }
                    ModexDropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(text = { Text("Session settings") }, leadingIcon = { ChatGlyph(ChatIcon.Settings) }, enabled = selected.isNotBlank(), onClick = { menu = false; showSettings = true; onSessionPermissions("") })
                        if (onHome != null) DropdownMenuItem(text = { Text("All hosts") }, leadingIcon = { ChatGlyph(ChatIcon.Computer) }, onClick = { menu = false; onHome() })
                        DropdownMenuItem(text = { Text("Connection settings") }, leadingIcon = { ChatGlyph(ChatIcon.Settings) }, onClick = { menu = false; onConnection() })
                        if (connected || connecting) DropdownMenuItem(text = { Text("Disconnect") }, leadingIcon = { ChatGlyph(ChatIcon.Disconnect) }, onClick = { menu = false; onDisconnect() })
                    }
                }
            }
            if (selected.isNotBlank() && !historyLoading && !openAtLatest && !followLatest && scroll.canScrollForward) {
                val previousPrompt by remember(timeline, scroll) {
                    derivedStateOf {
                        val first = scroll.firstVisibleItemIndex.coerceAtMost(timeline.size)
                        if (timeline.getOrNull(first)?.role == "You") null
                        else (first - 1 downTo 0).firstOrNull { timeline[it].role == "You" }
                    }
                }
                previousPrompt?.let { index ->
                    val entry = timeline[index]
                    val previewText = entry.text.lineSequence().firstOrNull { it.isNotBlank() }?.trim()
                        ?: entry.attachments.firstOrNull()?.name ?: "Previous prompt"
                    Surface(
                        color = Mocha.Mantle,
                        shape = RoundedCornerShape(10.dp),
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
                    ) {
                        Row(
                            Modifier.fillMaxWidth().heightIn(min = 48.dp)
                                .semantics { contentDescription = "Jump to previous prompt" }
                                .clickable(role = Role.Button) { scroll.requestScrollToItem(index) }
                                .padding(horizontal = 12.dp, vertical = 6.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Box(Modifier.rotate(180f)) { ChatGlyph(ChatIcon.Down, color = Mocha.Blue) }
                            Text(previewText, maxLines = 1, overflow = TextOverflow.Ellipsis,
                                style = MaterialTheme.typography.bodySmall, color = Mocha.Text)
                        }
                    }
                }
            }
            if (error.isNotBlank()) Surface(color = MaterialTheme.colorScheme.errorContainer, shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)) {
                Column(Modifier.padding(12.dp)) {
                    SelectionContainer { Text(error, style = MaterialTheme.typography.bodySmall) }
                    if (takeoverSession.isNotBlank()) TextButton(onClick = { showTakeover = true }, enabled = connected && !historyLoading) { Text("Take over on phone") }
                }
            }
            if (usageWarning.isNotBlank() && selected.isNotBlank()) {
                Surface(color = Mocha.Mantle, shape = RoundedCornerShape(10.dp),
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).clickable(onClick = ::openStatus)) {
                    Text(usageWarning, Modifier.padding(12.dp), color = Mocha.Red, style = MaterialTheme.typography.bodyMedium)
                }
            }
            if (historyLoading || connecting) LinearProgressIndicator(Modifier.fillMaxWidth(), color = Mocha.Blue, trackColor = Mocha.Surface)
            var requestingOlder by remember(selected) { mutableStateOf(false) }
            fun loadEarlier() {
                if (!connected || !hasOlderHistory || historyLoading || olderHistoryLoading || requestingOlder) return
                requestingOlder = true
                onOlderHistory()
            }
            LaunchedEffect(selected, connected, hasOlderHistory, historyLoading, olderHistoryLoading, error, timeline.size) {
                if (connected && hasOlderHistory && !historyLoading && !olderHistoryLoading && error.isBlank()) {
                    snapshotFlow {
                        scroll.layoutInfo.totalItemsCount > 0 && scroll.firstVisibleItemIndex <= 3 &&
                            (!openAtLatest || timeline.isEmpty()) && !requestingOlder
                    }.collect { nearTop -> if (nearTop) loadEarlier() }
                }
            }
            LaunchedEffect(olderHistoryLoading, timeline.size, error, hasOlderHistory) {
                if (!olderHistoryLoading) {
                    withFrameNanos { }
                    requestingOlder = false
                }
            }
            LaunchedEffect(selected) {
                snapshotFlow {
                    if (autoScrolling) null else scroll.isScrollInProgress to scroll.canScrollForward
                }.collect { gesture ->
                    if (gesture?.first == true) followLatest = !gesture.second
                }
            }
            LaunchedEffect(selected, historyLoading, olderHistoryLoading, requestingOlder) {
                if (historyLoading) {
                    openAtLatest = true
                    followLatest = true
                    return@LaunchedEffect
                }
                if (olderHistoryLoading || requestingOlder) return@LaunchedEffect
                // Observe rendered growth too: streaming text keeps expanding after its data arrives.
                snapshotFlow { Triple(scroll.layoutInfo, followLatest, scroll.isScrollInProgress) }
                    // Forced remeasure replaces layoutInfo even when a collapsed viewport cannot scroll.
                    .distinctUntilChangedBy { (layout, following, scrolling) ->
                        Triple(layout.totalItemsCount to layout.viewportSize,
                            layout.visibleItemsInfo.map { Triple(it.index, it.offset, it.size) },
                            following to scrolling)
                    }
                    .collect { (layout, following, scrolling) ->
                        if ((openAtLatest || following) && !scrolling && layout.totalItemsCount > 0 &&
                            (openAtLatest || scroll.canScrollForward)) {
                            autoScrolling = true
                            try {
                                scroll.scrollToItem(layout.totalItemsCount - 1)
                                openAtLatest = false
                            } finally {
                                autoScrolling = false
                            }
                        }
                    }
            }
            if (timeline.isEmpty() && selectedDecisions.isEmpty() && !hasOlderHistory) {
                Column(Modifier.weight(1f).fillMaxWidth().padding(32.dp), verticalArrangement = Arrangement.Center) {
                    Text("Your workspace.\nWithin reach.", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Medium)
                    Text(if (selected.isBlank()) "Open a conversation from the sidebar, or start a new chat." else "Ask Codex to explore, build or fix something.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.padding(top = 16.dp))
                    if (selected.isBlank()) Button(onClick = { openNewChat() }, enabled = canSwitch, shape = RoundedCornerShape(10.dp), colors = ButtonDefaults.buttonColors(containerColor = Mocha.Blue, contentColor = Mocha.Base), modifier = Modifier.padding(top = 12.dp)) { Text("New chat") }
                }
            } else Box(Modifier.weight(1f).fillMaxWidth()) {
                LazyColumn(state = scroll, modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(horizontal = 20.dp, vertical = 20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                itemsIndexed(timeline, key = { index, entry -> if (entry.id.isBlank()) "legacy-$index" else "${entry.turnId}/${entry.role}/${entry.id}" }) { _, entry ->
                    ChatTranscriptEntry(entry, showWork = entry !== completedWork, loadAttachment, { preview = it },
                        onEdit = if (entry.role == "You") ({
                            if (prompt.isNotBlank() && prompt != entry.text) editing = entry.text else applyEdit(entry.text)
                        }) else null)
                }
                items(selectedDecisions, key = { "decision-${it.optString("decision_id")}" }) { decision ->
                    InlineDecisionCard(decision, decision.optString("decision_id") in answeringDecisionIds, connected) { onAnswer(decision, it) }
                }
                pendingPlan(messages, busy, handledPlans)?.let { plan ->
                    item(key = "plan-review-${planKey(plan)}") { PlanReviewCard(connected && !historyLoading, planResponding) { onPlanAction(plan, it) } }
                }
                item(key = "history-bottom") { Spacer(Modifier.height(1.dp)) }
                }
                if (!historyLoading && !openAtLatest && scroll.canScrollForward) OutlinedIconButton(
                    onClick = { scope.launch {
                        autoScrolling = true
                        try {
                            scroll.animateScrollToItem((scroll.layoutInfo.totalItemsCount - 1).coerceAtLeast(0))
                            followLatest = true
                        } finally { autoScrolling = false }
                    } },
                    modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 8.dp).size(48.dp),
                    shape = RoundedCornerShape(12.dp), border = BorderStroke(1.dp, Mocha.Overlay),
                    colors = IconButtonDefaults.outlinedIconButtonColors(containerColor = Mocha.Mantle, contentColor = Mocha.Text)
                ) { ChatGlyph(ChatIcon.Down, "Scroll to latest") }
                if (hasOlderHistory && (olderHistoryLoading || error.isNotBlank())) Surface(modifier = Modifier.align(Alignment.TopCenter), color = Mocha.Base, shape = RoundedCornerShape(8.dp)) {
                    if (olderHistoryLoading) Row(Modifier.padding(8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        CircularProgressIndicator(Modifier.size(12.dp), strokeWidth = 1.5.dp, color = Mocha.Subtext)
                        Text("Loading earlier messages…", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                    } else TextButton(enabled = connected && !historyLoading, onClick = ::loadEarlier) {
                        Text("Retry loading earlier messages")
                    }
                }
            }
            val running = busy && showWorkStatus
            val activeWork = latestPrompt?.takeIf { running && it.status == "inProgress" }
            val workAction = if (!running) null else if (stopping) "Stopping" else {
                val reasoning = activeWork?.let { reasoningTimeline.drop(latestPromptIndex + 1).lastOrNull { it.role == "reasoning" } }
                reasoning?.let { reasoningLabel(it.summaryParts.lastOrNull { part -> part.isNotBlank() }.orEmpty().ifBlank { it.text }) }
                    .orEmpty().ifBlank { "Working" }
            }
            val work = if (running) activeWork ?: ChatEntry(role = "You", status = "inProgress") else completedWork
            PromptComposer(
                prompt, ::updatePrompt, busy,
                (sessionModel.ifBlank { "Loading model…" } + " · " + sessionEffort.ifBlank { defaultEffort(sessionModel).ifBlank { "Reasoning unavailable" } }), connected && selected.isNotBlank(),
                ::openModel,
                connected && !historyLoading && selected.isNotBlank() && !busy && !attachmentWorking && !modeChanging,
                attachments, attachmentWorking, attachmentProgress, attachmentError,
                connected && selected.isNotBlank() && !busy && !attachmentWorking,
                onPhoneFiles, onRemoteFiles, onRemoveAttachment, loadAttachment, { preview = it },
                modelColor = reasoningColor(sessionEffort.ifBlank { defaultEffort(sessionModel) }),
                focus = focus, stopping = stopping, stopEnabled = connected && !historyLoading && busy, onStop = onStop,
                draftKey = selected, referenceCatalog = referenceCatalog, referencesLoading = referencesLoading, referenceStatus = referenceStatus,
                referenceEnabled = connected && selected.isNotBlank() && !historyLoading, chosenReferences = chosenReferences,
                onRemoveReference = { updatePrompt(removeReferenceMarkers(prompt, it)) }, onRefreshReferences = onRefreshReferences,
                suggestionsEnabled = !drawer.isOpen,
                mode = sessionStatus?.mode.orEmpty(), modeChanging = modeChanging, modeUncertain = modeUncertain, modeEnabled = modeActionEnabled,
                onMode = onSessionMode, onModeRefresh = onSessionStatus, work = work, workAction = workAction,
                agentCount = agents.takeIf { it.isNotEmpty() }?.count { it.status == "running" },
                onAgentActivity = { keyboard?.hide(); showAgentActivity = true },
            ) {
                if (onSend(prompt, sessionModel, sessionEffort.ifBlank { defaultEffort(sessionModel) }, chosenReferences)) updatePrompt("")
            }
        }
    }
    if (showSessionStatus) SessionStatusDialog(sessionStatus, sessionStatusLoading, sessionStatusError, onSessionStatus) { showSessionStatus = false }
    if (newChat) AlertDialog(
        onDismissRequest = { newChat = false }, title = { Text("New conversation") },
        text = { Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text("Uses your host’s provider.", color = Mocha.Subtext)
            Box {
                OutlinedButton(onClick = { projectMenu = true }, modifier = Modifier.fillMaxWidth(), shape = RoundedCornerShape(10.dp)) {
                    ChatGlyph(ChatIcon.Folder)
                    Text(registeredProjects.firstOrNull { it.id == newProjectId }?.name ?: newWorkspace.ifBlank { "Outside projects" }, Modifier.weight(1f).padding(horizontal = 8.dp), maxLines = 1, overflow = TextOverflow.Ellipsis)
                    ChatGlyph(ChatIcon.ChevronDown)
                }
                ModexDropdownMenu(expanded = projectMenu, onDismissRequest = { projectMenu = false }) {
                    DropdownMenuItem(text = { Text("Outside projects") }, leadingIcon = { ChatGlyph(ChatIcon.Folder) }, onClick = { newWorkspace = ""; newProjectId = ""; projectMenu = false })
                    registeredProjects.forEach { project -> project.roots.forEach { root ->
                        DropdownMenuItem(text = { Column { Text(project.name); Text(root, style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) } }, leadingIcon = { ChatGlyph(ChatIcon.Folder) }, onClick = { newWorkspace = root; newProjectId = project.id; projectMenu = false })
                    } }
                }
            }
            Text(newWorkspace.ifBlank { "Outside projects" }, style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
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
        confirmButton = { TextButton(enabled = canSwitch && model.isNotBlank() && validModelId(model), onClick = { onCreateInProject(model.trim(), permissionMode, newEffort, newWorkspace, newProjectId); newChat = false; scope.launch { drawer.close() } }) { Text("Create chat") } },
        dismissButton = { TextButton(onClick = { newChat = false }) { Text("Cancel") } }
    )
    if (showSettings) SessionPermissionsDialog(
        status = sessionStatus,
        profiles = permissionProfiles,
        working = permissionsWorking,
        message = permissionsMessage,
        canApply = connected && !busy && !historyLoading && !modeChanging && selectedDecisions.isEmpty() && !planResponding,
        onRequest = onSessionPermissions,
        onClose = { showSettings = false },
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
private fun SessionStatusDialog(status: SessionStatus?, loading: Boolean, error: String, refresh: () -> Unit, close: () -> Unit) {
    fun known(value: String) = value.ifBlank { "Unavailable" }
    fun number(value: Long?) = value?.let(java.text.NumberFormat.getIntegerInstance()::format) ?: "Unavailable"
    fun sandbox(value: String) = when (value) {
        "readOnly" -> "Read-only"
        "workspaceWrite" -> "Workspace write"
        "dangerFullAccess" -> "Full access"
        "externalSandbox" -> "External sandbox"
        else -> known(value)
    }
    AlertDialog(
        onDismissRequest = close, title = { Row(verticalAlignment = Alignment.CenterVertically) { ChatGlyph(ChatIcon.Activity); Spacer(Modifier.width(8.dp)); Text("Session status") } },
        text = { Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (loading) Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(20.dp).semantics { contentDescription = "Refreshing session status" }, strokeWidth = 2.dp)
                Spacer(Modifier.width(8.dp)); Text("Refreshing…", color = Mocha.Subtext)
            }
            if (error.isNotBlank()) Text(error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            if (status == null) Text("No status has been reported for this session.", color = Mocha.Subtext)
            else SelectionContainer { Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                val context = contextRemaining(status)
                SessionUsageRow(
                    "Context window", context,
                    if (context != null) "${number(status.contextTokens)} used / ${number(status.contextWindow)} tokens" else null,
                    showBar = false,
                )
                SessionRateLimitRow("5h limit", status.usageWindow(5 * 60L))
                SessionRateLimitRow("Weekly limit", status.usageWindow(7 * 24 * 60L))
                HorizontalDivider(Modifier.padding(vertical = 4.dp), color = Mocha.Overlay)
                Text("Mode: ${if (status.mode == "plan") "Plan" else if (status.mode == "default") "Default" else "Unavailable"}")
                Text("Model: ${known(status.model)}")
                Text("Reasoning: ${known(status.effort)}")
                Text("Workspace: ${known(status.workspace)}")
                Text("Provider: ${known(status.provider)}")
                Text("Sandbox: ${sandbox(status.sandbox)}")
                Text("Approvals: ${known(status.approvalPolicy)}")
                Text("Reviewer: ${known(status.approvalsReviewer)}")
                Text("Session total: ${if (status.totalTokens != null) "${number(status.totalTokens)} tokens" else "Unavailable"}")
                Text("Limits: ${listOf(status.limitName, status.planType).filter { it.isNotBlank() }.joinToString(" · ").ifBlank { "Unavailable" }}")
            } }
        } },
        confirmButton = { TextButton(onClick = close) { Text("Close") } },
        dismissButton = { TextButton(enabled = !loading, onClick = refresh) { Text("Refresh") } }
    )
}

@Composable
private fun SessionRateLimitRow(label: String, window: RateLimitWindow?) {
    val reset = window?.resetsAt?.takeIf { it in 1..Long.MAX_VALUE / 1000 }?.let {
        java.text.DateFormat.getDateTimeInstance(java.text.DateFormat.SHORT, java.text.DateFormat.SHORT)
            .format(java.util.Date(it * 1000))
    }
    SessionUsageRow(label, window?.remainingPercent(), reset?.let { "Resets $it" }, showBar = true)
}

@Composable
private fun SessionUsageRow(label: String, remaining: Int?, detail: String?, showBar: Boolean) {
    Column(Modifier.fillMaxWidth().padding(bottom = 4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(label, Modifier.weight(1f), style = MaterialTheme.typography.labelLarge)
            Text(
                remaining?.let { "$it% left" } ?: "Unavailable",
                color = if (remaining == null) Mocha.Subtext else Mocha.Blue,
                style = MaterialTheme.typography.labelLarge,
            )
        }
        if (remaining != null && showBar) {
            LinearProgressIndicator(
                progress = { remaining / 100f }, modifier = Modifier.fillMaxWidth(),
                color = Mocha.Blue, trackColor = Mocha.Surface, gapSize = 0.dp, drawStopIndicator = {},
            )
        }
        detail?.let { Text(it, color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall) }
    }
}

@Composable
private fun PromptComposer(
    prompt: String, onChange: (String, PromptReference?) -> Unit, busy: Boolean, model: String, modelEnabled: Boolean, onModel: () -> Unit, enabled: Boolean,
    attachments: List<Attachment>, attachmentWorking: Boolean, attachmentProgress: String, attachmentError: String,
    attachEnabled: Boolean, onPhoneFiles: () -> Unit, onRemoteFiles: () -> Unit, onRemove: (String) -> Unit, loadAttachment: suspend (Attachment) -> java.io.File, onPreview: (Attachment) -> Unit,
    modelColor: Color, focus: FocusRequester, stopping: Boolean, stopEnabled: Boolean, onStop: () -> Unit,
    draftKey: String, referenceCatalog: ReferenceCatalog, referencesLoading: Boolean, referenceStatus: String,
    referenceEnabled: Boolean, chosenReferences: List<PromptReference>,
    onRemoveReference: (PromptReference) -> Unit, onRefreshReferences: () -> Unit,
    suggestionsEnabled: Boolean, mode: String, modeChanging: Boolean, modeUncertain: Boolean, modeEnabled: Boolean,
    onMode: (String) -> Unit, onModeRefresh: () -> Unit, work: ChatEntry?, workAction: String?,
    agentCount: Int?, onAgentActivity: () -> Unit, onSend: () -> Unit
) {
    var attachMenu by remember { mutableStateOf(false) }
    var editor by remember(draftKey) { mutableStateOf(TextFieldValue(prompt, TextRange(prompt.length))) }
    var fieldFocused by remember { mutableStateOf(false) }
    var dismissedQuery by remember(draftKey) { mutableStateOf<Pair<String, Int>?>(null) }
    var manualReferences by remember(draftKey) { mutableStateOf(false) }
    LaunchedEffect(prompt, draftKey) { if (editor.text != prompt) editor = TextFieldValue(prompt, TextRange(prompt.length)) }
    val query = referenceQuery(editor.text, editor.selection.start, editor.selection.end)
    val showReferences = suggestionsEnabled && referenceEnabled && (manualReferences || (fieldFocused && query != null && dismissedQuery != (editor.text to editor.selection.start)))
    fun closeReferences() { manualReferences = false; dismissedQuery = editor.text to editor.selection.start }
    BackHandler(showReferences) { closeReferences() }
    fun changeEditor(value: TextFieldValue, reference: PromptReference? = null) {
        if (value.text != editor.text || value.selection != editor.selection) dismissedQuery = null
        editor = value
        onChange(value.text, reference)
    }
    val sendEnabled = enabled && (prompt.isNotBlank() || attachments.isNotEmpty())
    val keyboardVisible = WindowInsets.ime.getBottom(LocalDensity.current) > 0
    val control = modeControl(mode, modeUncertain, modeChanging)
    val modeColor = if (mode == "plan") Mocha.Pink else Mocha.Blue
    Column(Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, bottom = 12.dp)) {
        Row(Modifier.fillMaxWidth().padding(end = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            val modeAvailable = modeEnabled && control.enabled
            Row(
                Modifier.sizeIn(minWidth = 48.dp, minHeight = 48.dp)
                    .clickable(
                        interactionSource = null, indication = null, enabled = modeAvailable, role = Role.Button,
                        onClick = { if (control.refresh) onModeRefresh() else onMode(control.target) },
                    )
                    .padding(horizontal = 8.dp)
                    .semantics { stateDescription = if (mode == "plan") "Plan mode enabled" else if (mode == "default") "Default mode enabled" else "Mode unavailable" },
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                if (modeChanging) CircularProgressIndicator(Modifier.size(12.dp), strokeWidth = 1.5.dp, color = Mocha.Subtext)
                Text(control.label, style = MaterialTheme.typography.labelSmall,
                    color = if (!modeAvailable) Mocha.Subtext else if (control.refresh) Mocha.Red else modeColor)
            }
            if (work != null) Box(Modifier.weight(1f).padding(start = 8.dp), contentAlignment = Alignment.CenterEnd) {
                WorkTimer(work, workAction)
            } else if (agentCount != null) {
                Spacer(Modifier.weight(1f))
            }
            if (agentCount != null) {
                Spacer(Modifier.width(8.dp))
                AgentActivityButton(agentCount, onAgentActivity)
            }
        }
        Surface(shape = RoundedCornerShape(12.dp), color = Mocha.Mantle, border = BorderStroke(1.dp, Mocha.Surface), modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Message input container" }) {
        Column(Modifier.padding(12.dp)) {
            if (showReferences) Box(Modifier.weight(1f, fill = false)) {
                ReferencePicker(referenceCatalog, query?.query.orEmpty(), referencesLoading, referenceStatus, chosenReferences, onRefreshReferences,
                onDismiss = ::closeReferences,
                onSelect = { reference ->
                    val range = query ?: ReferenceQuery(editor.selection.min, editor.selection.max, "")
                    val (text, cursor) = insertReference(editor.text, range, reference)
                    manualReferences = false
                    changeEditor(TextFieldValue(text, TextRange(cursor)), reference)
                    focus.requestFocus()
                })
            }
            if (chosenReferences.isNotEmpty()) Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                chosenReferences.forEach { reference ->
                    InputChip(selected = true, colors = InputChipDefaults.inputChipColors(selectedContainerColor = Mocha.Surface, selectedLabelColor = Mocha.Blue, selectedLeadingIconColor = Mocha.Blue, selectedTrailingIconColor = Mocha.Subtext), shape = RoundedCornerShape(6.dp), onClick = { onRemoveReference(reference) }, label = { Text(reference.marker, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        leadingIcon = { ChatGlyph(if (reference.kind == "plugin") ChatIcon.Plugin else ChatIcon.Skill) }, trailingIcon = { ChatGlyph(ChatIcon.Close) },
                        modifier = Modifier.heightIn(min = 48.dp).widthIn(max = 260.dp).semantics { contentDescription = "Remove ${reference.kind} ${reference.name}" })
                }
            }
            if (attachments.isNotEmpty() && !showReferences) Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                attachments.forEach { file ->
                    if (keyboardVisible) InputChip(
                        selected = false, onClick = { onPreview(file) },
                        label = { Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        leadingIcon = { ChatGlyph(if (file.kind == "image") ChatIcon.Image else ChatIcon.File) },
                        trailingIcon = { if (!busy && !attachmentWorking) IconButton(onClick = { onRemove(file.path) }) { ChatGlyph(ChatIcon.Close, "Remove ${file.name}") } },
                        modifier = Modifier.heightIn(min = 48.dp).widthIn(max = 260.dp), shape = RoundedCornerShape(6.dp)
                    ) else AttachmentTile(file, loadAttachment, { onPreview(file) }, if (!busy && !attachmentWorking) ({ onRemove(file.path) }) else null)
                }
            }
            if (attachmentWorking) Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp)
                Text(attachmentProgress, Modifier.padding(start = 8.dp), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            if (attachmentError.isNotEmpty()) Text(attachmentError, style = MaterialTheme.typography.bodySmall, color = Mocha.Red)
            BasicTextField(editor, { changeEditor(it) }, textStyle = MaterialTheme.typography.bodyLarge.copy(color = Mocha.Text), cursorBrush = SolidColor(Mocha.Blue), maxLines = 6,
                modifier = Modifier.fillMaxWidth().focusRequester(focus).onFocusChanged { fieldFocused = it.isFocused }.heightIn(min = 48.dp).semantics { contentDescription = "Message Codex" },
                decorationBox = { inner -> Box(Modifier.padding(vertical = 8.dp)) { if (prompt.isEmpty()) Text("Message Codex", color = Mocha.Subtext, style = MaterialTheme.typography.bodyLarge); inner() } })
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Box {
                    IconButton(onClick = { attachMenu = true }, enabled = attachEnabled) { ChatGlyph(ChatIcon.Attach, "Attach files") }
                    ModexDropdownMenu(expanded = attachMenu, onDismissRequest = { attachMenu = false }) {
                        DropdownMenuItem(text = { Text("From this phone") }, leadingIcon = { ChatGlyph(ChatIcon.Phone) }, enabled = attachEnabled, onClick = { attachMenu = false; onPhoneFiles() })
                        DropdownMenuItem(text = { Text("From connected computer") }, leadingIcon = { ChatGlyph(ChatIcon.Computer) }, enabled = attachEnabled, onClick = { attachMenu = false; onRemoteFiles() })
                        DropdownMenuItem(text = { Text("Skills & plugins") }, leadingIcon = { ChatGlyph(ChatIcon.Skill) }, enabled = referenceEnabled, onClick = {
                            attachMenu = false
                            val start = editor.selection.min; val end = editor.selection.max
                            val marker = (if (start > 0 && !editor.text[start - 1].isWhitespace()) " " else "") + "@"
                            changeEditor(TextFieldValue(editor.text.replaceRange(start, end, marker), TextRange(start + marker.length)))
                            focus.requestFocus()
                        })
                    }
                }
                Box(Modifier.weight(1f)) {
                    TextButton(
                        onClick = onModel, enabled = modelEnabled, shape = RoundedCornerShape(6.dp),
                        colors = ButtonDefaults.textButtonColors(contentColor = modelColor),
                        contentPadding = PaddingValues(horizontal = 4.dp), modifier = Modifier.heightIn(min = 48.dp).semantics { contentDescription = "Choose model" }
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.Start) {
                            ChatGlyph(ChatIcon.Model)
                            Spacer(Modifier.width(6.dp))
                            Text(model, modifier = Modifier.weight(1f, fill = false), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.labelMedium)
                        }
                    }
                }
                FilledIconButton(onClick = if (busy) onStop else onSend, enabled = if (busy) stopEnabled && !stopping else sendEnabled, shape = RoundedCornerShape(10.dp), modifier = Modifier.size(48.dp),
                    colors = IconButtonDefaults.filledIconButtonColors(containerColor = if (busy) Mocha.Red else modeColor, contentColor = Mocha.Base,
                        disabledContainerColor = if (busy) Mocha.Red else Mocha.Surface, disabledContentColor = Mocha.Base)) {
                    if (stopping) CircularProgressIndicator(Modifier.size(20.dp).semantics { contentDescription = "Stopping" }, strokeWidth = 2.dp, color = Mocha.Base) else if (busy) ChatGlyph(ChatIcon.Stop, "Stop response") else ChatGlyph(ChatIcon.Send,"Send message")
                }
            }
        }
    }
}
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun SessionSidebarRow(id: String, title: String?, selected: Boolean, enabled: Boolean, running: Boolean, modifier: Modifier = Modifier,
    archived: Boolean = false, working: Boolean = false, canFork: Boolean = true, onAction: (String, String) -> Unit = { _, _ -> }, onClick: () -> Unit) {
    var menu by remember(id, archived) { mutableStateOf(false) }
    val haptic = LocalHapticFeedback.current
    val openActions = { haptic.performHapticFeedback(HapticFeedbackType.LongPress); menu = true }
    val actionLabel = "Actions for ${title ?: id}"
    val interaction = if (archived) {
        Modifier.pointerInput(enabled, working, id) { if (enabled && !working) detectTapGestures(onLongPress = { openActions() }) }
            .then(if (enabled && !working) Modifier.semantics { onLongClick(actionLabel) { openActions(); true } } else Modifier)
    } else Modifier.combinedClickable(
        interactionSource = remember { MutableInteractionSource() }, indication = LocalIndication.current,
        enabled = enabled && !working, onClick = onClick,
        onLongClickLabel = actionLabel, onLongClick = openActions,
    )
    Box(modifier.fillMaxWidth()) {
        Surface(color = if (selected) Mocha.Blue else Mocha.Mantle, contentColor = if (selected) Mocha.Base else Mocha.Text, shape = RoundedCornerShape(6.dp), modifier = Modifier.fillMaxWidth().then(interaction).semantics(mergeDescendants = true) { this.selected = selected }) {
            Row(Modifier.fillMaxWidth().heightIn(min = 48.dp).padding(horizontal = 12.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(title ?: "Untitled session · ${id.take(8)}", maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                if (running || working) CircularProgressIndicator(Modifier.size(14.dp).semantics { contentDescription = if (working) "Session action pending" else "Session running" }, strokeWidth = 2.dp, color = if (selected) Mocha.Base else Mocha.Blue)
            }
        }
        ModexDropdownMenu(menu, onDismissRequest = { menu = false }) {
            DropdownMenuItem(text = { Text(if (archived) "Restore" else "Archive") }, leadingIcon = { ChatGlyph(if (archived) ChatIcon.Restore else ChatIcon.Archive) }, onClick = { menu = false; onAction(if (archived) "unarchive" else "archive", id) })
            if (!archived) DropdownMenuItem(text = { Text("Fork") }, leadingIcon = { ChatGlyph(ChatIcon.Fork) }, enabled = !running && canFork, onClick = { menu = false; onAction("fork", id) })
            DropdownMenuItem(text = { Text("Delete", color = Mocha.Red) }, leadingIcon = { ChatGlyph(ChatIcon.Delete, color = Mocha.Red) }, onClick = { menu = false; onAction("delete", id) })
        }
    }
}
