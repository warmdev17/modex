package dev.modex

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.compose.ui.platform.LocalLifecycleOwner

@Composable
internal fun ConnectionHome() {
    val app = LocalContext.current.applicationContext as ModexApplication
    val manager = app.connections
    var page by rememberSaveable { mutableStateOf("Hosts") }
    var opened by rememberSaveable { mutableStateOf("") }
    var editor by rememberSaveable { mutableStateOf(false) }
    var create by rememberSaveable { mutableStateOf(false) }
    var newName by rememberSaveable { mutableStateOf("") }
    var delete by rememberSaveable { mutableStateOf("") }
    val owner = LocalLifecycleOwner.current
    DisposableEffect(owner) {
        val observer = LifecycleEventObserver { _, event -> if (event == Lifecycle.Event.ON_START) manager.foreground() }
        owner.lifecycle.addObserver(observer)
        onDispose { owner.lifecycle.removeObserver(observer) }
    }
    val profile = manager.profiles.firstOrNull { it.id == opened }
    if (profile != null) {
        BackHandler { opened = "" }
        key(profile.id) {
            HostWorkspace(manager.controller(profile.id), profile.name, editor,
                onHome = { opened = "" }, onName = { manager.rename(profile.id, it) },
                onForget = { delete = profile.id })
        }
    } else Scaffold(
        modifier = Modifier.safeDrawingPadding(),
        containerColor = Mocha.Base,
        bottomBar = {
            NavigationBar(containerColor = Mocha.Mantle) {
                listOf("Hosts" to ChatIcon.Computer, "Connections" to ChatIcon.Terminal, "Settings" to ChatIcon.Settings).forEach { (title, icon) ->
                    NavigationBarItem(selected = page == title, onClick = { page = title },
                        icon = { ChatGlyph(icon, title) }, label = { Text(title) },
                        colors = NavigationBarItemDefaults.colors(indicatorColor = Mocha.Surface, selectedIconColor = Mocha.Blue, selectedTextColor = Mocha.Blue))
                }
            }
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(horizontal = 20.dp)) {
            Row(Modifier.fillMaxWidth().padding(top = 24.dp, bottom = 20.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("Modex", color = Mocha.Subtext, style = MaterialTheme.typography.labelLarge)
                    Text(page, style = MaterialTheme.typography.headlineLarge, fontWeight = FontWeight.SemiBold)
                }
                if (page == "Hosts") FilledTonalButton(shape = RoundedCornerShape(10.dp), onClick = { newName = ""; create = true },
                    colors = ButtonDefaults.filledTonalButtonColors(containerColor = Mocha.Blue, contentColor = Mocha.Base)) {
                    ChatGlyph(ChatIcon.Attach); Spacer(Modifier.width(6.dp)); Text("Add host")
                }
            }
            if (manager.error.isNotBlank()) Text(manager.error, color = Mocha.Red, modifier = Modifier.padding(bottom = 12.dp))
            when (page) {
                "Settings" -> {
                    Surface(color = Mocha.Mantle, shape = RoundedCornerShape(12.dp)) {
                        Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f).padding(end = 12.dp)) {
                                Text("Reconnect on open", style = MaterialTheme.typography.titleSmall)
                                Text("Reconnect saved hosts when Modex returns to the foreground. Prompts and approvals are never resent automatically.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                            }
                            Switch(checked = manager.reconnectOnOpen, onCheckedChange = manager::reconnectOnOpen)
                        }
                    }
                    Text("Privacy", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 24.dp, bottom = 8.dp))
                    Text("Hosts, SSH keys and session state are encrypted on this device and excluded from backup. Removing a host does not delete files or conversations on the remote machine.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium)
                    val context = LocalContext.current
                    val version = remember { context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty() }
                    Text("Modex $version", color = Mocha.Subtext, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(top = 24.dp))
                }
                else -> {
                    val visible = manager.profiles.filter { page == "Hosts" || manager.controller(it.id).let { vm -> vm.connected || vm.connecting } }
                    if (visible.isEmpty()) Column(Modifier.fillMaxWidth().padding(vertical = 64.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        ChatGlyph(ChatIcon.Computer, color = Mocha.Subtext)
                        Text(if (page == "Hosts") "Your computers, anywhere" else "No active connections", style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 16.dp))
                        Text(if (page == "Hosts") "Save a laptop or VPS to connect in one tap." else "Connect a saved host to start working.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp))
                    }
                    LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp), contentPadding = PaddingValues(bottom = 20.dp)) {
                        items(visible, key = { it.id }) { host ->
                            val vm = manager.controller(host.id)
                            var actions by remember { mutableStateOf(false) }
                            Surface(color = Mocha.Mantle, shape = RoundedCornerShape(12.dp)) {
                                Column(Modifier.fillMaxWidth().padding(16.dp)) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Surface(color = Mocha.Surface, shape = RoundedCornerShape(10.dp)) { Box(Modifier.size(44.dp), contentAlignment = Alignment.Center) { ChatGlyph(ChatIcon.Computer, color = if (vm.connected) Mocha.Blue else Mocha.Subtext) } }
                                        Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                                            Text(host.name, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                            Text(vm.savedConnection?.let { "${it.user}@${it.host}:${it.port}" } ?: "Finish setting up this host", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                        }
                                        if (vm.connecting) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                                        else Text(if (vm.connected) "Online" else "Offline", color = if (vm.connected) Mocha.Blue else Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
                                    }
                                    if (vm.error.isNotBlank()) Text(vm.error, color = Mocha.Red, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 10.dp))
                                    if (vm.runningThreads.isNotEmpty() || vm.decisions.isNotEmpty()) Text("${vm.runningThreads.size} running · ${vm.decisions.size} waiting for input", color = Mocha.Blue, style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(top = 8.dp))
                                    Row(Modifier.fillMaxWidth().padding(top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                                        Box {
                                            IconButton(onClick = { actions = true }) { ChatGlyph(ChatIcon.More, "Manage ${host.name}") }
                                            DropdownMenu(expanded = actions, onDismissRequest = { actions = false }) {
                                                DropdownMenuItem(text = { Text("Edit host") }, onClick = { actions = false; opened = host.id; editor = true })
                                                DropdownMenuItem(text = { Text("Remove host", color = Mocha.Red) }, onClick = { actions = false; delete = host.id })
                                            }
                                        }
                                        Spacer(Modifier.weight(1f))
                                        if (vm.connected || vm.connecting) TextButton(onClick = vm::disconnect) { Text("Disconnect") }
                                        Button(shape = RoundedCornerShape(10.dp), onClick = {
                                            editor = vm.savedConnection == null
                                            opened = host.id
                                            if (!vm.connected && !vm.connecting) vm.savedConnection?.let(vm::connect)
                                        }) { Text(if (vm.connected || vm.connecting) "Open" else if (vm.savedConnection == null) "Set up" else "Connect") }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    if (create) AlertDialog(onDismissRequest = { create = false }, title = { Text("Add host") },
        text = { OutlinedTextField(newName, { newName = it.take(80) }, label = { Text("Name") }, placeholder = { Text("Laptop or VPS") }, singleLine = true) },
        confirmButton = { TextButton(enabled = newName.isNotBlank(), onClick = {
            manager.add(newName)?.let { opened = it; editor = true; create = false }
        }) { Text("Continue") } }, dismissButton = { TextButton(onClick = { create = false }) { Text("Cancel") } })
    if (delete.isNotBlank()) AlertDialog(onDismissRequest = { delete = "" }, title = { Text("Remove saved host?") },
        text = { Text("Disconnect this host and remove its saved key and session state from this device. Remote files and conversations are kept.") },
        confirmButton = { TextButton(onClick = { manager.remove(delete); if (opened == delete) opened = ""; delete = "" }) { Text("Remove", color = Mocha.Red) } },
        dismissButton = { TextButton(onClick = { delete = "" }) { Text("Cancel") } })
}
