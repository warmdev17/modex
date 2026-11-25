package dev.modex

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
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
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.lifecycle.ViewModel

internal class NewHostDraft : ViewModel() {
    var key by mutableStateOf<ByteArray?>(null)
        private set

    fun replaceKey(bytes: ByteArray?) {
        key?.fill(0)
        key = bytes
    }

    override fun onCleared() = replaceKey(null)
}

@Composable
internal fun HostSettings(
    hostName: String,
    saved: Connection?,
    keyLoaded: Boolean,
    onImportKey: (ByteArray) -> Unit,
    onBack: () -> Unit,
    onHome: () -> Unit,
    onSubmit: (String, Connection, Boolean) -> String?,
    newHost: Boolean = false,
    connecting: Boolean = false,
    canImport: Boolean = true,
    connectionError: String = "",
    beforePickKey: () -> Unit = {},
    onDisconnect: (() -> Unit)? = null,
) {
    val context = LocalContext.current
    var displayName by rememberSaveable { mutableStateOf(hostName) }
    var host by rememberSaveable { mutableStateOf(saved?.host.orEmpty()) }
    var port by rememberSaveable { mutableStateOf(saved?.port?.toString() ?: "22") }
    var user by rememberSaveable { mutableStateOf(saved?.user.orEmpty()) }
    var pin by rememberSaveable { mutableStateOf(saved?.pin.orEmpty()) }
    var binary by rememberSaveable { mutableStateOf(saved?.binary.orEmpty()) }
    var state by rememberSaveable { mutableStateOf(saved?.state.orEmpty()) }
    var advanced by rememberSaveable { mutableStateOf(false) }
    var importError by rememberSaveable { mutableStateOf("") }
    var saveError by rememberSaveable { mutableStateOf("") }
    var fieldErrors by remember { mutableStateOf(HostFormErrors()) }
    var nameError by remember { mutableStateOf("") }
    var focusError by remember { mutableStateOf<String?>(null) }
    var showConnectionError by remember { mutableStateOf(false) }
    var showDiscard by rememberSaveable { mutableStateOf(false) }

    fun leave(exit: () -> Unit) {
        val dirty = keyLoaded || port != "22" || listOf(displayName, host, user, pin, binary, state).any { it.isNotBlank() }
        if (newHost && dirty) showDiscard = true else exit()
    }
    BackHandler { leave(onBack) }
    if (showDiscard) AlertDialog(
        onDismissRequest = { showDiscard = false },
        title = { Text("Discard draft?") },
        text = { Text("The details and any imported key for this host will be discarded.") },
        confirmButton = { TextButton(onClick = onHome) { Text("Discard draft", color = Mocha.Red) } },
        dismissButton = { TextButton(onClick = { showDiscard = false }) { Text("Keep editing") } },
    )

    fun submit(connect: Boolean) {
        importError = ""
        saveError = ""
        val cleanName = displayName.trim()
        nameError = when {
            !newHost && cleanName.isEmpty() -> "Enter a host name."
            cleanName.any(Char::isISOControl) -> "Enter a valid display name."
            else -> ""
        }
        fieldErrors = validateHostForm(host, port, user, pin, binary, state, keyLoaded)
        if (nameError.isNotBlank() || !fieldErrors.isEmpty()) {
            if (fieldErrors.binary.isNotEmpty() || fieldErrors.state.isNotEmpty()) advanced = true
            focusError = when {
                nameError.isNotBlank() -> "name"
                fieldErrors.host.isNotEmpty() -> "host"
                fieldErrors.user.isNotEmpty() -> "user"
                fieldErrors.port.isNotEmpty() -> "port"
                fieldErrors.pin.isNotEmpty() -> "pin"
                fieldErrors.key.isNotEmpty() -> "key"
                fieldErrors.binary.isNotEmpty() -> "binary"
                else -> "state"
            }
            return
        }
        val connection = Connection(host.trim(), port.trim().toInt(), user.trim(), pin.trim(), binary.trim(), state.trim())
        saveError = onSubmit(displayName, connection, connect).orEmpty()
    }
    val keyPicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) try {
            if (!canImport) {
                importError = "Disconnect before replacing the SSH key."
                return@rememberLauncherForActivityResult
            }
            val bytes = context.contentResolver.openInputStream(uri)?.use(::readImportedKey)
                ?: throw java.io.IOException()
            validateImportedKey(bytes)?.let { importError = it; return@rememberLauncherForActivityResult }
            try { onImportKey(bytes) } catch (failure: Exception) { bytes.fill(0); throw failure }
            importError = ""; fieldErrors = fieldErrors.copy(key = "")
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
    Column(Modifier.fillMaxSize().safeDrawingPadding().imePadding().padding(horizontal = 16.dp)) {
        val hostFocus = remember { FocusRequester() }
        val userFocus = remember { FocusRequester() }
        val portFocus = remember { FocusRequester() }
        val pinFocus = remember { FocusRequester() }
        val keyFocus = remember { FocusRequester() }
        val binaryFocus = remember { FocusRequester() }
        val stateFocus = remember { FocusRequester() }
        val nameFocus = remember { FocusRequester() }
        LaunchedEffect(focusError, advanced) {
            when (focusError) {
                "name" -> nameFocus
                "host" -> hostFocus
                "user" -> userFocus
                "port" -> portFocus
                "pin" -> pinFocus
                "key" -> keyFocus
                "binary" -> binaryFocus
                "state" -> stateFocus
                else -> null
            }?.requestFocus()
            focusError = null
        }
        Row(Modifier.fillMaxWidth().height(56.dp).semantics { contentDescription = "Host settings toolbar" }, verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = { leave(onBack) }) {
                Box(Modifier.rotate(180f)) { ChatGlyph(ChatIcon.ChevronRight, "Back") }
            }
            Text(if (newHost) "Add host" else "Host settings", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
            IconButton(onClick = { leave(onHome) }) { ChatGlyph(ChatIcon.Computer, "All hosts") }
        }
        val issue = saveError.ifBlank { connectionError }
        val canSave = !connecting
        if (showConnectionError && issue.isNotBlank()) {
            ConnectionIssueDialog(issue, onClose = { showConnectionError = false })
        }
        Column(Modifier.weight(1f).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            if (connecting) LinearProgressIndicator(Modifier.fillMaxWidth())
            OutlinedTextField(displayName, { displayName = it.take(80); nameError = "" }, label = { Text(if (newHost) "Display name (optional)" else "Host name") },
                singleLine = true, isError = nameError.isNotBlank(), supportingText = nameError.takeIf(String::isNotBlank)?.let { error -> { Text(error) } },
                modifier = Modifier.fillMaxWidth().focusRequester(nameFocus))
            OutlinedTextField(host, { host = it }, label = { Text("Hostname or IP") }, singleLine = true, isError = fieldErrors.host.isNotEmpty(), supportingText = fieldErrors.host.takeIf(String::isNotEmpty)?.let { error -> { Text(error) } }, modifier = Modifier.fillMaxWidth().focusRequester(hostFocus))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(user, { user = it }, label = { Text("SSH username") }, singleLine = true, isError = fieldErrors.user.isNotEmpty(), supportingText = fieldErrors.user.takeIf(String::isNotEmpty)?.let { error -> { Text(error) } }, modifier = Modifier.weight(2f).focusRequester(userFocus))
                OutlinedTextField(port, { port = it }, label = { Text("Port") }, singleLine = true, isError = fieldErrors.port.isNotEmpty(), supportingText = fieldErrors.port.takeIf(String::isNotEmpty)?.let { error -> { Text(error) } }, modifier = Modifier.weight(1f).focusRequester(portFocus),
                    keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Number))
            }
            OutlinedTextField(pin, { pin = it }, label = { Text("SSH fingerprint") }, placeholder = { Text("SHA256:…") }, singleLine = true, isError = fieldErrors.pin.isNotEmpty(), supportingText = { Text(fieldErrors.pin.ifBlank { "Paste the verified SHA256 host-key fingerprint." }) }, modifier = Modifier.fillMaxWidth().focusRequester(pinFocus))
            OutlinedButton(onClick = {
                // Returning from the picker must not auto-reconnect with the old key.
                beforePickKey()
                importError = ""
                keyPicker.launch(arrayOf("*/*"))
            }, enabled = canImport, modifier = Modifier.fillMaxWidth().focusRequester(keyFocus)) {
                Text(if (keyLoaded) "Replace SSH key" else "Import SSH key")
            }
            Text(importError.ifBlank { fieldErrors.key }.ifBlank { "Use a supported unencrypted private key." }, color = if (importError.isNotBlank() || fieldErrors.key.isNotBlank()) Mocha.Red else Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
            TextButton(onClick = { advanced = !advanced }) {
                ChatGlyph(if (advanced) ChatIcon.ChevronDown else ChatIcon.ChevronRight)
                Spacer(Modifier.width(6.dp))
                Text("Advanced settings")
            }
            if (advanced) {
                OutlinedTextField(binary, { binary = it }, label = { Text("Host binary") }, placeholder = { Text("~/.local/bin/modex-host") }, singleLine = true, isError = fieldErrors.binary.isNotEmpty(), supportingText = fieldErrors.binary.takeIf(String::isNotEmpty)?.let { error -> { Text(error) } }, modifier = Modifier.fillMaxWidth().focusRequester(binaryFocus))
                OutlinedTextField(state, { state = it }, label = { Text("State directory") }, placeholder = { Text("~/.local/state/modex-phone") }, singleLine = true, isError = fieldErrors.state.isNotEmpty(), supportingText = fieldErrors.state.takeIf(String::isNotEmpty)?.let { error -> { Text(error) } }, modifier = Modifier.fillMaxWidth().focusRequester(stateFocus))
                Text("Leave blank for defaults. Custom paths must be absolute.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
            }
            if (onDisconnect != null) TextButton(onClick = onDisconnect) { Text("Disconnect") }
        }
        if (issue.isNotBlank()) TextButton(onClick = { showConnectionError = true }) {
            Text(issue, maxLines = 2, overflow = TextOverflow.Ellipsis, color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
        }
        Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { submit(false) }, enabled = canSave, modifier = Modifier.weight(1f)) { Text("Save host") }
            Button(onClick = { submit(true) }, enabled = canSave, modifier = Modifier.weight(1f)) { Text("Connect") }
        }
    }
}
