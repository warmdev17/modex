package dev.modex

import android.content.ContentResolver
import android.net.Uri
import android.provider.OpenableColumns
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Draft metadata stays in the app ViewModel, scoped to its host and originating thread. */
internal class AttachmentState(private val scope: CoroutineScope, private val transport: () -> SshTransport?, private val connection: () -> Connection?) {
    var drafts by mutableStateOf(mapOf<String,List<Attachment>>()); private set
    var directory by mutableStateOf<RemoteDirectory?>(null); private set
    var working by mutableStateOf(false); private set
    var error by mutableStateOf(""); private set
    var progress by mutableStateOf(""); private set
    var generation = 0; private set

    fun clear() { generation++; drafts = emptyMap(); directory = null; error = ""; progress = ""; working = false }
    fun remove(thread: String, path: String) { drafts = drafts + (thread to drafts[thread].orEmpty().filter { it.path != path }) }
    fun sent(thread: String, paths: Set<String>) { drafts = drafts + (thread to drafts[thread].orEmpty().filter { it.path !in paths }) }
    fun addRemote(thread: String, file: RemoteFile) {
        if (working || thread.isBlank() || file.directory) return
        if (file.size > MAX_ATTACHMENT_BYTES) { error = "Select files up to 20 MiB."; return }
        append(thread, Attachment(file.path, file.name, attachmentKind(file.name), file.size))
    }
    private fun append(thread: String, file: Attachment) {
        val existing = drafts[thread].orEmpty()
        if (existing.any { it.path == file.path }) return
        if (existing.size >= MAX_ATTACHMENTS) { error = "Attach up to 8 files per message."; return }
        drafts = drafts + (thread to (existing + file)); error = ""
    }
    fun browse(path: String) {
        if (working) return
        val current = transport() ?: return
        val revision = generation
        directory = null; working = true; error = ""; progress = "Loading files…"
        scope.launch {
            try {
                val result = withContext(Dispatchers.IO) { current.listRemoteDirectory(path) }
                if (generation == revision && transport() === current) directory = result
            } catch (_: Exception) {
                if (generation == revision) error = "Could not open this folder. Check its path, read permission and SFTP availability."
            } finally { if (generation == revision) { working = false; progress = "" } }
        }
    }
    fun fromPhone(thread: String, uris: List<Uri>, revision: Int, resolver: ContentResolver) {
        if (revision != generation || working || thread.isBlank() || uris.isEmpty()) return
        if (drafts[thread].orEmpty().size + uris.size > MAX_ATTACHMENTS) { error = "Attach up to 8 files per message."; return }
        val current = transport() ?: return
        val host = connection() ?: return
        working = true; error = ""; progress = "Preparing upload…"
        scope.launch {
            try {
                for (uri in uris) {
                    val file = withContext(Dispatchers.IO) {
                        var name = "attachment"
                        resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use { cursor ->
                            if (cursor.moveToFirst()) {
                                val n = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                                if (n >= 0 && !cursor.isNull(n)) name = attachmentName(cursor.getString(n))
                                val size = cursor.getColumnIndex(OpenableColumns.SIZE)
                                require(size < 0 || cursor.isNull(size) || cursor.getLong(size) <= MAX_ATTACHMENT_BYTES)
                            }
                        }
                        val extension = when (resolver.getType(uri)) { "image/png" -> "png"; "image/jpeg" -> "jpg"; "image/gif" -> "gif"; "image/webp" -> "webp"; else -> null }
                        if (extension != null && attachmentKind(name) != "image") name += ".$extension"
                        var lastUpdate = 0L
                        resolver.openInputStream(uri)?.use { input ->
                            current.uploadAttachment(host.state, name, input) { bytes ->
                                val now = System.currentTimeMillis()
                                if (now - lastUpdate >= 150) {
                                    lastUpdate = now
                                    scope.launch { if (generation == revision && working) progress = "Uploading $name · ${bytes / 1024} KiB" }
                                }
                            }
                        } ?: error("Document unavailable")
                    }
                    if (generation != revision || transport() !== current) return@launch
                    append(thread, file)
                }
            } catch (_: Exception) {
                if (generation == revision) error = "Upload stopped. Check SFTP access and the 20 MiB file limit. Select the file again to retry."
            } finally { if (generation == revision) { working = false; progress = "" } }
        }
    }
}
