package dev.modex

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties

@Composable
internal fun RemoteFilePicker(
    initialPath: String,
    directory: RemoteDirectory?,
    working: Boolean,
    error: String,
    connected: Boolean,
    remaining: Int,
    onBrowse: (String) -> Unit,
    onAttach: (List<RemoteFile>) -> Unit,
    onDismiss: () -> Unit,
    onChooseDirectory: ((String) -> Unit)? = null,
    onCreateDirectory: ((String) -> Unit)? = null,
    retryPath: String = initialPath,
    onPreview: ((RemoteFile) -> Unit)? = null,
) {
    val folderMode = onChooseDirectory != null
    val files = directory?.files.orEmpty().filter { !folderMode || it.directory }
    var newFolder by rememberSaveable { mutableStateOf(false) }
    var folderName by rememberSaveable { mutableStateOf("") }
    var editingPath by rememberSaveable(initialPath) { mutableStateOf(false) }
    var path by rememberSaveable(initialPath) { mutableStateOf(initialPath) }
    var chosen by remember(initialPath) { mutableStateOf(emptyList<RemoteFile>()) }
    LaunchedEffect(directory?.path) { directory?.let { path = it.path } }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Surface(Modifier.fillMaxSize(), color = Mocha.Base) {
            Column(Modifier.fillMaxSize().safeDrawingPadding().padding(horizontal = 20.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(Modifier.fillMaxWidth().height(56.dp), verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close file browser") }
                    Text(if (folderMode) "Project folder" else "Connected computer", Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
                }
                Surface(
                    color = Mocha.Mantle,
                    shape = RoundedCornerShape(8.dp),
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                ) {
                    val currentPath = directory?.path ?: initialPath
                    val crumbs = currentPath.trim('/').split('/').filter(String::isNotEmpty)
                    val breadcrumbScroll = rememberScrollState()
                    LaunchedEffect(currentPath) { breadcrumbScroll.scrollTo(breadcrumbScroll.maxValue) }
                    Row(Modifier.padding(start = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                        Row(Modifier.weight(1f).horizontalScroll(breadcrumbScroll), verticalAlignment = Alignment.CenterVertically) {
                            if (directory != null && currentPath.startsWith('/')) {
                                TextButton(onClick = { onBrowse("/") }, enabled = connected && !working && crumbs.isNotEmpty()) { Text("/") }
                                var parentPath = ""
                                crumbs.forEachIndexed { index, name ->
                                    parentPath += "/$name"
                                    val destination = parentPath
                                    if (index > 0) ChatGlyph(ChatIcon.ChevronRight)
                                    TextButton(onClick = { onBrowse(destination) }, enabled = connected && !working && index < crumbs.lastIndex,
                                        modifier = Modifier.widthIn(max = 160.dp)) {
                                        Text(name, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    }
                                }
                            } else Text(currentPath, Modifier.padding(horizontal = 12.dp), maxLines = 1)
                        }
                        IconButton(onClick = { editingPath = true }, enabled = connected && !working) {
                            ChatGlyph(ChatIcon.Edit, "Edit path $currentPath")
                        }
                    }
                }
                if (editingPath) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedTextField(path, { path = it }, label = { Text("Folder path") }, singleLine = true, modifier = Modifier.weight(1f), enabled = connected && !working)
                        TextButton(onClick = { editingPath = false; onBrowse(path) }, enabled = connected && !working, modifier = Modifier.heightIn(min = 48.dp)) { Text("Go") }
                    }
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    TextButton(onClick = { directory?.parent?.let(onBrowse) }, enabled = connected && !working && directory?.parent != null, modifier = Modifier.heightIn(min = 48.dp)) { Text("Up") }
                    if (folderMode && onCreateDirectory != null) TextButton(onClick = { folderName = ""; newFolder = true }, enabled = connected && !working && directory != null, modifier = Modifier.heightIn(min = 48.dp)) { Text("New folder") }
                }
                Box(Modifier.fillMaxWidth().heightIn(min = 40.dp), contentAlignment = Alignment.CenterStart) {
                    when {
                        working -> Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            CircularProgressIndicator(Modifier.size(18.dp).semantics { contentDescription = "Loading files" }, strokeWidth = 2.dp)
                            Text("Loading files…", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
                        }
                        error.isNotEmpty() -> Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(error, Modifier.weight(1f), color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
                            TextButton(onClick = { onBrowse(retryPath) }, enabled = connected, modifier = Modifier.heightIn(min = 48.dp)) { Text("Retry") }
                        }
                        !connected -> Text("Reconnect to browse files.", color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
                    }
                }
                LazyColumn(Modifier.fillMaxWidth().weight(1f)) {
                    if (directory != null && files.isEmpty()) item { Text(if (folderMode) "No folders here." else "No regular files or folders here.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext) }
                    items(files, key = { it.path }) { file ->
                        val selected = chosen.any { it.path == file.path }
                        val selectEnabled = connected && !working && (file.directory || file.size <= MAX_ATTACHMENT_BYTES && (selected || chosen.size < remaining))
                        val previewEnabled = connected && !working && !file.directory && file.size <= MAX_ATTACHMENT_BYTES
                        val itemAction = if (file.directory) {
                            Modifier.clickable(enabled = selectEnabled, role = Role.Button) { onBrowse(file.path) }
                        } else {
                            Modifier.toggleable(selected, enabled = selectEnabled, role = Role.Checkbox) { checked ->
                                chosen = if (checked) chosen + file else chosen.filter { it.path != file.path }
                            }
                        }
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                            Row(Modifier.weight(1f).heightIn(min = 48.dp).then(itemAction).padding(start = 8.dp),
                                verticalAlignment = Alignment.CenterVertically) {
                                ChatGlyph(if (file.directory) ChatIcon.Folder else if (attachmentKind(file.name) == "image") ChatIcon.Image else ChatIcon.File)
                                Column(Modifier.weight(1f).padding(start = 10.dp)) {
                                    Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis, color = if (selectEnabled || previewEnabled) Mocha.Text else Mocha.Overlay)
                                    if (!file.directory) {
                                        Text(if (file.size > MAX_ATTACHMENT_BYTES) "Over 20 MiB limit" else "${file.size / 1024} KiB", style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
                                    }
                                }
                                if (!file.directory) Checkbox(selected, onCheckedChange = null, enabled = selectEnabled)
                            }
                            if (!file.directory && onPreview != null) {
                                IconButton(onClick = { onPreview(file) }, enabled = previewEnabled, modifier = Modifier.size(48.dp)) {
                                    ChatGlyph(if (attachmentKind(file.name) == "image") ChatIcon.Image else ChatIcon.SelectText, "Preview ${file.name}")
                                }
                            }
                        }
                    }
                }
                if (directory?.truncated == true) {
                    Text("Showing the first 500 entries. Open a narrower folder.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                }
                if (!folderMode) {
                    Text("${chosen.size} of $remaining selected · 20 MiB per file", style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
                }
                Button(
                    onClick = { if (folderMode) directory?.path?.let { onChooseDirectory?.invoke(it) } else onAttach(chosen) },
                    enabled = connected && !working && !editingPath && (if (folderMode) directory != null else chosen.isNotEmpty()),
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(10.dp),
                ) { Text(if (folderMode) "Use this folder" else "Attach (${chosen.size})") }
            }
        }
    }
    if (newFolder) AlertDialog(
        onDismissRequest = { newFolder = false }, title = { Text("New folder") },
        text = { OutlinedTextField(folderName, { folderName = it }, label = { Text("Folder name") }, singleLine = true) },
        confirmButton = { TextButton(enabled = connected && !working && validDirectoryName(folderName), onClick = { onCreateDirectory?.invoke(folderName); newFolder = false }) { Text("Create folder") } },
        dismissButton = { TextButton(onClick = { newFolder = false }) { Text("Cancel") } },
    )
}
