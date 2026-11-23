package dev.modex

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import java.io.File

@Composable
internal fun ChatTranscriptEntry(
    entry: ChatEntry, showWork: Boolean,
    loadAttachment: suspend (Attachment) -> File, onPreview: (Attachment) -> Unit,
    onEdit: (() -> Unit)? = null,
) {
    MessageActions(entry.text, onEdit) {
        when (entry.role) {
            "You" -> Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                Surface(color = Mocha.Surface, shape = RoundedCornerShape(14.dp), modifier = Modifier.fillMaxWidth(0.9f)) {
                    Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        if (entry.attachments.isNotEmpty()) {
                            Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                entry.attachments.forEach { file -> AttachmentTile(file, loadAttachment, { onPreview(file) }) }
                            }
                        }
                        if (entry.text.isNotBlank()) Text(entry.text, style = MaterialTheme.typography.bodyLarge)
                        if (showWork && entry.status != "inProgress") WorkTimer(entry)
                        entry.notice?.let { notice ->
                            Text(notice.message, style = MaterialTheme.typography.bodySmall, color = if (notice.retrying) Mocha.Blue else Mocha.Subtext,
                                modifier = Modifier.semantics { contentDescription = "Turn notice: ${notice.message}" })
                        }
                    }
                }
            }
            "command", "web", "file_change" -> CommandCard(entry)
            else -> MarkdownMessage(entry.text, selectable = false)
        }
    }
}
