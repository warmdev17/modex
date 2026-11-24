package dev.modex

import android.content.ContentValues
import android.content.Context
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import androidx.annotation.RequiresApi
import java.io.File
import java.io.OutputStream
import java.net.URLConnection
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext

internal fun attachmentMime(name: String): String = when (name.substringAfterLast('.', "").lowercase()) {
    "apk" -> "application/vnd.android.package-archive"
    else -> URLConnection.guessContentTypeFromName(name) ?: "application/octet-stream"
}

internal suspend fun <T> exportAttachment(
    source: File,
    create: () -> T,
    open: (T) -> OutputStream,
    publish: (T) -> Unit,
    cleanup: (T) -> Unit,
    checkCancellation: () -> Unit = {},
): T = withContext(Dispatchers.IO) {
    require(source.length() <= MAX_ATTACHMENT_BYTES) { "File exceeds 20 MiB." }
    val destination = create()
    try {
        checkCancellation()
        source.inputStream().use { input -> open(destination).use { output ->
            copyAttachment(input, output) { checkCancellation() }
        } }
        checkCancellation()
        publish(destination)
        destination
    } catch (failure: Throwable) {
        runCatching { cleanup(destination) }
        throw failure
    }
}

@RequiresApi(Build.VERSION_CODES.Q)
internal suspend fun saveAttachmentToDownloads(context: Context, source: File, name: String): Uri {
    val resolver = context.contentResolver
    val coroutine = currentCoroutineContext()
    return exportAttachment(source, {
        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, attachmentName(name))
            put(MediaStore.MediaColumns.MIME_TYPE, attachmentMime(name))
            put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
            put(MediaStore.MediaColumns.IS_PENDING, 1)
        }
        resolver.insert(MediaStore.Downloads.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY), values)
            ?: error("Downloads could not create the file.")
    }, { uri -> resolver.openOutputStream(uri, "w") ?: error("Downloads could not open the file.") }, { uri ->
        check(resolver.update(uri, ContentValues().apply { put(MediaStore.MediaColumns.IS_PENDING, 0) }, null, null) == 1) {
            "Downloads could not finish the file."
        }
    }, { uri -> resolver.delete(uri, null, null) }, { coroutine.ensureActive() })
}

internal suspend fun saveAttachmentToUri(context: Context, source: File, uri: Uri): Uri {
    val resolver = context.contentResolver
    val coroutine = currentCoroutineContext()
    return exportAttachment(source, { uri }, { target ->
        resolver.openOutputStream(target, "w") ?: error("The selected location could not open the file.")
    }, {}, { target -> resolver.delete(target, null, null) }, { coroutine.ensureActive() })
}
