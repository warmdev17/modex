package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.Build
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

/** Only synthetic messages on a disposable emulator; never sends a host prompt. */
internal fun Instrumentation.autoFollowSmoke() {
    val output = Bundle()
    try {
        if (Build.VERSION.SDK_INT >= 33) uiAutomation.grantRuntimePermission(targetContext.packageName, android.Manifest.permission.POST_NOTIFICATIONS)
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var messages by mutableStateOf((0..29).map { ChatEntry(id = "past-$it", role = "You", text = "Past message $it") } + ChatEntry(id = "stream", role = "Codex", text = "Stream start", status = "inProgress"))
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            "fixture", true, false, "follow", listOf("follow"), emptyMap(), messages,
            false, false, false, "", "", "fixture-model", emptyList(), emptySet(), {}, {}, {}, { _, _, _ -> }, { _, _, _, _ -> false }, { _, _ -> }, {}, {}
        ) } } } }
        fun root(): AccessibilityNodeInfo? {
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return uiAutomation.rootInActiveWindow
        }
        fun find(n: AccessibilityNodeInfo?, match: (AccessibilityNodeInfo) -> Boolean): AccessibilityNodeInfo? {
            if (n == null) return null
            if (match(n)) return n
            for (i in 0 until n.childCount) find(n.getChild(i), match)?.let { return it }
            return null
        }
        fun visible(label: String) = find(root()) { it.isVisibleToUser && it.text?.toString()?.contains(label) == true } != null
        fun atBottom(label: String): Boolean {
            val list = find(root()) { it.isScrollable } ?: return false
            return visible(label) && list.actionList.none { it.id == AccessibilityNodeInfo.ACTION_SCROLL_FORWARD }
        }
        fun await(label: String) {
            val end = System.currentTimeMillis() + 8000
            while (System.currentTimeMillis() < end && !atBottom(label)) Thread.sleep(100)
            check(atBottom(label)) {
                fun describe(n: AccessibilityNodeInfo?): String = if (n == null) "null" else
                    "[${n.text?.takeLast(100)}|scroll=${n.isScrollable}|actions=${n.actionList.map { it.id }}]" +
                        (0 until n.childCount).joinToString { describe(n.getChild(it)) }
                "Chat did not follow $label to the bottom: ${describe(root())}"
            }
        }
        fun list() = find(root()) { it.isScrollable } ?: error("History is not scrollable")
        fun scroll(action: Int) {
            check(list().performAction(action)) { "Cannot scroll history" }
            waitForIdleSync(); Thread.sleep(500)
        }
        fun screenshot(name: String) {
            uiAutomation.takeScreenshot()?.let { bitmap ->
                File(targetContext.filesDir, name).outputStream().use { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
                bitmap.recycle()
            }
        }
        await("Stream start")
        val longText = "Stream start\n\n" + (0..39).joinToString("\n\n") { "Stream paragraph $it" } + "\n\nSTREAM_END_FIRST"
        runOnMainSync { messages = messages.dropLast(1) + messages.last().copy(text = longText) }
        await("STREAM_END_FIRST")
        screenshot("auto-follow-stream.png")
        scroll(AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD)
        check(!atBottom("STREAM_END_FIRST")) { "Did not leave the bottom" }
        fun viewport(): android.graphics.Bitmap {
            val bounds = Rect().also { list().getBoundsInScreen(it) }
            val screen = uiAutomation.takeScreenshot() ?: error("No screenshot")
            bounds.intersect(0, 0, screen.width, screen.height)
            val cropped = android.graphics.Bitmap.createBitmap(screen, bounds.left, bounds.top, bounds.width(), bounds.height())
            if (cropped !== screen) screen.recycle()
            return cropped
        }
        val before = viewport()
        runOnMainSync { messages = messages.dropLast(1) + messages.last().copy(text = longText + "\n\n" + (40..59).joinToString("\n\n") { "Stream paragraph $it" } + "\n\nSTREAM_END_SECOND") }
        Thread.sleep(1800)
        val after = viewport()
        check(before.sameAs(after)) { "Streaming moved the reading viewport" }
        before.recycle(); after.recycle()
        check(!atBottom("STREAM_END_SECOND")) { "Streaming pulled reader to bottom" }
        repeat(12) { if (!atBottom("STREAM_END_SECOND")) scroll(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD) }
        await("STREAM_END_SECOND")
        runOnMainSync { messages = messages + (0..5).map { ChatEntry(id = "batch-$it", role = "You", text = if (it == 5) "BATCH_END" else "Batch message $it") } }
        await("BATCH_END")
        runOnMainSync { messages = messages + ChatEntry(id = "next", role = "Codex", text = "Next stream", status = "inProgress") }
        await("Next stream")
        runOnMainSync { messages = messages.dropLast(1) + messages.last().copy(text = "Next stream\n\n" + (0..39).joinToString("\n\n") { "Another paragraph $it" } + "\n\nFINAL_STREAM_END") }
        await("FINAL_STREAM_END")
        screenshot("auto-follow-resumed.png")
        output.putString("result", "PASS: animated text growth, tall-message reading anchor, resume following at bottom and batched messages")
        finish(-1, output)
    } catch (e: Throwable) {
        output.putString("result", "FAIL: ${e.stackTraceToString()}")
        finish(0, output)
    }
}
