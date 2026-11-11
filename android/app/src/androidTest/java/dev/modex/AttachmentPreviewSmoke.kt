package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Color
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.runtime.*
import androidx.compose.material3.Surface
import java.io.File

/** Render-only previews from isolated app-cache files, without host or external app launches. */
internal fun Instrumentation.attachmentPreviewSmoke() {
    val result = Bundle()
    val root = File(targetContext.cacheDir, "attachment-preview/smoke").apply { mkdirs() }
    try {
        val picture = File(root, "sample.png")
        val bitmap = Bitmap.createBitmap(64, 48, Bitmap.Config.ARGB_8888).apply { eraseColor(Color.RED) }
        picture.outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }; bitmap.recycle()
        val note = File(root, "notes.json").apply { writeText("""{"status":"readable on the phone","count":1234567890123456789,"active":true,"missing":null}""") }
        val files = listOf(Attachment(picture.path,picture.name,"image"), Attachment(note.path,note.name,"file"))
        val activity = startActivitySync(Intent(targetContext,MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        runOnMainSync { activity.setContent { ModexTheme { Surface {
            ChatWorkspace(host = "preview", connected = true, connecting = false, error = "", sessionModel = "fixture", selected = "t", threads = listOf("t"), titles = emptyMap(),
                messages = listOf(ChatEntry("u", "You", "Attached earlier", attachments = files)), decisions = emptyList(), busy = false, historyLoading = false,
                hasMore = false, sessionSettings = "", answeringDecisionIds = emptySet(), onMore = {}, onSelect = {}, onCreate = { _, _, _ -> }, onSend = { _, _, _ -> false }, onAnswer = { _, _ -> }, onRefresh = {}, onConnection = {}, onDisconnect = {},
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
        waitNode("Preview sample.png")
        fun screenshot(name: String) {
            val image = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir,name).outputStream().use { image.compress(Bitmap.CompressFormat.PNG,100,it) }; image.recycle()
        }
        screenshot("attachment-thumbnails.png")
        click("Preview sample.png"); waitNode("Close preview"); waitNode("Open with")
        screenshot("attachment-image-preview.png")
        click("Close preview"); click("notes.json"); waitNode(jsonPreview(note.readText()).text)
        screenshot("attachment-text-preview.png")
        click("Close preview")
        val uri = androidx.core.content.FileProvider.getUriForFile(targetContext, "dev.modex.attachments", note)
        check(targetContext.contentResolver.openInputStream(uri)!!.use { it.readBytes().decodeToString() } == note.readText())
        var denied = false
        try { androidx.core.content.FileProvider.getUriForFile(targetContext,"dev.modex.attachments", File(targetContext.filesDir,"private-key")) } catch (_: IllegalArgumentException) { denied = true }
        check(denied) { "FileProvider exposes files outside preview cache" }
        result.putString("result","PASS: composer/sent thumbnails, image and text full-screen previews, close navigation and scoped FileProvider")
        finish(-1,result)
    } catch (e: Throwable) { result.putString("result","FAIL: attachment preview: ${e.message}"); finish(0,result) }
    finally { root.deleteRecursively() }
}
