package dev.modex

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
internal fun SlashCommandPicker(
    commands: List<AndroidSlashCommand>, enabled: (AndroidSlashCommand) -> Boolean,
    onDismiss: () -> Unit, onSelect: (AndroidSlashCommand) -> Unit,
) {
    Surface(Modifier.fillMaxWidth().padding(bottom = 8.dp), shape = MaterialTheme.shapes.small,
        color = Mocha.Surface, contentColor = Mocha.Text, border = BorderStroke(1.dp, Mocha.Overlay)) {
        Column {
            Row(Modifier.fillMaxWidth().padding(start = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("Android commands", Modifier.weight(1f), style = MaterialTheme.typography.labelLarge)
                IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close command suggestions") }
            }
            if (commands.isEmpty()) Text("No matching command. Send it as a message.", Modifier.padding(12.dp), style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
            LazyColumn(Modifier.fillMaxWidth().heightIn(max = 200.dp)) {
                items(commands, key = { it.token }) { command ->
                    val available = enabled(command)
                    val color = if (command == AndroidSlashCommand.DELETE) Mocha.Red else Mocha.Text
                    DropdownMenuItem(
                        text = { Column {
                            Text("/${command.token}")
                            Text(command.description, style = MaterialTheme.typography.bodySmall,
                                color = if (available) Mocha.Subtext else LocalContentColor.current, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        } },
                        leadingIcon = { ChatGlyph(commandIcon(command)) },
                        colors = MenuDefaults.itemColors(textColor = color, leadingIconColor = color),
                        enabled = available, onClick = { onSelect(command) },
                    )
                }
            }
        }
    }
}

private fun commandIcon(command: AndroidSlashCommand): ChatIcon = when (command) {
    AndroidSlashCommand.HELP, AndroidSlashCommand.SKILLS -> ChatIcon.Skill
    AndroidSlashCommand.MODEL -> ChatIcon.ModelChoice
    AndroidSlashCommand.NEW -> ChatIcon.New
    AndroidSlashCommand.RESUME -> ChatIcon.Menu
    AndroidSlashCommand.FORK -> ChatIcon.Fork
    AndroidSlashCommand.ARCHIVE -> ChatIcon.Archive
    AndroidSlashCommand.DELETE -> ChatIcon.Delete
    AndroidSlashCommand.COPY -> ChatIcon.Copy
}
