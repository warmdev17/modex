package dev.modex

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.contentDescription
import androidx.core.app.NotificationManagerCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.LifecycleEventObserver
import androidx.compose.ui.platform.LocalLifecycleOwner

internal enum class NotificationSettingsAction { RequestPermission, OpenSystemSettings }

internal fun shouldOfferNotificationPermission(sdk: Int, seen: Boolean, manual: Boolean, connected: Boolean, granted: Boolean) =
    sdk >= 33 && !seen && manual && connected && !granted

internal fun notificationSettingsAction(sdk: Int, granted: Boolean, requested: Boolean, rationale: Boolean) =
    if (sdk >= 33 && !granted && (!requested || rationale)) NotificationSettingsAction.RequestPermission
    else NotificationSettingsAction.OpenSystemSettings

private fun notificationPermissionGranted(context: Context) =
    Build.VERSION.SDK_INT < 33 || context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

@Composable
internal fun ConnectionHome() {
    val context = LocalContext.current
    val app = context.applicationContext as ModexApplication
    val manager = app.connections
    val notificationPreferences = remember { context.getSharedPreferences("notification-permission", Context.MODE_PRIVATE) }
    var notificationAllowed by remember { mutableStateOf(notificationPermissionGranted(context)) }
    val notifications = remember(context) { NotificationManagerCompat.from(context) }
    var notificationEnabled by remember { mutableStateOf(notifications.areNotificationsEnabled()) }
    var showNotificationPrompt by rememberSaveable { mutableStateOf(false) }
    val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {
        notificationAllowed = notificationPermissionGranted(context)
        notificationEnabled = notifications.areNotificationsEnabled()
    }
    fun requestNotificationPermission() {
        notificationPreferences.edit().putBoolean("requested", true).apply()
        notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
    }
    fun openNotificationSettings() {
        context.startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
            .putExtra(Settings.EXTRA_APP_PACKAGE, context.packageName))
    }
    var page by rememberSaveable { mutableStateOf("Hosts") }
    var opened by rememberSaveable { mutableStateOf("") }
    var targetThread by rememberSaveable { mutableStateOf("") }
    var editor by rememberSaveable { mutableStateOf(false) }
    var create by rememberSaveable { mutableStateOf(false) }
    var delete by rememberSaveable { mutableStateOf("") }
    var issue by remember { mutableStateOf<Pair<HostProfile, String>?>(null) }
    var manualConnectHost by rememberSaveable { mutableStateOf("") }

    fun closeHost() {
        opened = ""; targetThread = ""; manualConnectHost = ""
    }

    fun returnToHosts() {
        closeHost()
        page = "Hosts"
    }

    fun connectHost(id: String, connection: Connection) {
        val vm = manager.controller(id)
        if (vm.connected || vm.connecting) return
        manualConnectHost = id
        vm.connect(connection)
    }

    manager.profiles.firstOrNull { it.id == manualConnectHost }?.let { host ->
        val vm = manager.controller(host.id)
        LaunchedEffect(host.id, vm.connecting, vm.connected, vm.connectFailure) {
            if (!vm.connecting) {
                if (shouldOfferNotificationPermission(Build.VERSION.SDK_INT,
                        notificationPreferences.getBoolean("seen", false), manual = true,
                        connected = vm.connected, granted = notificationAllowed)) {
                    notificationPreferences.edit().putBoolean("seen", true).apply()
                    showNotificationPrompt = true
                }
                manualConnectHost = ""
                if (!vm.connected && vm.connectFailure.isNotBlank()) {
                    returnToHosts()
                    issue = host to vm.connectFailure
                }
            }
        }
    }
    val owner = LocalLifecycleOwner.current
    DisposableEffect(owner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                notificationAllowed = notificationPermissionGranted(context)
                notificationEnabled = notifications.areNotificationsEnabled()
            }
            if (event == Lifecycle.Event.ON_START) manager.foreground()
        }
        owner.lifecycle.addObserver(observer)
        onDispose { owner.lifecycle.removeObserver(observer) }
    }
    val activities = manager.profiles.flatMap { host ->
        val vm = manager.controller(host.id)
        hostActivity(host.id, vm.runningThreads, vm.decisions, vm.titles)
    }.sortedWith(compareByDescending<HostActivity> { it.waiting > 0 }.thenBy { it.hostId }.thenBy { it.title })
    val connectedHosts = manager.profiles.filter { manager.controller(it.id).connected }.map { it.id }.toSet()
    val liveWaiting = activities.filter { it.hostId in connectedHosts }.sumOf { it.waiting }
    val profile = manager.profiles.firstOrNull { it.id == opened }
    if (create) {
        val draft: NewHostDraft = viewModel()
        val close = { draft.replaceKey(null); create = false }
        HostSettings(
            hostName = "", saved = null, keyLoaded = draft.key != null, newHost = true,
            onImportKey = draft::replaceKey, onBack = close, onHome = close,
            onSubmit = { name, connection, connect ->
                draft.key?.let { bytes ->
                    manager.add(name, connection, bytes).fold(
                        onSuccess = { id ->
                            close()
                            if (connect) {
                                editor = false
                                targetThread = ""
                                opened = id
                                connectHost(id, connection)
                            }
                            null
                        },
                        onFailure = { it.message },
                    )
                }
            },
        )
    } else if (profile != null) {
        val vm = manager.controller(profile.id)
        BackHandler { closeHost() }
        LaunchedEffect(profile.id, targetThread, vm.connected, vm.historyLoading) {
            if (targetThread.isNotBlank() && vm.connected && !vm.historyLoading) {
                val target = targetThread; targetThread = ""
                if (vm.selected != target) vm.select(target)
            }
        }
        key(profile.id) {
            HostWorkspace(vm, profile.name, editor, onHome = ::returnToHosts,
                onName = { manager.rename(profile.id, it) },
                onConnect = { connectHost(profile.id, it) })
        }
    } else Scaffold(
        modifier = Modifier.safeDrawingPadding(),
        containerColor = Mocha.Base,
        bottomBar = {
            NavigationBar(containerColor = Mocha.Mantle) {
                listOf("Hosts" to ChatIcon.Computer, "Activity" to ChatIcon.Activity, "Settings" to ChatIcon.Settings).forEach { (title, icon) ->
                    NavigationBarItem(selected = page == title, onClick = { page = title },
                        icon = { BadgedBox(badge = { if (title == "Activity" && liveWaiting > 0) Badge { Text(liveWaiting.toString()) } }) { ChatGlyph(icon, title) } }, label = { Text(title) },
                        colors = NavigationBarItemDefaults.colors(indicatorColor = Mocha.Surface, selectedIconColor = Mocha.Blue, selectedTextColor = Mocha.Blue))
                }
            }
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp)) {
            Row(Modifier.fillMaxWidth().height(56.dp).semantics { contentDescription = "Navigation toolbar" }, verticalAlignment = Alignment.CenterVertically) {
                Text(page, modifier = Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                if (page == "Hosts") FilledTonalButton(shape = RoundedCornerShape(10.dp), onClick = { create = true },
                    colors = ButtonDefaults.filledTonalButtonColors(containerColor = Mocha.Blue, contentColor = Mocha.Base)) {
                    ChatGlyph(ChatIcon.Attach); Spacer(Modifier.width(6.dp)); Text("Add host")
                }
            }
            if (manager.error.isNotBlank()) Text(manager.error, color = Mocha.Red, modifier = Modifier.padding(bottom = 12.dp))
            when (page) {
                "Activity" -> ActivityList(activities, manager.profiles.associate { it.id to it.name }, connectedHosts) { hostId, threadId ->
                    opened = hostId; targetThread = threadId; editor = false
                    val vm = manager.controller(hostId)
                    vm.savedConnection?.let { connectHost(hostId, it) }
                }
                "Settings" -> Column(Modifier.verticalScroll(rememberScrollState()).padding(bottom = 16.dp)) {
                    Surface(color = Mocha.Mantle, shape = RoundedCornerShape(12.dp)) {
                        Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f).padding(end = 12.dp)) {
                                Text("Reconnect on open", style = MaterialTheme.typography.titleSmall)
                                Text("Reconnect saved hosts when Modex returns to the foreground. Prompts and approvals are never resent automatically.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                            }
                            Switch(checked = manager.reconnectOnOpen, onCheckedChange = manager::reconnectOnOpen)
                        }
                    }
                    Surface(color = Mocha.Mantle, shape = RoundedCornerShape(12.dp), modifier = Modifier.padding(top = 12.dp)) {
                        Row(Modifier.fillMaxWidth().padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f).padding(end = 12.dp)) {
                                Text("Notifications", style = MaterialTheme.typography.titleSmall)
                                Text(if (notificationEnabled) "On · ongoing connections; tap to return to Modex"
                                else "Off · enable ongoing connection notifications and tap to return",
                                    color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                            }
                            TextButton(onClick = {
                                val rationale = Build.VERSION.SDK_INT >= 33 &&
                                    (context as? Activity)?.shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS) == true
                                when (notificationSettingsAction(Build.VERSION.SDK_INT, notificationAllowed,
                                    notificationPreferences.getBoolean("requested", false), rationale)) {
                                    NotificationSettingsAction.RequestPermission -> requestNotificationPermission()
                                    NotificationSettingsAction.OpenSystemSettings -> openNotificationSettings()
                                }
                            }) { Text(if (notificationEnabled) "Manage" else "Enable notifications") }
                        }
                    }
                    Text("Privacy", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 24.dp, bottom = 8.dp))
                    Text("Hosts, SSH keys and session state are encrypted on this device and excluded from backup. Removing a host does not delete files or conversations on the remote machine.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium)
                    val version = remember { context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty() }
                    Text("Modex $version", color = Mocha.Subtext, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(top = 24.dp))
                }
                else -> {
                    val visible = manager.profiles
                    if (visible.isEmpty()) Column(Modifier.fillMaxWidth().padding(vertical = 64.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        ChatGlyph(ChatIcon.Computer, color = Mocha.Subtext)
                        Text("Your computers, anywhere", style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 16.dp))
                        Text("Save a laptop or VPS to connect in one tap.", color = Mocha.Subtext, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp))
                    }
                    LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp), contentPadding = PaddingValues(bottom = 20.dp)) {
                        items(visible, key = { it.id }) { host ->
                            val vm = manager.controller(host.id)
                            var actions by remember { mutableStateOf(false) }
                            Surface(color = Mocha.Mantle, shape = RoundedCornerShape(12.dp)) {
                                Column(Modifier.fillMaxWidth().padding(16.dp)) {
                                    Row(Modifier.heightIn(min = 48.dp), verticalAlignment = Alignment.CenterVertically) {
                                        Surface(color = Mocha.Surface, shape = RoundedCornerShape(10.dp)) { Box(Modifier.size(44.dp), contentAlignment = Alignment.Center) { ChatGlyph(ChatIcon.Computer, color = if (vm.connected) Mocha.Blue else Mocha.Subtext) } }
                                        Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                                            Text(host.name, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                            Text(vm.savedConnection?.let { "${it.user}@${it.host}:${it.port}" } ?: "Finish setting up this host", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                        }
                                        val hasIssue = !vm.connecting && !vm.connected && vm.error.isNotBlank()
                                        val statusAction = if (hasIssue) Modifier.clickable(role = Role.Button) {
                                            issue = host to vm.error
                                        } else Modifier
                                        Box(Modifier.sizeIn(minWidth = 48.dp, minHeight = 48.dp).then(statusAction), contentAlignment = Alignment.CenterEnd) {
                                            when {
                                                vm.connecting -> CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                                                hasIssue -> Text("Issue", color = Mocha.Red, style = MaterialTheme.typography.labelSmall)
                                                else -> Text(if (vm.connected) "Online" else "Offline", color = if (vm.connected) Mocha.Blue else Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
                                            }
                                        }
                                    }
                                    if (vm.runningThreads.isNotEmpty() || vm.decisions.isNotEmpty()) Text("${vm.runningThreads.size} running · ${vm.decisions.size} waiting for input", color = Mocha.Blue, style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(top = 8.dp))
                                    Row(Modifier.fillMaxWidth().padding(top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                                        Box {
                                            IconButton(onClick = { actions = true }) { ChatGlyph(ChatIcon.More, "Manage ${host.name}") }
                                            ModexDropdownMenu(expanded = actions, onDismissRequest = { actions = false }) {
                                                DropdownMenuItem(text = { Text("Edit host") }, leadingIcon = { ChatGlyph(ChatIcon.Edit) }, onClick = { actions = false; opened = host.id; editor = true })
                                                DropdownMenuItem(text = { Text("Remove host", color = Mocha.Red) }, leadingIcon = { ChatGlyph(ChatIcon.Delete, color = Mocha.Red) }, onClick = { actions = false; delete = host.id })
                                            }
                                        }
                                        Spacer(Modifier.weight(1f))
                                        if (vm.connected || vm.connecting) TextButton(onClick = vm::disconnect) { Text("Disconnect") }
                                        Button(shape = RoundedCornerShape(10.dp), onClick = {
                                            editor = vm.savedConnection == null; targetThread = ""
                                            opened = host.id
                                            vm.savedConnection?.let { connectHost(host.id, it) }
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
    issue?.let { (host, message) ->
        ConnectionIssueDialog(
            message, onClose = { issue = null }, hostName = host.name,
            onEdit = { issue = null; opened = host.id; editor = true },
        )
    }
    if (delete.isNotBlank()) AlertDialog(onDismissRequest = { delete = "" }, title = { Text("Remove saved host?") },
        text = { Text("Disconnect this host and remove its saved key and session state from this device. Remote files and conversations are kept.") },
        confirmButton = { TextButton(onClick = { manager.remove(delete); if (opened == delete) opened = ""; delete = "" }) { Text("Remove", color = Mocha.Red) } },
        dismissButton = { TextButton(onClick = { delete = "" }) { Text("Cancel") } })
    if (showNotificationPrompt) AlertDialog(
        onDismissRequest = { showNotificationPrompt = false },
        title = { Text("Stay connected") },
        text = { Text("Notifications show ongoing host connections and let you tap to return to Modex.") },
        confirmButton = { TextButton(onClick = { showNotificationPrompt = false; requestNotificationPermission() }) { Text("Enable notifications") } },
        dismissButton = { TextButton(onClick = { showNotificationPrompt = false }) { Text("Not now") } },
    )
}
