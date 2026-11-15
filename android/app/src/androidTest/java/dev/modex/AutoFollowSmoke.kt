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
        var busy by mutableStateOf(false)
        var stopping by mutableStateOf(false)
        var stopRequested = false
        var messages by mutableStateOf((0..29).map { ChatEntry(id = "past-$it", role = "You", text = "Past message $it") } + ChatEntry(id = "stream", role = "Codex", text = "Stream start", status = "inProgress"))
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            "fixture", true, false, "follow", listOf("follow"), emptyMap(), messages,
            busy, false, false, "", "", "fixture-model", emptyList(), emptySet(), {}, {}, {}, { _, _, _ -> }, { _, _, _, _ -> false }, { _, _ -> }, {}, {}, stopping = stopping, onStop = { stopRequested = true; stopping = true }
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
        check(find(root()) { it.contentDescription == "Scroll to latest" } == null) { "Jump button visible at bottom" }
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
        var jump = find(root()) { it.contentDescription == "Scroll to latest" } ?: error("No jump button while reading history")
        while (!jump.isClickable) jump = jump.parent ?: error("Jump button is not clickable")
        check(jump.performAction(AccessibilityNodeInfo.ACTION_CLICK))
        waitForIdleSync()
        await("STREAM_END_SECOND")
        runOnMainSync { messages = messages + (0..5).map { ChatEntry(id = "batch-$it", role = "You", text = if (it == 5) "BATCH_END" else "Batch message $it") } }
        await("BATCH_END")
        runOnMainSync { messages = messages + ChatEntry(id = "next", role = "Codex", text = "Next stream", status = "inProgress") }
        await("Next stream")
        runOnMainSync { messages = messages.dropLast(1) + messages.last().copy(text = "Next stream\n\n" + (0..39).joinToString("\n\n") { "Another paragraph $it" } + "\n\nFINAL_STREAM_END") }
        await("FINAL_STREAM_END")
        screenshot("auto-follow-resumed.png")
        check(find(root()) { it.contentDescription == "Scroll to latest" } == null) { "Jump button remained at bottom" }
        runOnMainSync {
            busy = true
            messages = messages + ChatEntry(id = "active-user", role = "You", text = "Continue profile work", status = "inProgress", turnId = "active", startedAtMs = System.currentTimeMillis() - 9000) +
                ChatEntry(id = "active-reasoning", role = "reasoning", text = "**Continuing profile work**", status = "inProgress", turnId = "active")
        }
        waitForIdleSync(); Thread.sleep(700)
        screenshot("composer-active.png")
        val activityNode = find(root()) { it.contentDescription?.toString()?.startsWith("Turn activity: ") == true } ?: error("No activity above composer")
        val activityText = activityNode.contentDescription.toString()
        check(activityText.contains("Continuing profile work") && Regex("[0-9]+s").containsMatchIn(activityText)) { "Missing thinking action or elapsed time: $activityText" }
        val activityBounds = Rect().also { activityNode.getBoundsInScreen(it) }
        val inputBounds = Rect().also { find(root()) { it.contentDescription == "Message Codex" }!!.getBoundsInScreen(it) }
        check(activityBounds.bottom <= inputBounds.top) { "Activity is not above prompt input" }
        check(find(list()) { it.text?.toString()?.contains("Continuing profile work") == true } == null) { "Thinking is duplicated inside the transcript" }
        check(find(root()) { it.text == "working" } == null) { "Duplicate working beside model" }
        check(find(root()) { it.text?.contains("@ for skills") == true } == null) { "Composer has verbose placeholder" }
        var stop = find(root()) { it.contentDescription == "Stop response" } ?: error("No stop button")
        while (!stop.isClickable) stop = stop.parent ?: error("Stop button is not clickable")
        val stopBounds = Rect().also { stop.getBoundsInScreen(it) }
        val screen = uiAutomation.takeScreenshot() ?: error("No screenshot")
        val color = screen.getPixel(stopBounds.left + stopBounds.width() / 6, stopBounds.centerY()) and 0xffffff
        screen.recycle()
        check(color == 0xf38ba8) { "Stop control is not Catppuccin red: ${color.toString(16)}" }
        check(stop.performAction(AccessibilityNodeInfo.ACTION_CLICK))
        waitForIdleSync(); Thread.sleep(500)
        runOnMainSync { check(stopRequested) { "Stop did not reach callback" } }
        check(find(root()) { it.contentDescription == "Stopping" } != null)
        runOnMainSync {
            busy = false; stopping = false
            messages = messages.map { if (it.id in setOf("active-user", "active-reasoning")) it.copy(status = "interrupted", durationMs = 12000) else it }
        }
        waitForIdleSync(); Thread.sleep(500)
        check(find(root()) { it.contentDescription?.toString()?.startsWith("Turn activity: ") == true } == null) { "Live activity remained after interrupt" }
        check(visible("Interrupted · 12s")) { "Completed history lost interruption timer" }
        check(find(list()) { it.text?.toString()?.contains("Continuing profile work") == true } == null) { "Historical thinking remained inside the transcript" }
        runOnMainSync {
            busy = true
            messages = messages + ChatEntry(id = "next-user", role = "You", text = "Next task", status = "inProgress", turnId = "next")
        }
        waitForIdleSync(); Thread.sleep(500)
        val fallback = find(root()) { it.contentDescription?.toString()?.startsWith("Turn activity: ") == true } ?: error("No fallback activity before reasoning arrives")
        check(fallback.contentDescription.toString().contains("Working") && !fallback.contentDescription.toString().contains("Continuing profile work")) { "Previous turn thinking leaked into new activity" }
        output.putString("result", "PASS: animated text growth, tall-message reading anchor, jump to latest, resumed following, activity placement, red stop, interruption and new-turn fallback")
        finish(-1, output)
    } catch (e: Throwable) {
        output.putString("result", "FAIL: ${e.stackTraceToString()}")
        finish(0, output)
    }
}
