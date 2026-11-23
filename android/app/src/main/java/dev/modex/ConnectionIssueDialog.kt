package dev.modex

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
internal fun ConnectionIssueDialog(
    message: String,
    onClose: () -> Unit,
    hostName: String? = null,
    onEdit: (() -> Unit)? = null,
) {
    AlertDialog(
        onDismissRequest = onClose,
        containerColor = Mocha.Mantle,
        titleContentColor = Mocha.Red,
        textContentColor = Mocha.Text,
        shape = MaterialTheme.shapes.large,
        icon = {
            Surface(color = Mocha.Red.copy(alpha = 0.14f), shape = CircleShape) {
                Box(Modifier.size(40.dp), contentAlignment = Alignment.Center) {
                    ChatGlyph(ChatIcon.Error, color = Mocha.Red)
                }
            }
        },
        title = { Text("Connection error") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (hostName != null) Text(
                    hostName, color = Mocha.Subtext, style = MaterialTheme.typography.labelLarge,
                    maxLines = 2, overflow = TextOverflow.Ellipsis,
                )
                Text(message)
            }
        },
        confirmButton = {
            if (onEdit != null) Button(onClick = onEdit, shape = RoundedCornerShape(10.dp)) { Text("Edit host") }
            else TextButton(onClick = onClose) { Text("Close") }
        },
        dismissButton = if (onEdit != null) ({ TextButton(onClick = onClose) { Text("Close") } }) else null,
    )
}
