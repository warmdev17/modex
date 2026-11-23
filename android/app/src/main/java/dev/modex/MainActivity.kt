package dev.modex

import android.os.Bundle
import androidx.activity.compose.BackHandler
import androidx.activity.enableEdgeToEdge
import androidx.activity.SystemBarStyle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.Alignment
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
    var displayName by rememberSaveable { mutableStateOf(hostName) }
    var wasConnected by remember { mutableStateOf(vm.connected) }
    var wasConnecting by remember { mutableStateOf(vm.connecting) }
    LaunchedEffect(vm.connected, vm.connecting) {
        if (wasConnected && !vm.connected && !vm.connecting) onHome()
        if (vm.connected) wasConnected = true
    }
    val saved = vm.savedConnection
    var host by rememberSaveable { mutableStateOf(saved?.host.orEmpty()) }; var port by rememberSaveable { mutableStateOf(saved?.port?.toString() ?: "22") }
    var user by rememberSaveable { mutableStateOf(saved?.user.orEmpty()) }; var pin by rememberSaveable { mutableStateOf(saved?.pin.orEmpty()) }
    var binary by rememberSaveable { mutableStateOf(saved?.binary.orEmpty()) }; var state by rememberSaveable { mutableStateOf(saved?.state.orEmpty()) }
    var advanced by rememberSaveable { mutableStateOf(false) }
    var showConnectionError by remember { mutableStateOf(false) }
    val notifications = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    LaunchedEffect(Unit) { if (Build.VERSION.SDK_INT >= 33) notifications.launch(android.Manifest.permission.POST_NOTIFICATIONS) }
    var connectionSettings by rememberSaveable { mutableStateOf(initiallyEditing) }; var importError by rememberSaveable { mutableStateOf("") }
    var openAfterConnect by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(vm.connected, vm.connecting) {
        if (vm.connected && openAfterConnect) connectionSettings = false
        if (!vm.connecting) {
            if (wasConnecting && !vm.connected && !connectionSettings && vm.error.isNotBlank()) onHome()
            openAfterConnect = false
        }
        wasConnecting = vm.connecting
    }
    val keyPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) try {
            if (vm.connected || vm.connecting) {
                importError = "Disconnect before replacing the SSH key."
                return@rememberLauncherForActivityResult
            }
            val bytes = context.contentResolver.openInputStream(uri)?.use(::readImportedKey)
                ?: throw java.io.IOException()
            vm.importKey(bytes); importError = ""
        } catch (_: SecurityException) {
            importError = "Android denied access to this file. Copy it to Downloads and select it again."
        } catch (_: java.io.IOException) {
            importError = "Could not read this file. Copy it to Downloads on this phone and select it again."
        } catch (error: IllegalArgumentException) {
            importError = when (error.message) {
                "The selected file is empty.", "The selected file exceeds 256 KiB." -> error.message!!
                else -> "Could not import key. Disconnect and select the file again."
            }
        } catch (_: Exception) {
            importError = "The file provider could not open this file. Copy it to Downloads and select it again."
        }
    }
    BackHandler(connectionSettings) { if (initiallyEditing || vm.savedConnection == null) onHome() else connectionSettings = false }
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
    } else Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding().padding(horizontal = 16.dp)) {
        Row(Modifier.fillMaxWidth().height(56.dp).semantics { contentDescription = "Host settings toolbar" }, verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = { if (initiallyEditing || vm.savedConnection == null) onHome() else connectionSettings = false }) {
                Box(Modifier.rotate(180f)) { ChatGlyph(ChatIcon.ChevronRight, "Back") }
            }
            Text("Host settings", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
            IconButton(onClick = onHome) { ChatGlyph(ChatIcon.Computer, "All hosts") }
        }
        val issue = importError.ifBlank { vm.error }
        val draft = Connection(host.trim(), port.toIntOrNull() ?: 0, user.trim(), pin.trim(), binary.trim(), state.trim())
        val canSave = vm.keyLoaded && displayName.isNotBlank() && host.isNotBlank() && user.isNotBlank() && pin.isNotBlank() && !vm.connecting
        if (showConnectionError && issue.isNotBlank()) AlertDialog(
            onDismissRequest = { showConnectionError = false },
            title = { Text("Connection details") },
            text = { Text(issue) },
            confirmButton = { TextButton(onClick = { showConnectionError = false }) { Text("Close") } },
        )
        Column(Modifier.weight(1f).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (vm.connecting) LinearProgressIndicator(Modifier.fillMaxWidth())
                OutlinedTextField(displayName, { displayName = it.take(80) }, label = { Text("Host name") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(host, { host = it }, label = { Text("Hostname or IP") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(user, { user = it }, label = { Text("SSH username") }, singleLine = true, modifier = Modifier.weight(2f))
                    OutlinedTextField(port, { port = it.filter(Char::isDigit).take(5) }, label = { Text("Port") }, singleLine = true, modifier = Modifier.weight(1f),
                        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Number))
                }
                OutlinedTextField(pin, { pin = it }, label = { Text("SSH fingerprint") }, placeholder = { Text("SHA256:…") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedButton(onClick = {
                    // Returning from the picker must not auto-reconnect with the old key.
                    vm.disconnect()
                    importError = ""
                    keyPicker.launch(arrayOf("*/*"))
                }, enabled = !vm.connected && !vm.connecting, modifier = Modifier.fillMaxWidth()) {
                    Text(if (vm.keyLoaded) "Replace SSH key" else "Import SSH key")
                }
                TextButton(onClick = { advanced = !advanced }) {
                    ChatGlyph(if (advanced) ChatIcon.ChevronDown else ChatIcon.ChevronRight)
                    Spacer(Modifier.width(6.dp))
                    Text("Advanced settings")
                }
                if (advanced) {
                    OutlinedTextField(binary, { binary = it }, label = { Text("Host binary") }, placeholder = { Text("~/.local/bin/modex-host") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(state, { state = it }, label = { Text("State directory") }, placeholder = { Text("~/.local/state/modex-phone") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    Text("Leave blank for defaults. Custom paths must be absolute.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
                }
                if (vm.connected || vm.connecting) TextButton(onClick = vm::disconnect) { Text("Disconnect") }
        }
        if (issue.isNotBlank()) TextButton(onClick = { showConnectionError = true }) {
            Text(issue, maxLines = 2, overflow = TextOverflow.Ellipsis, color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
        }
        Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = {
                if (vm.configure(draft)) { onName(displayName); onHome() }
            }, enabled = canSave, modifier = Modifier.weight(1f)) { Text("Save host") }
            Button(onClick = {
                if (vm.configure(draft)) {
                    onName(displayName)
                    vm.connect(draft)
                    if (vm.connected) connectionSettings = false
                    else if (vm.connecting) openAfterConnect = true
                }
            }, enabled = canSave, modifier = Modifier.weight(1f)) { Text("Connect") }
        }
    }
}
