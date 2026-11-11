package dev.modex

import java.io.InputStream
import java.io.OutputStream
import java.io.InterruptedIOException
import org.json.JSONObject

internal const val MAX_ATTACHMENT_BYTES = 20L * 1024 * 1024
internal const val MAX_ATTACHMENTS = 8

internal data class Attachment(val path: String, val name: String, val kind: String, val size: Long = 0) {
    fun toJson() = JSONObject().put("path", path).put("kind", kind)
}
internal data class RemoteFile(val name: String, val path: String, val directory: Boolean, val size: Long)
internal data class RemoteDirectory(val path: String, val parent: String?, val files: List<RemoteFile>, val truncated: Boolean)

internal fun attachmentName(value: String): String = value.replace('\\','/').substringAfterLast('/')
    .filter { !it.isISOControl() }.take(128).trim().takeUnless { it.isBlank() || it == "." || it == ".." } ?: "attachment"

internal fun attachmentKind(name: String): String = if (name.substringAfterLast('.').lowercase() in setOf("png","jpg","jpeg","gif","webp")) "image" else "file"

internal fun copyAttachment(input: InputStream, output: OutputStream, limit: Long = MAX_ATTACHMENT_BYTES, progress: (Long) -> Unit = {}): Long {
    val buffer = ByteArray(32768)
    var size = 0L
    while (true) {
        if (Thread.currentThread().isInterrupted) throw InterruptedIOException()
        val n = input.read(buffer)
        if (n < 0) break
        require(size + n <= limit) { "File exceeds 20 MiB." }
        output.write(buffer, 0, n)
        size += n
        progress(size)
    }
    return size
}

internal fun attachmentMessage(text: String, files: List<Attachment>): String =
    (listOf(text.takeIf { it.isNotBlank() }) + files.map { "[${if (it.kind == "image") "Image" else "File"}: ${it.name}]" }).filterNotNull().joinToString("\n")

// JSch expands globs in ls/stat/put/rename/chmod/rm, unlike mkdir/realpath.
internal fun literalSftpPath(path: String): String = buildString {
    path.forEach { char -> if (char == '\\' || char == '*' || char == '?') append('\\'); append(char) }
}

internal fun attachmentTextPreview(file: java.io.File): String? {
    val bytes = file.inputStream().use { input ->
        val buffer = ByteArray(256 * 1024 + 1)
        var size = 0
        while (size < buffer.size) { val n = input.read(buffer, size, buffer.size - size); if (n < 0) break; size += n }
        buffer.copyOf(size)
    }
    if (bytes.any { it == 0.toByte() }) return null
    val content = bytes.take(256 * 1024).toByteArray().decodeToString()
    if (content.any { it == '\uFFFD' || (it.isISOControl() && it !in "\n\r\t") }) return null
    return content + if (bytes.size > 256 * 1024) "\n[Preview limited to 256 KiB; use Open with for the complete file]" else ""
}
