package dev.modex

import android.content.ClipData
import android.app.Activity
import android.app.DownloadManager
import android.content.Intent
import android.graphics.BitmapFactory
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.horizontalScroll
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.core.content.FileProvider
import java.io.File
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private suspend fun imagePreview(file: File, maxEdge: Int = 2048) = withContext(Dispatchers.IO) {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    BitmapFactory.decodeFile(file.path, bounds)
    require(bounds.outWidth > 0 && bounds.outHeight > 0)
    var sample = 1
    while (bounds.outWidth / sample > maxEdge || bounds.outHeight / sample > maxEdge) sample *= 2
    BitmapFactory.decodeFile(file.path, BitmapFactory.Options().apply { inSampleSize = sample })?.asImageBitmap()
}

internal val AttachmentPreviewStateSaver = listSaver<MutableState<Attachment?>, Any>(
    save = { state -> state.value?.let { listOf(it.path, it.name, it.kind, it.size) }.orEmpty() },
    restore = { saved -> mutableStateOf(saved.takeIf { it.isNotEmpty() }?.let { Attachment(it[0] as String, it[1] as String, it[2] as String, it[3] as Long) }) },
)

internal data class PendingAttachmentExport(
    val host: String,
    val attachment: Attachment,
    val sourcePath: String,
    val mime: String,
) {
    fun sourceFor(host: String, attachment: Attachment): File? =
        sourcePath.takeIf { this.host == host && this.attachment == attachment }?.let(::File)
}

private val PendingAttachmentExportStateSaver = listSaver<MutableState<PendingAttachmentExport?>, Any>(
    save = { state -> state.value?.let { listOf(it.host, it.attachment.path, it.attachment.name, it.attachment.kind, it.attachment.size, it.sourcePath, it.mime) }.orEmpty() },
    restore = { saved -> mutableStateOf(saved.takeIf { it.isNotEmpty() }?.let {
        PendingAttachmentExport(it[0] as String, Attachment(it[1] as String, it[2] as String, it[3] as String, it[4] as Long), it[5] as String, it[6] as String)
    }) },
)

@Composable
internal fun AttachmentTile(file: Attachment, load: suspend (Attachment) -> File, onOpen: () -> Unit, onRemove: (() -> Unit)? = null) {
    val image by produceState<androidx.compose.ui.graphics.ImageBitmap?>(null, file.path, file.kind) {
        if (file.kind == "image") value = runCatching { imagePreview(load(file), 384) }.getOrNull()
    }
    Box {
        Surface(onClick = onOpen, shape = RoundedCornerShape(10.dp), color = Mocha.Surface, modifier = Modifier.width(if (file.kind == "image") 128.dp else 184.dp)) {
            Column {
                if (file.kind == "image") Box(Modifier.fillMaxWidth().height(104.dp), contentAlignment = Alignment.Center) {
                    image?.let { Image(it, "Preview ${file.name}", Modifier.fillMaxSize(), contentScale = ContentScale.Crop) } ?: ChatGlyph(ChatIcon.Image)
                }
                Row(Modifier.padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                    if (file.kind != "image") { ChatGlyph(ChatIcon.File); Spacer(Modifier.width(8.dp)) }
                    Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.labelMedium, modifier = Modifier.weight(1f).padding(end = if (onRemove != null && file.kind != "image") 28.dp else 0.dp))
                }
            }
        }
        if (onRemove != null) IconButton(onClick = onRemove, colors = IconButtonDefaults.iconButtonColors(containerColor = Mocha.Mantle.copy(alpha = 0.92f), contentColor = Mocha.Text), modifier = Modifier.align(Alignment.TopEnd).size(48.dp).padding(6.dp)) { ChatGlyph(ChatIcon.Close, "Remove ${file.name}") }
    }
}

@Composable
internal fun AttachmentViewer(file: Attachment, load: suspend (Attachment) -> File, host: String = "", onDismiss: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var revision by remember { mutableIntStateOf(0) }
    var localPath by rememberSaveable(host, file.path, file.name, file.size) { mutableStateOf("") }
    var image by remember(file.path) { mutableStateOf<androidx.compose.ui.graphics.ImageBitmap?>(null) }
    var text by remember(file.path) { mutableStateOf<AnnotatedString?>(null) }
    var loadError by remember(file.path) { mutableStateOf("") }
    var previewError by remember(file.path) { mutableStateOf("") }
    var actionError by rememberSaveable(host, file.path, file.name, file.size) { mutableStateOf("") }
    var loading by remember(file.path) { mutableStateOf(true) }
    var saving by remember(file.path) { mutableStateOf(false) }
    var savedUri by rememberSaveable(host, file.path, file.name, file.size) { mutableStateOf("") }
    var savedMime by rememberSaveable(host, file.path, file.name, file.size) { mutableStateOf("") }
    var savedInDownloads by rememberSaveable(host, file.path, file.name, file.size) { mutableStateOf(false) }
    var pending by rememberSaveable(host, file.path, file.name, file.size, saver = PendingAttachmentExportStateSaver) {
        mutableStateOf<PendingAttachmentExport?>(null)
    }
    val local = localPath.takeIf { it.isNotBlank() }?.let(::File)

    fun openUri(uri: android.net.Uri, mime: String, chooser: Boolean = false) {
        val view = Intent(Intent.ACTION_VIEW).setDataAndType(uri, mime)
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            .apply { clipData = ClipData.newRawUri(file.name, uri) }
        context.startActivity(if (chooser) Intent.createChooser(view, "Open attachment") else view)
    }
    fun save(source: File, mime: String, inDownloads: Boolean, write: suspend () -> android.net.Uri) {
        scope.launch {
            saving = true; actionError = ""
            try {
                val uri = write()
                savedUri = uri.toString(); savedMime = context.contentResolver.getType(uri) ?: mime; savedInDownloads = inDownloads
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { actionError = "Could not save this attachment. Check the destination has space and try again." }
            finally { saving = false }
        }
    }
    val saveAs = rememberLauncherForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        val uri = result.data?.data.takeIf { result.resultCode == Activity.RESULT_OK }
        val staged = pending
        pending = null
        val source = staged?.sourceFor(host, file)
        if (uri != null && source != null) save(source, staged.mime, false) { saveAttachmentToUri(context, source, uri) }
    }
    LaunchedEffect(file.path, revision) {
        loading = true; loadError = ""; previewError = ""; image = null; text = null
        try {
            val downloaded = load(file)
            localPath = downloaded.absolutePath
            if (file.kind == "image") {
                image = try { imagePreview(downloaded) }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (_: Exception) { null }
                if (image == null) previewError = "A preview is not available, but the original file can still be saved or opened."
            } else text = withContext(Dispatchers.IO) { attachmentTextPreview(downloaded)?.let { if (file.name.endsWith(".json", true)) jsonPreview(it) else AnnotatedString(it) } }
        } catch (cancelled: CancellationException) { throw cancelled }
        catch (_: Exception) { loadError = "Could not load this attachment. Reconnect and check the file still exists and is at most 20 MiB." }
        finally { loading = false }
    }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Surface(Modifier.fillMaxSize(), color = Mocha.Base) {
            Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close preview") }
                    Text(file.name, Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.titleMedium)
                    TextButton(enabled = local != null && !loading && !saving && pending == null, onClick = {
                        val source = local ?: return@TextButton
                        val mime = attachmentMime(file.name)
                        if (Build.VERSION.SDK_INT >= 29) save(source, mime, true) { saveAttachmentToDownloads(context, source, file.name) }
                        else {
                            pending = PendingAttachmentExport(host, file, source.absolutePath, mime)
                            try { saveAs.launch(Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(mime).putExtra(Intent.EXTRA_TITLE, attachmentName(file.name))) }
                            catch (_: Exception) { pending = null; actionError = "No document picker is available. Try Open with instead." }
                        }
                    }) { Text(if (Build.VERSION.SDK_INT >= 29) "Save to Downloads" else "Save as…") }
                    TextButton(enabled = local != null && !loading && !saving, onClick = {
                        try {
                            val uri = FileProvider.getUriForFile(context, context.packageName + ".attachments", local!!)
                            openUri(uri, attachmentMime(file.name), true)
                        } catch (_: Exception) { actionError = "No compatible app could open this file. Save it and try from Downloads." }
                    }) { Text("Open with") }
                }
                if (saving) Row(Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    Text(if (Build.VERSION.SDK_INT >= 29) "Saving to Downloads…" else "Saving…", Modifier.padding(start = 12.dp), color = Mocha.Subtext)
                }
                if (savedUri.isNotBlank() && !saving) Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(if (savedInDownloads) "Saved to Downloads" else "Saved", Modifier.weight(1f), color = Mocha.Blue, style = MaterialTheme.typography.bodySmall)
                    TextButton(onClick = {
                        try { openUri(android.net.Uri.parse(savedUri), savedMime) }
                        catch (_: Exception) { actionError = if (savedInDownloads) "Saved successfully. Open the Downloads app to find the file." else "Saved successfully, but no compatible app could open it." }
                    }) { Text("Open") }
                    if (savedInDownloads) TextButton(onClick = {
                        try { context.startActivity(Intent(DownloadManager.ACTION_VIEW_DOWNLOADS)) }
                        catch (_: Exception) { actionError = "Saved successfully. Open your file manager and choose Downloads." }
                    }) { Text("Downloads") }
                }
                if (actionError.isNotBlank()) Text(actionError, Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 6.dp), color = Mocha.Red, style = MaterialTheme.typography.bodySmall)
                Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                    when {
                        loading -> CircularProgressIndicator()
                        loadError.isNotEmpty() && local == null -> Column(Modifier.padding(24.dp)) { Text(loadError, color = Mocha.Subtext); TextButton(onClick = { revision++ }) { Text("Retry") } }
                        image != null -> {
                            var scale by remember(file.path) { mutableFloatStateOf(1f) }
                            var offset by remember(file.path) { mutableStateOf(androidx.compose.ui.geometry.Offset.Zero) }
                            Box(Modifier.fillMaxSize().clip(RoundedCornerShape(0.dp)).pointerInput(file.path) {
                                detectTransformGestures { _, pan, zoom, _ -> scale = (scale * zoom).coerceIn(1f, 5f); offset = if (scale == 1f) androidx.compose.ui.geometry.Offset.Zero else offset + pan }
                            }) {
                                Image(image!!, file.name, Modifier.fillMaxSize().graphicsLayer { scaleX = scale; scaleY = scale; translationX = offset.x; translationY = offset.y }, contentScale = ContentScale.Fit)
                            }
                        }
                        text != null -> SelectionContainer(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp)) { Text(text!!, modifier = Modifier.horizontalScroll(rememberScrollState()), style = MaterialTheme.typography.bodyMedium, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace) }
                        else -> Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(24.dp)) { ChatGlyph(ChatIcon.File); Text(file.name, Modifier.padding(16.dp)); Text(previewError.ifBlank { "Use Save or Open with to view this file in another app." }, color = Mocha.Subtext) }
                    }
                }
            }
        }
    }
}
