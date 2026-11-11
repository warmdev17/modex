package dev.modex

import android.content.ClipData
import android.content.Intent
import android.graphics.BitmapFactory
import android.webkit.MimeTypeMap
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
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

private suspend fun imagePreview(file: File, maxEdge: Int = 2048) = withContext(Dispatchers.IO) {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    BitmapFactory.decodeFile(file.path, bounds)
    require(bounds.outWidth > 0 && bounds.outHeight > 0)
    var sample = 1
    while (bounds.outWidth / sample > maxEdge || bounds.outHeight / sample > maxEdge) sample *= 2
    BitmapFactory.decodeFile(file.path, BitmapFactory.Options().apply { inSampleSize = sample })?.asImageBitmap()
}

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
internal fun AttachmentViewer(file: Attachment, load: suspend (Attachment) -> File, onDismiss: () -> Unit) {
    val context = LocalContext.current
    var revision by remember { mutableIntStateOf(0) }
    var local by remember(file.path) { mutableStateOf<File?>(null) }
    var image by remember(file.path) { mutableStateOf<androidx.compose.ui.graphics.ImageBitmap?>(null) }
    var text by remember(file.path) { mutableStateOf<AnnotatedString?>(null) }
    var error by remember(file.path) { mutableStateOf("") }
    var loading by remember(file.path) { mutableStateOf(true) }
    LaunchedEffect(file.path, revision) {
        loading = true; error = ""
        try {
            val downloaded = load(file)
            local = downloaded
            if (file.kind == "image") image = imagePreview(downloaded) ?: error("Unsupported image")
            else text = withContext(Dispatchers.IO) { attachmentTextPreview(downloaded)?.let { if (file.name.endsWith(".json", true)) jsonPreview(it) else AnnotatedString(it) } }
        } catch (_: Exception) { error = "Could not open this attachment. Reconnect and check the file still exists and is at most 20 MiB." }
        finally { loading = false }
    }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Surface(Modifier.fillMaxSize(), color = Mocha.Base) {
            Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close preview") }
                    Text(file.name, Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.titleMedium)
                    TextButton(enabled = local != null && !loading && error.isEmpty(), onClick = {
                        try {
                            val uri = FileProvider.getUriForFile(context, context.packageName + ".attachments", local!!)
                            val mime = MimeTypeMap.getSingleton().getMimeTypeFromExtension(file.name.substringAfterLast('.', "").lowercase()) ?: "application/octet-stream"
                            context.startActivity(Intent.createChooser(Intent(Intent.ACTION_VIEW).setDataAndType(uri, mime).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION).apply { clipData = ClipData.newRawUri(file.name, uri) }, "Open attachment"))
                        } catch (_: Exception) { error = "No compatible app could open this file. Install an app that supports this format." }
                    }) { Text("Open with") }
                }
                Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                    when {
                        loading -> CircularProgressIndicator()
                        error.isNotEmpty() -> Column(Modifier.padding(24.dp)) { Text(error, color = Mocha.Subtext); TextButton(onClick = { revision++ }) { Text("Retry") } }
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
                        else -> Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(24.dp)) { ChatGlyph(ChatIcon.File); Text(file.name, Modifier.padding(16.dp)); Text("Use Open with to view this file in another app.", color = Mocha.Subtext) }
                    }
                }
            }
        }
    }
}
