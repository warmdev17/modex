package dev.modex

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.*
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import kotlinx.coroutines.delay

internal fun copyMessage(context: Context, text: String) {
    (context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText("Message", text))
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun MessageActions(text: String, onEdit: (() -> Unit)? = null, content: @Composable () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    var selection by remember { mutableStateOf<String?>(null) }
    val context = LocalContext.current
    val haptic = LocalHapticFeedback.current
    Box(Modifier.fillMaxWidth().combinedClickable(interactionSource = remember { MutableInteractionSource() }, indication = if (onEdit != null) null else LocalIndication.current, onClick = {}, onLongClickLabel = "Message actions", onLongClick = { haptic.performHapticFeedback(HapticFeedbackType.LongPress); menu = true })) {
        content()
        ModexDropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            if (onEdit != null) DropdownMenuItem(text = { Text("Edit") }, leadingIcon = { ChatGlyph(ChatIcon.Edit) }, onClick = { menu = false; onEdit() })
            DropdownMenuItem(text = { Text("Copy") }, leadingIcon = { ChatGlyph(ChatIcon.Copy) }, onClick = { copyMessage(context, text); menu = false })
            DropdownMenuItem(text = { Text("Select text") }, leadingIcon = { ChatGlyph(ChatIcon.SelectText) }, onClick = { selection = text; menu = false })
        }
    }
    selection?.let { snapshot ->
        Dialog(onDismissRequest = { selection = null }, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
            Surface(Modifier.fillMaxSize(), color = Mocha.Base) {
                Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                    Row(Modifier.fillMaxWidth().padding(8.dp)) {
                        IconButton(onClick = { selection = null }) { ChatGlyph(ChatIcon.Close, "Close text selection") }
                        Text("Select text", modifier = Modifier.weight(1f).padding(12.dp), style = MaterialTheme.typography.titleMedium)
                        TextButton(onClick = { copyMessage(context, snapshot) }) { Text("Copy all") }
                    }
                    HorizontalDivider(color = Mocha.Surface)
                    SelectionContainer { Text(snapshot, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp)) }
                }
            }
        }
    }
}

@Composable
internal fun WorkTimer(entry: ChatEntry, action: String? = null, modifier: Modifier = Modifier) {
    var now by remember(entry.id) { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(entry.id, entry.status) { while (entry.status == "inProgress") { now = System.currentTimeMillis(); delay(1000) } }
    val duration = if (entry.status == "inProgress") entry.startedAtMs?.let { now - it } else entry.durationMs
    if (action != null) Row(modifier.semantics(mergeDescendants = true) {
        contentDescription = "Turn activity: $action" + (duration?.let { " (${formatWorkDuration(it)})" } ?: "")
    }, verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("• $action", modifier = Modifier.weight(1f, fill = false), maxLines = 1, overflow = TextOverflow.Ellipsis, color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
        if (duration != null) Text("(${formatWorkDuration(duration)})", color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
    } else if (entry.status.isNotBlank()) Text(workStatusLabel(entry.status) + (duration?.let { " · ${formatWorkDuration(it)}" } ?: ""), color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
}
