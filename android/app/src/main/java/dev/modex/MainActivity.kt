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
import android.os.Build

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(statusBarStyle = SystemBarStyle.dark(0xFF1E1E2E.toInt()), navigationBarStyle = SystemBarStyle.dark(0xFF1E1E2E.toInt()))
        setContent {
            ModexTheme { Surface(Modifier.fillMaxSize()) { ConnectionHome() } }
        }
    }
}

@Composable
internal fun HostWorkspace(vm: ModexViewModel, hostName: String, initiallyEditing: Boolean, onHome: () -> Unit, onName: (String) -> Unit) {
    val context = LocalContext.current
    var wasConnected by remember { mutableStateOf(vm.connected) }
    var wasConnecting by remember { mutableStateOf(vm.connecting) }
    LaunchedEffect(vm.connected, vm.connecting) {
        if (wasConnected && !vm.connected && !vm.connecting) onHome()
        if (vm.connected) wasConnected = true
    }
    val settingsState = rememberSaveableStateHolder()
    val notifications = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    LaunchedEffect(Unit) { if (Build.VERSION.SDK_INT >= 33) notifications.launch(android.Manifest.permission.POST_NOTIFICATIONS) }
    var connectionSettings by rememberSaveable { mutableStateOf(initiallyEditing) }
    var openAfterConnect by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(vm.connected, vm.connecting) {
        if (vm.connected && openAfterConnect) connectionSettings = false
        if (!vm.connecting) {
            if (wasConnecting && !vm.connected && !connectionSettings && vm.error.isNotBlank()) onHome()
            openAfterConnect = false
        }
        wasConnecting = vm.connecting
    }
    var phoneThread by rememberSaveable { mutableStateOf("") }
    var phoneGeneration by rememberSaveable { mutableStateOf(0) }
    var phoneTarget by rememberSaveable { mutableStateOf("") }
    var remoteThread by rememberSaveable { mutableStateOf("") }
    val documentPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
        if (phoneTarget == vm.savedConnection.toString()) vm.attachments.fromPhone(phoneThread, uris, phoneGeneration, context.contentResolver)
    }
    var projectBrowser by rememberSaveable { mutableStateOf(false) }
    var projectFolder by rememberSaveable { mutableStateOf("") }
    var projectNameInput by rememberSaveable { mutableStateOf("") }
    LaunchedEffect(vm.connected) { if (!vm.connected) { projectBrowser = false; projectFolder = "" } }
    if (projectBrowser) RemoteFilePicker(
        "~", vm.attachments.directory, vm.attachments.working, vm.attachments.error, vm.connected,
        0, { vm.attachments.browse(it) }, {}, { projectBrowser = false },
        onChooseDirectory = { path -> projectFolder = path; projectNameInput = projectName(path); projectBrowser = false },
        onCreateDirectory = vm.attachments::createDirectory
    )
    if (projectFolder.isNotBlank()) AlertDialog(
        onDismissRequest = { projectFolder = "" }, title = { Text("New project") },
        text = { Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            OutlinedTextField(projectNameInput, { projectNameInput = it }, label = { Text("Project name") }, singleLine = true)
            Text(projectFolder, color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
        } },
        confirmButton = { TextButton(enabled = vm.connected && !vm.projectCreating && projectNameInput.isNotBlank() && projectNameInput.toByteArray().size <= 256 && projectNameInput.none { it.isISOControl() }, onClick = { vm.createProject(projectNameInput, projectFolder); projectFolder = "" }) { Text("Create project") } },
        dismissButton = { TextButton(onClick = { projectFolder = "" }) { Text("Cancel") } }
    )
    if (remoteThread.isNotEmpty()) RemoteFilePicker(
        vm.attachments.directoryFor(remoteThread), vm.attachments.directory, vm.attachments.working, vm.attachments.error, vm.connected,
        MAX_ATTACHMENTS - vm.attachments.drafts[remoteThread].orEmpty().size, { vm.attachments.browse(it, remoteThread) },
        { files -> files.forEach { vm.attachments.addRemote(remoteThread, it) }; remoteThread = "" }, { remoteThread = "" }
    )
    if ((vm.connected || vm.connecting || vm.savedConnection != null) && !connectionSettings) {
        ChatWorkspace(
            host = hostName, connected = vm.connected, connecting = vm.connecting,
            selected = vm.selected, threads = vm.threads, titles = vm.titles, messages = vm.messages,
            runningThreads = vm.runningThreads, busy = vm.busy, stopping = vm.stopping, onStop = vm::stop, historyLoading = vm.historyLoading, hasMore = vm.nextCursor != null, error = vm.error, sessionSettings = vm.sessionSettings,
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
            onCreate = { model, mode, effort -> vm.create(model, mode, effort) }, onCreateInProject = vm::create,
            onNewProject = { vm.attachments.browse("~"); projectBrowser = true }, onSend = vm::send, onAnswer = vm::answer,
            onConnection = { connectionSettings = true }, onDisconnect = vm::disconnect, onHome = onHome,
            takeoverSession = vm.takeoverThread, onTakeover = vm::takeOver,
            models = vm.models, modelCatalogStatus = vm.modelCatalogStatus,
            referenceCatalog = vm.references.catalog, referencesLoading = vm.references.loading, referenceStatus = vm.references.status, onRefreshReferences = { vm.refreshReferences() },
            threadWorkspaces = vm.threadWorkspaces, threadListLoading = vm.threadListLoading,
            registeredProjects = vm.projects, projectMembership = vm.projectMembership, projectStatus = vm.projectStatus,
            attachments = vm.attachments.drafts[vm.selected].orEmpty(), attachmentWorking = vm.attachments.working,
            attachmentProgress = vm.attachments.progress, attachmentError = vm.attachments.error,
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
                        vm.connect(connection)
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
