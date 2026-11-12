package dev.modex

import android.os.Bundle
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
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.Alignment
import android.os.Build

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge(statusBarStyle = SystemBarStyle.dark(0xFF1E1E2E.toInt()), navigationBarStyle = SystemBarStyle.dark(0xFF1E1E2E.toInt()))
        setContent {
            ModexTheme { Surface(Modifier.fillMaxSize()) { ModexApp() } }
        }
    }
}

@Composable
private fun ModexApp() {
    val context = LocalContext.current
    val app = context.applicationContext as ModexApplication
    val vm: ModexViewModel = viewModel(viewModelStoreOwner = app, factory = ViewModelProvider.AndroidViewModelFactory.getInstance(app))
    val saved = vm.savedConnection
    var host by rememberSaveable { mutableStateOf(saved?.host.orEmpty()) }; var port by rememberSaveable { mutableStateOf(saved?.port?.toString() ?: "22") }
    var user by rememberSaveable { mutableStateOf(saved?.user.orEmpty()) }; var pin by rememberSaveable { mutableStateOf(saved?.pin.orEmpty()) }
    var binary by rememberSaveable { mutableStateOf(saved?.binary ?: "/tmp/modex-host") }; var state by rememberSaveable { mutableStateOf(saved?.state.orEmpty()) }
    val owner = LocalLifecycleOwner.current
    DisposableEffect(owner) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_START) vm.foreground() }
        owner.lifecycle.addObserver(observer)
        onDispose { owner.lifecycle.removeObserver(observer) }
    }
    val notifications = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    LaunchedEffect(Unit) { if (Build.VERSION.SDK_INT >= 33) notifications.launch(android.Manifest.permission.POST_NOTIFICATIONS) }
    var connectionSettings by rememberSaveable { mutableStateOf(false) }; var importError by rememberSaveable { mutableStateOf("") }
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
    var phoneThread by rememberSaveable { mutableStateOf("") }
    var phoneGeneration by rememberSaveable { mutableStateOf(0) }
    var phoneTarget by rememberSaveable { mutableStateOf("") }
    var remoteThread by rememberSaveable { mutableStateOf("") }
    val documentPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenMultipleDocuments()) { uris ->
        if (phoneTarget == vm.savedConnection.toString()) vm.attachments.fromPhone(phoneThread, uris, phoneGeneration, context.contentResolver)
    }
    if (remoteThread.isNotEmpty()) RemoteFilePicker(
        vm.attachments.directoryFor(remoteThread), vm.attachments.directory, vm.attachments.working, vm.attachments.error, vm.connected,
        MAX_ATTACHMENTS - vm.attachments.drafts[remoteThread].orEmpty().size, { vm.attachments.browse(it, remoteThread) },
        { files -> files.forEach { vm.attachments.addRemote(remoteThread, it) }; remoteThread = "" }, { remoteThread = "" }
    )
    if ((vm.connected || vm.connecting || vm.savedConnection != null) && !connectionSettings) {
        ChatWorkspace(
            host = vm.savedConnection?.host.orEmpty(), connected = vm.connected, connecting = vm.connecting,
            selected = vm.selected, threads = vm.threads, titles = vm.titles, messages = vm.messages,
            busy = vm.busy, historyLoading = vm.historyLoading, hasMore = vm.nextCursor != null, error = vm.error, sessionSettings = vm.sessionSettings,
            sessionModel = vm.sessionModel, sessionEffort = vm.sessionEffort, hostModel = vm.hostChoice.model, hostEffort = vm.hostChoice.effort, onModelChoice = vm::chooseModel, decisions = vm.decisions, answeringDecisionIds = vm.answeringDecisionIds,
            onSelect = vm::select, onRefresh = vm::refreshThreads, onMore = vm::more,
            onCreate = vm::create, onSend = vm::send, onAnswer = vm::answer,
            onConnection = { connectionSettings = true }, onDisconnect = vm::disconnect,
            models = vm.models, modelCatalogStatus = vm.modelCatalogStatus,
            threadWorkspaces = vm.threadWorkspaces, threadListLoading = vm.threadListLoading,
            registeredProjects = vm.projects, projectMembership = vm.projectMembership, projectStatus = vm.projectStatus,
            attachments = vm.attachments.drafts[vm.selected].orEmpty(), attachmentWorking = vm.attachments.working,
            attachmentProgress = vm.attachments.progress, attachmentError = vm.attachments.error,
            loadAttachment = vm::attachmentFile,
            onPhoneFiles = { phoneThread = vm.selected; phoneGeneration = vm.attachments.generation; phoneTarget = vm.savedConnection.toString(); documentPicker.launch(arrayOf("*/*")) },
            onRemoteFiles = { remoteThread = vm.selected; vm.attachments.browse(vm.attachments.directoryFor(remoteThread), remoteThread) },
            onRemoveAttachment = { vm.attachments.remove(vm.selected, it) }
        )
    } else Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding().padding(20.dp)) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text("Connection", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.SemiBold)
                Text("Your Codex host", color = Mocha.Subtext, modifier = Modifier.padding(top = 4.dp))
            }
            if (vm.savedConnection != null) TextButton(onClick = { connectionSettings = false }) { Text("Back to chat") }
        }
        Spacer(Modifier.height(20.dp))
        if (vm.error.isNotBlank()) Text(vm.error, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(vertical = 8.dp))
        if (!vm.connected && !vm.connecting) {
            Column(Modifier.weight(1f).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("Connect to your host", style = MaterialTheme.typography.titleLarge)
                Text("Start the Modex supervisor on your laptop or VPS first. Verify its SSH fingerprint through a trusted channel.")
                OutlinedTextField(host, { host = it }, label = { Text("Hostname or IP") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(user, { user = it }, label = { Text("SSH username") }, singleLine = true, modifier = Modifier.weight(2f))
                    OutlinedTextField(port, { port = it }, label = { Text("Port") }, singleLine = true, modifier = Modifier.weight(1f))
                }
                OutlinedTextField(pin, { pin = it }, label = { Text("Verified SHA256 host fingerprint") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(binary, { binary = it }, label = { Text("Remote modex-host path") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(state, { state = it }, label = { Text("Remote supervisor state directory") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedButton(onClick = {
                    // Returning from the picker must not auto-reconnect with the old key.
                    vm.disconnect()
                    importError = ""
                    keyPicker.launch(arrayOf("*/*"))
                }, modifier = Modifier.fillMaxWidth()) { Text(if (vm.keyLoaded) "Replace SSH key" else "Import SSH private key") }
                Text("Your key and connection are encrypted on this device. Use Forget saved host to remove them. Passphrase keys are not supported yet.", style = MaterialTheme.typography.bodySmall)
                if (importError.isNotEmpty()) Text(importError, color = MaterialTheme.colorScheme.error)
                Button(onClick = { vm.connect(Connection(host.trim(), port.toIntOrNull() ?: 0, user.trim(), pin.trim(), binary, state)); connectionSettings = false }, enabled = vm.keyLoaded && host.isNotBlank() && user.isNotBlank() && pin.isNotBlank() && state.startsWith("/"), modifier = Modifier.fillMaxWidth()) { Text("Connect / reconnect") }
                Row {
                    TextButton(onClick = vm::reset) { Text("Reset session cursor") }
                    TextButton(onClick = { vm.forget(); host = ""; user = ""; pin = ""; state = "" }) { Text("Forget saved host") }
                }
            }
        } else {
            Text(if (vm.connecting) "Connecting…" else "Connected to $host", color = Mocha.Blue)
            TextButton(onClick = vm::disconnect) { Text("Disconnect") }
            TextButton(onClick = { connectionSettings = false }) { Text("Back to chat") }
        }
    }
}
