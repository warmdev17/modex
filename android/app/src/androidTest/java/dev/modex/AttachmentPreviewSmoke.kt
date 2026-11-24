package dev.modex

import android.app.Instrumentation
import android.content.ContentUris
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Color
import android.os.Bundle
import android.os.Build
import android.provider.MediaStore
import android.net.Uri
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.runtime.*
import androidx.compose.material3.Surface
import java.io.File

/** Preview and export smoke from isolated app-cache files, without a host connection. */
internal fun Instrumentation.attachmentPreviewSmoke() {
    val result = Bundle()
    val root = File(targetContext.cacheDir, "attachment-preview/smoke").apply { mkdirs() }
    val ownedDownloads = mutableListOf<Uri>()
    try {
        val id = System.currentTimeMillis()
        val picture = File(root, "sample-$id.png")
        val bitmap = Bitmap.createBitmap(64, 48, Bitmap.Config.ARGB_8888).apply { eraseColor(Color.RED) }
        picture.outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }; bitmap.recycle()
        val note = File(root, "notes-$id.json").apply { writeText("""{"status":"readable on the phone","count":1234567890123456789,"active":true,"missing":null}""") }
        val apk = File(root, "fixture-$id.apk").apply { writeBytes(byteArrayOf(0, 1, 2, 3, 127, -1)) }
        var files by mutableStateOf(listOf(Attachment(picture.path,picture.name,"image"), Attachment(note.path,note.name,"file")))
        val activity = startActivitySync(Intent(targetContext,MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        runOnMainSync { activity.setContent { ModexTheme { Surface {
            ChatWorkspace(host = "preview", connected = true, connecting = false, error = "", sessionModel = "fixture", selected = "t", threads = listOf("t"), titles = emptyMap(),
                messages = listOf(ChatEntry("u", "You", "Attached earlier", attachments = files)), decisions = emptyList(), busy = false, historyLoading = false,
                hasMore = false, sessionSettings = "", answeringDecisionIds = emptySet(), onMore = {}, onSelect = {}, onCreate = { _, _, _ -> }, onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onRefresh = {}, onConnection = {}, onDisconnect = {},
                attachments = files, loadAttachment = { File(it.path) })
        } } } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun waitNode(label: String): AccessibilityNodeInfo {
            val until = System.currentTimeMillis() + 5000
            while (System.currentTimeMillis() < until) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun click(label: String) {
            var n: AccessibilityNodeInfo? = waitNode(label)
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true)
            waitForIdleSync(); Thread.sleep(300)
        }
        var cancelledLegacyPicker = false
        fun saveCurrent(file: File) {
            if (Build.VERSION.SDK_INT < 29) {
                if (!cancelledLegacyPicker) {
                    click("Save as…")
                    waitNode("SAVE")
                    check(uiAutomation.performGlobalAction(android.accessibilityservice.AccessibilityService.GLOBAL_ACTION_BACK))
                    waitNode("Save as…")
                    cancelledLegacyPicker = true
                }
                click("Save as…")
                click("SAVE")
                try { waitNode("Saved") } catch (failure: Throwable) {
                    error("Save as did not complete for ${file.name}: ${failure.message}")
                }
                return
            }
            val collection = MediaStore.Downloads.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
            fun publishedIds(): Set<Long> = targetContext.contentResolver.query(collection,
                arrayOf(MediaStore.MediaColumns._ID), "${MediaStore.MediaColumns.DISPLAY_NAME} LIKE ? AND ${MediaStore.MediaColumns.IS_PENDING}=0",
                arrayOf(file.nameWithoutExtension + "%"), null)?.use { cursor ->
                    buildSet { while (cursor.moveToNext()) add(cursor.getLong(0)) }
                }.orEmpty()
            val before = publishedIds()
            click("Save to Downloads")
            val until = System.currentTimeMillis() + 5000
            var created: Uri? = null
            while (System.currentTimeMillis() < until && created == null) {
                val id = (publishedIds() - before).firstOrNull()
                if (id != null) created = ContentUris.withAppendedId(collection, id) else Thread.sleep(100)
            }
            val uri = created ?: error("Saved MediaStore row was not published")
            ownedDownloads += uri
            check(targetContext.contentResolver.openInputStream(uri)!!.use { it.readBytes() }.contentEquals(file.readBytes())) { "Saved bytes changed for ${file.name}" }
            check(targetContext.contentResolver.getType(uri) == attachmentMime(file.name)) { "Wrong MIME for ${file.name}" }
            waitNode("Saved to Downloads")
        }
        waitNode("Preview ${picture.name}")
        fun screenshot(name: String) {
            val image = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir,name).outputStream().use { image.compress(Bitmap.CompressFormat.PNG,100,it) }; image.recycle()
        }
        screenshot("attachment-thumbnails.png")
        val saveLabel = if (Build.VERSION.SDK_INT >= 29) "Save to Downloads" else "Save as…"
        click("Preview ${picture.name}"); waitNode("Close preview"); waitNode("Open with"); waitNode(saveLabel)
        screenshot("attachment-image-preview.png")
        saveCurrent(picture)
        click("Close preview"); click(note.name); waitNode(jsonPreview(note.readText()).text)
        screenshot("attachment-text-preview.png")
        saveCurrent(note)
        click("Close preview"); runOnMainSync { files = listOf(Attachment(apk.path,apk.name,"file")) }; waitForIdleSync()
        click(apk.name); waitNode(saveLabel)
        saveCurrent(apk); saveCurrent(apk)
        click("Close preview")
        val uri = androidx.core.content.FileProvider.getUriForFile(targetContext, "dev.modex.attachments", note)
        check(targetContext.contentResolver.openInputStream(uri)!!.use { it.readBytes().decodeToString() } == note.readText())
        var denied = false
        try { androidx.core.content.FileProvider.getUriForFile(targetContext,"dev.modex.attachments", File(targetContext.filesDir,"private-key")) } catch (_: IllegalArgumentException) { denied = true }
        check(denied) { "FileProvider exposes files outside preview cache" }
        result.putString("result", if (Build.VERSION.SDK_INT >= 29)
            "PASS: attachment previews plus original PNG/JSON/APK Downloads bytes, MIME and duplicate rows"
        else "PASS: attachment previews plus neutral picker cancellation and Save as for ${apk.name}")
        finish(-1,result)
    } catch (e: Throwable) { result.putString("result","FAIL: attachment preview: ${e.message}"); finish(0,result) }
    finally { ownedDownloads.forEach { targetContext.contentResolver.delete(it, null, null) }; root.deleteRecursively() }
}
