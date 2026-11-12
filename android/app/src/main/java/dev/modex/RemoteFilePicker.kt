package dev.modex

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties

@Composable
internal fun RemoteFilePicker(initialPath: String, directory: RemoteDirectory?, working: Boolean, error: String, connected: Boolean, remaining: Int,
    onBrowse: (String) -> Unit, onAttach: (List<RemoteFile>) -> Unit, onDismiss: () -> Unit) {
    var path by rememberSaveable { mutableStateOf(initialPath) }
    var chosen by remember { mutableStateOf(emptyList<RemoteFile>()) }
    LaunchedEffect(directory?.path) { directory?.let { path = it.path } }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
      Surface(Modifier.fillMaxSize(), color = Mocha.Base) {
        Column(Modifier.fillMaxSize().safeDrawingPadding().padding(horizontal = 20.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close file browser") }
                Text("Connected computer", Modifier.weight(1f), style = MaterialTheme.typography.titleLarge)
            }
            OutlinedTextField(path, { path = it }, label = { Text("Folder path") }, singleLine = true, modifier = Modifier.fillMaxWidth(), enabled = connected && !working)
            Row(verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = { directory?.parent?.let(onBrowse) }, enabled = connected && !working && directory?.parent != null) { Text("Up") }
                TextButton(onClick = { onBrowse(path) }, enabled = connected && !working) { Text("Open folder") }
                if (working) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            }
            if (!connected) Text("Reconnect to browse files.", color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
            if (error.isNotEmpty()) Text(error, color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
            LazyColumn(Modifier.fillMaxWidth().weight(1f)) {
                if (directory?.files?.isEmpty() == true) item { Text("No regular files or folders here.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext) }
                items(directory?.files.orEmpty(), key = { it.path }) { file ->
                    val selected = chosen.any { it.path == file.path }
                    val enabled = connected && !working && (file.directory || (file.size <= MAX_ATTACHMENT_BYTES && (selected || chosen.size < remaining)))
                    Row(Modifier.fillMaxWidth().clickable(enabled = enabled) {
                        if (file.directory) onBrowse(file.path) else chosen = if (selected) chosen.filter { it.path != file.path } else chosen + file
                    }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                        ChatGlyph(if (file.directory) ChatIcon.Folder else if (attachmentKind(file.name) == "image") ChatIcon.Image else ChatIcon.File)
                        Column(Modifier.weight(1f).padding(start = 10.dp)) {
                            Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis, color = if (enabled) Mocha.Text else Mocha.Overlay)
                            if (!file.directory) Text(if (file.size > MAX_ATTACHMENT_BYTES) "Over 20 MiB limit" else "${file.size / 1024} KiB", style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
                        }
                        if (!file.directory) Checkbox(selected, onCheckedChange = null, enabled = enabled)
                    }
                }
            }
            if (directory?.truncated == true) Text("Showing the first 500 entries. Open a narrower folder.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
            Text("${chosen.size} of $remaining selected · 20 MiB per file", style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
            Button(onClick = { onAttach(chosen) }, enabled = connected && !working && chosen.isNotEmpty(), modifier = Modifier.fillMaxWidth(), shape = androidx.compose.foundation.shape.RoundedCornerShape(10.dp)) { Text("Attach (${chosen.size})") }
        }
      }
    }
}
