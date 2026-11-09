package dev.modex

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.Alignment
import android.os.Build
import org.json.JSONArray
import org.json.JSONObject

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            val colors = darkColorScheme(primary = Color.White, onPrimary = Color.Black, background = Color(0xFF101010), surface = Color(0xFF101010), onSurface = Color(0xFFF4F4F4), surfaceContainer = Color(0xFF202020), surfaceContainerHighest = Color(0xFF2B2B2B), primaryContainer = Color(0xFF303030), onPrimaryContainer = Color.White)
            MaterialTheme(colorScheme = colors) { Surface(Modifier.fillMaxSize()) { ModexApp() } }
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
    var text by rememberSaveable { mutableStateOf("") }; var model by rememberSaveable { mutableStateOf("") }
    var sessions by rememberSaveable { mutableStateOf(false) }; var importError by rememberSaveable { mutableStateOf("") }
    val keyPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) try {
            val bytes = context.contentResolver.openInputStream(uri)?.use { input ->
                val output = java.io.ByteArrayOutputStream()
                val chunk = ByteArray(4096)
                while (true) { val count = input.read(chunk); if (count < 0) break; require(output.size() + count <= 256 * 1024); output.write(chunk, 0, count) }
                output.toByteArray() } ?: error("No file")
            vm.importKey(bytes); importError = ""
        } catch (_: Exception) { importError = "Could not import key. Select an unencrypted private key under 256 KiB." }
    }
    Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding().padding(16.dp)) {
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
            Column { Text("modex", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Bold)
                Text(if (vm.connected) "Connected · SSH" else if (vm.connecting) "Connecting…" else "Your Codex, wherever you are", style = MaterialTheme.typography.labelLarge) }
            if (vm.connected || vm.connecting) TextButton(onClick = vm::disconnect) { Text("Disconnect") }
        }
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
                OutlinedButton(onClick = { keyPicker.launch(arrayOf("*/*")) }, modifier = Modifier.fillMaxWidth()) { Text(if (vm.keyLoaded) "Replace SSH key" else "Import SSH private key") }
                Text("Your key and connection are encrypted on this device. Use Forget saved host to remove them. Passphrase keys are not supported yet.", style = MaterialTheme.typography.bodySmall)
                if (importError.isNotEmpty()) Text(importError, color = MaterialTheme.colorScheme.error)
                Button(onClick = { vm.connect(Connection(host.trim(), port.toIntOrNull() ?: 0, user.trim(), pin.trim(), binary, state)) }, enabled = vm.keyLoaded && host.isNotBlank() && user.isNotBlank() && pin.isNotBlank() && state.startsWith("/"), modifier = Modifier.fillMaxWidth()) { Text("Connect / reconnect") }
                Row {
                    TextButton(onClick = vm::reset) { Text("Reset session cursor") }
                    TextButton(onClick = { vm.forget(); host = ""; user = ""; pin = ""; state = "" }) { Text("Forget saved host") }
                }
            }
        } else if (vm.connecting) {
            LinearProgressIndicator(Modifier.fillMaxWidth().padding(top = 16.dp))
        } else {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                TextButton(onClick = { sessions = true; vm.refreshThreads() }) { Text(if (vm.selected.isEmpty()) "Choose session" else "Sessions · ${vm.selected.take(8)}") }
                Text(if (vm.historyLoading) "Loading…" else if (vm.busy) "Working…" else "Ready", modifier = Modifier.padding(12.dp))
            }
            if (sessions || vm.selected.isEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(model, { model = it }, label = { Text("Model (optional)") }, singleLine = true, modifier = Modifier.weight(1f))
                    Button(onClick = { vm.create(model); sessions = false }, enabled = !vm.busy) { Text("New chat") }
                }
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("${vm.threads.size} sessions loaded", modifier = Modifier.padding(12.dp))
                    TextButton(onClick = vm::refreshThreads) { Text("Refresh") }
                }
                if (vm.threads.isEmpty()) Text("No sessions loaded for this workspace. Refresh or create a new chat.")
                LazyColumn(Modifier.weight(1f)) {
                    items(vm.threads) { id -> TextButton(onClick = { vm.select(id); sessions = false }, enabled = !vm.busy, modifier = Modifier.fillMaxWidth()) { Text(vm.titles[id] ?: id, maxLines = 2) } }
                    if (vm.nextCursor != null) item { TextButton(onClick = vm::more) { Text("Load more") } }
                }
            } else {
                if (vm.historyLoading) LinearProgressIndicator(Modifier.fillMaxWidth())
                val scroll = androidx.compose.foundation.lazy.rememberLazyListState()
                LaunchedEffect(vm.messages.lastOrNull()?.second) { if (vm.messages.isNotEmpty()) scroll.animateScrollToItem(vm.messages.lastIndex) }
                LazyColumn(Modifier.weight(1f), state = scroll, verticalArrangement = Arrangement.spacedBy(12.dp), contentPadding = PaddingValues(vertical = 16.dp)) {
                    items(vm.messages) { (role, body) ->
                        Column(Modifier.fillMaxWidth(), horizontalAlignment = if (role == "You") Alignment.End else Alignment.Start) {
                            if (role == "You") Surface(color = MaterialTheme.colorScheme.surfaceContainerHighest, shape = MaterialTheme.shapes.large, modifier = Modifier.widthIn(max = 320.dp)) {
                                SelectionContainer { Text(body, modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp), style = MaterialTheme.typography.bodyLarge) }
                            } else SelectionContainer { Text(body, modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp), style = MaterialTheme.typography.bodyLarge) }
                        }
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(text, { text = it }, label = { Text("Message Codex") }, modifier = Modifier.weight(1f), maxLines = 5)
                    Button(onClick = { vm.send(text); text = "" }, enabled = !vm.busy && !vm.historyLoading && text.isNotBlank(), modifier = Modifier.heightIn(min = 48.dp)) { Text("Send") }
                }
            }
        }
    }
    vm.decisions.firstOrNull()?.takeIf { vm.connected }?.let { decision -> DecisionDialog(decision) { vm.answer(decision, it) } }
}

@Composable
internal fun DecisionDialog(decision: JSONObject, respond: (JSONObject) -> Unit) {
    val context = decision.getJSONObject("context")
    val fileMode = decision.optString("method") == "item/fileChange/requestApproval"
    val questionMode = decision.optString("method") == "item/tool/requestUserInput"
    val questions = context.optJSONArray("questions") ?: JSONArray()
    val answers = remember(decision.optString("decision_id")) { mutableStateMapOf<String, String>() }
    val offered = context.optJSONArray("availableDecisions")
    fun offered(choice: String): Boolean = offered == null || (0 until offered.length()).any { offered.opt(it) == choice }
    AlertDialog(onDismissRequest = {}, title = { Text(if (questionMode) "Codex needs your input" else if (fileMode) "Review file access" else "Allow this command?") }, text = {
        Column(Modifier.heightIn(max = 420.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Session ${decision.optString("thread_id").take(8)}")
            if (questionMode) {
                for (i in 0 until questions.length()) {
                    val q = questions.getJSONObject(i); val id = q.getString("id")
                    Text(q.getString("question"))
                    val options = q.optJSONArray("options")
                    if (options != null) for (n in 0 until options.length()) {
                        val option = options.getJSONObject(n)
                        OutlinedButton(onClick = { answers[id] = option.getString("label") }) { Text(option.getString("label")) }
                        Text(option.optString("description"), style = MaterialTheme.typography.bodySmall)
                    }
                    OutlinedTextField(answers[id].orEmpty(), { answers[id] = it }, label = { Text("Your answer") })
                }
            } else {
                Text(if (fileMode) "File changes cannot be allowed until a diff viewer is available. You can decline or cancel this request." else "Codex is waiting for your permission. Allowing this request does not approve future commands.", style = MaterialTheme.typography.bodyMedium)
                for ((label, value) in approvalDetails(context)) {
                    Column(Modifier.fillMaxWidth().padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        if (label == "Command") Surface(color = MaterialTheme.colorScheme.surfaceContainerHighest, shape = MaterialTheme.shapes.small) {
                            SelectionContainer { Text(value, modifier = Modifier.fillMaxWidth().padding(12.dp), fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodyMedium) }
                        } else SelectionContainer { Text(value, style = MaterialTheme.typography.bodyMedium) }
                    }
                }
            }
        }
    }, confirmButton = {
        if (questionMode) Button(enabled = (0 until questions.length()).all { !answers[questions.getJSONObject(it).getString("id")].isNullOrBlank() }, onClick = {
            val result = JSONObject(); answers.forEach { (id, answer) -> result.put(id, JSONObject().put("answers", JSONArray().put(answer))) }; respond(JSONObject().put("answers", result))
        }) { Text("Submit answer") }
        else if (offered("accept") && decision.optString("method") != "item/fileChange/requestApproval") Button(onClick = { respond(JSONObject().put("decision", "accept")) }) { Text("Allow once") }
    }, dismissButton = {
        Row {
            if (!questionMode && offered("decline")) TextButton(onClick = { respond(JSONObject().put("decision", "decline")) }) { Text("Decline") }
            if (!questionMode && offered("cancel")) TextButton(onClick = { respond(JSONObject().put("decision", "cancel")) }) { Text("Cancel action") }
        }
    })
}
