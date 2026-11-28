package dev.modex

import android.os.Bundle
import androidx.activity.enableEdgeToEdge
import androidx.activity.SystemBarStyle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(statusBarStyle = SystemBarStyle.dark(0xFF1E1E2E.toInt()), navigationBarStyle = SystemBarStyle.dark(0xFF1E1E2E.toInt()))
        setContent {
            ModexTheme { Surface(Modifier.fillMaxSize()) { ConnectionHome() } }
        }
    }

    override fun onStop() {
        (application as ModexApplication).sessions.values.forEach { it.flushDrafts() }
        super.onStop()
    }
}

@Composable
internal fun HostWorkspace(
    vm: ModexViewModel,
    hostName: String,
    initiallyEditing: Boolean,
    onHome: () -> Unit,
    onName: (String) -> Unit,
    onConnect: (Connection) -> Unit = vm::connect,
) {
    val context = LocalContext.current
    var wasConnected by remember { mutableStateOf(vm.connected) }
    LaunchedEffect(vm.connected, vm.connecting) {
        if (wasConnected && !vm.connected && !vm.connecting && vm.connectFailure.isBlank()) onHome()
        if (vm.connected) wasConnected = true
    }
    val settingsState = rememberSaveableStateHolder()
    var connectionSettings by rememberSaveable { mutableStateOf(initiallyEditing) }
    var openAfterConnect by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(vm.connected, vm.connecting) {
        if (vm.connected && openAfterConnect) connectionSettings = false
        if (!vm.connecting) openAfterConnect = false
    }
    var phoneThread by rememberSaveable { mutableStateOf("") }
    var phoneGeneration by rememberSaveable { mutableStateOf(0) }
    var phoneTarget by rememberSaveable { mutableStateOf("") }
    var remoteThread by rememberSaveable { mutableStateOf("") }
    var remotePreview by rememberSaveable(remoteThread, saver = AttachmentPreviewStateSaver) {
        mutableStateOf<Attachment?>(null)
    }
    val documentPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
        if (phoneTarget == vm.savedConnection.toString()) vm.attachments.fromPhone(phoneThread, uris, phoneGeneration, context.contentResolver)
    }
    var projectBrowser by rememberSaveable { mutableStateOf(false) }
    var projectFolder by rememberSaveable { mutableStateOf("") }
    var projectNameInput by rememberSaveable { mutableStateOf("") }
    LaunchedEffect(vm.connected) { if (!vm.connected) { projectBrowser = false; projectFolder = ""; remotePreview = null } }
    if (projectBrowser) RemoteFilePicker(
        "~", vm.attachments.directory, vm.attachments.working, vm.attachments.error, vm.connected,
        0, { vm.attachments.browse(it) }, {}, { projectBrowser = false },
        onChooseDirectory = { path -> projectFolder = path; projectNameInput = projectName(path); projectBrowser = false },
        onCreateDirectory = vm.attachments::createDirectory,
        retryPath = vm.attachments.attemptedPath,
    )
    if (projectFolder.isNotBlank()) AlertDialog(
        onDismissRequest = { projectFolder = "" }, title = { Text("New project") },
        text = { Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedTextField(projectNameInput, { projectNameInput = it }, label = { Text("Project name") }, singleLine = true)
            Text(projectFolder, color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
        } },
        confirmButton = { TextButton(enabled = vm.backendCapabilities.projectCreation && vm.connected && !vm.projectCreating && projectNameInput.isNotBlank() && projectNameInput.toByteArray().size <= 256 && projectNameInput.none { it.isISOControl() }, onClick = { vm.createProject(projectNameInput, projectFolder); projectFolder = "" }) { Text("Create project") } },
        dismissButton = { TextButton(onClick = { projectFolder = "" }) { Text("Cancel") } }
    )
    if (remoteThread.isNotEmpty()) RemoteFilePicker(
        vm.attachments.directoryFor(remoteThread), vm.attachments.directory, vm.attachments.working, vm.attachments.error, vm.connected,
        MAX_ATTACHMENTS - vm.attachments.drafts[remoteThread].orEmpty().size, { vm.attachments.browse(it, remoteThread) },
        { files -> files.forEach { vm.attachments.addRemote(remoteThread, it) }; remoteThread = "" }, { remoteThread = "" },
        retryPath = vm.attachments.attemptedPath,
        onPreview = { remotePreview = Attachment(it.path, it.name, attachmentKind(it.name), it.size) },
    )
    remotePreview?.let { file -> AttachmentViewer(file, vm::attachmentFile) { remotePreview = null } }
    if ((vm.connected || vm.connecting || vm.savedConnection != null) && !connectionSettings) {
        ChatWorkspace(
            host = hostName, connected = vm.connected, connecting = vm.connecting,
            backendCapabilities = vm.backendCapabilities, sessionCapabilities = vm.sessionCapabilities,
            selected = vm.selected, threads = vm.threads, titles = vm.titles, messages = vm.messages,
            runningThreads = vm.runningThreads, busy = vm.busy, activeTurnId = vm.activeTurnId, steerSending = vm.steerSending, stopping = vm.stopping, onStop = vm::stop, historyLoading = vm.historyLoading, hasMore = vm.nextCursor != null, error = vm.error, sessionSettings = vm.sessionSettings,
            sessionModel = vm.sessionModel, sessionEffort = vm.sessionEffort, hostModel = vm.hostChoice.model, hostEffort = vm.hostChoice.effort, onModelChoice = vm::chooseModel, decisions = vm.decisions, answeringDecisionIds = vm.answeringDecisionIds,
            sessionStatus = vm.sessionStatus, sessionStatusLoading = vm.sessionStatusLoading, sessionStatusError = vm.sessionStatusError, modeChanging = vm.modeChanging, modeUncertain = vm.modeUncertain,
            onSessionStatus = vm::refreshSessionStatus, onSessionMode = vm::changeSessionMode,
            permissionProfiles = vm.permissionProfiles, permissionsWorking = vm.permissionsWorking,
            permissionsMessage = vm.permissionsMessage, onSessionPermissions = vm::sessionPermissions,
            usageWarning = vm.usageWarning,
            handledPlans = vm.handledPlans, planResponding = vm.planResponding, onPlanAction = vm::respondPlan,
            hasOlderHistory = vm.hasOlderHistory, olderHistoryLoading = vm.olderHistoryLoading, onOlderHistory = vm::loadOlderHistory,
            archived = vm.archived, onArchived = vm::showArchived, actionThreads = vm.actionThreads,
            onSessionAction = { action, target, confirmed -> vm.lifecycle(action, target, confirmed) },
            onSelect = vm::select, onRefresh = vm::refreshThreads, onMore = vm::more,
            onCreate = { model, mode, effort -> vm.create(model, mode, effort) },
            onCreateInProject = { model, mode, effort, workspace, project -> vm.create(model, mode, effort, workspace, project) },
            onCreateWithAgent = { agent, model, mode, effort, workspace, project -> vm.create(model, mode, effort, workspace, project, agent) },
            onNewChatScope = vm::scopeNewChat,
            onNewProject = { vm.attachments.browse("~"); projectBrowser = true }, onSend = vm::send, onAnswer = vm::answer,
            onConnection = { connectionSettings = true }, onDisconnect = vm::disconnect, onHome = onHome,
            takeoverSession = vm.takeoverThread, onTakeover = vm::takeOver,
            models = vm.models, modelCatalogStatus = vm.modelCatalogStatus,
            newChatModels = vm.newChatModels, newChatModelCatalogStatus = vm.newChatModelCatalogStatus,
            chatAgents = vm.chatAgents, agentsLoading = vm.agentsLoading, sessionAgent = vm.sessionAgent, threadAgents = vm.threadAgents,
            agyOptions = vm.agyOptions, agyOptionsLoading = vm.agyOptionsLoading, agyOptionsError = vm.agyOptionsError,
            agyOptionsUncertain = vm.agyOptionsUncertain, onRefreshAgyOptions = vm::refreshAntigravityOptions,
            onSetAgyOption = vm::setAntigravityOption,
            goal = vm.goal, goalLoading = vm.goalLoading, goalWorking = vm.goalWorking,
            goalUncertain = vm.goalUncertain, goalError = vm.goalError,
            onGoalRefresh = vm::refreshGoal, onGoalSet = vm::setGoal, onGoalClear = vm::clearGoal,
            referenceCatalog = vm.references.catalog, referencesLoading = vm.references.loading, referenceStatus = vm.references.status, onRefreshReferences = { vm.refreshReferences() },
            threadWorkspaces = vm.threadWorkspaces, threadListLoading = vm.threadListLoading, threadListStatus = vm.threadListStatus,
            registeredProjects = vm.projects, projectMembership = vm.projectMembership, projectStatus = vm.projectStatus,
            attachments = vm.attachments.drafts[vm.selected].orEmpty(), attachmentWorking = vm.attachments.working,
            attachmentProgress = vm.attachments.progress, attachmentError = vm.attachments.error.ifBlank { vm.promptDrafts.error },
            promptDrafts = vm.promptDrafts,
            agentConversation = vm.agentConversation, onOpenAgent = vm::openAgent, onAgentHistory = vm::loadAgentHistory,
            loadAttachment = vm::attachmentFile,
            onPhoneFiles = { phoneThread = vm.selected; phoneGeneration = vm.attachments.generation; phoneTarget = vm.savedConnection.toString(); documentPicker.launch(arrayOf("*/*")) },
            onRemoteFiles = { remoteThread = vm.selected; vm.attachments.browse(vm.attachments.directoryFor(remoteThread), remoteThread) },
            onRemoveAttachment = { vm.attachments.remove(vm.selected, it) }
        )
    } else settingsState.SaveableStateProvider("host-settings") {
        HostSettings(
            hostName, vm.savedConnection, vm.keyLoaded,
            onImportKey = vm::importKey,
            onBack = { if (initiallyEditing || vm.savedConnection == null) onHome() else connectionSettings = false },
            onHome = onHome,
            onSubmit = { name, connection, connect ->
                if (vm.configure(connection)) {
                    onName(name)
                    if (connect) {
                        onConnect(connection)
                        if (vm.connected) connectionSettings = false
                        else if (vm.connecting) openAfterConnect = true
                    } else onHome()
                }
                null
            },
            connecting = vm.connecting,
            canImport = !vm.connected && !vm.connecting,
            connectionError = vm.error,
            beforePickKey = vm::disconnect,
            onDisconnect = if (vm.connected || vm.connecting) vm::disconnect else null,
        )
    }
}
