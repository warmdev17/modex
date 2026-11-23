package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.view.View
import android.view.ViewGroup
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue

/** Synthetic Compose rendering only; never connects to a host or sends a prompt. */
internal fun Instrumentation.streamingRenderSmoke() {
    val output = Bundle()
    try {
        if (Build.VERSION.SDK_INT >= 33) uiAutomation.grantRuntimePermission(targetContext.packageName, android.Manifest.permission.POST_NOTIFICATIONS)
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var messages by mutableStateOf((0..29).map { ChatEntry(id = "past-$it", role = "You", text = "Past message $it") } +
            ChatEntry(id = "stream", role = "Codex", text = "Stream start", status = "inProgress"))
        runOnMainSync {
            activity.setContent {
                ModexTheme {
                    Surface {
                        ChatWorkspace(
                            host = "fixture", connected = true, connecting = false, selected = "stream",
                            threads = listOf("stream"), titles = emptyMap(), messages = messages,
                            busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "",
                            sessionModel = "fixture", decisions = emptyList(), answeringDecisionIds = emptySet(),
                            onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
                            onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {}
                        )
                    }
                }
            }
        }
        fun root(): AccessibilityNodeInfo? {
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return uiAutomation.rootInActiveWindow
        }
        fun find(node: AccessibilityNodeInfo?, match: (AccessibilityNodeInfo) -> Boolean): AccessibilityNodeInfo? {
            if (node == null) return null
            if (match(node)) return node
            for (i in 0 until node.childCount) find(node.getChild(i), match)?.let { return it }
            return null
        }
        fun composeView(view: View): View? {
            if (view.javaClass.name == "androidx.compose.ui.platform.AndroidComposeView") return view
            if (view is ViewGroup) for (i in 0 until view.childCount) composeView(view.getChildAt(i))?.let { return it }
            return null
        }
        fun semanticsContains(text: String): Boolean {
            val compose = composeView(activity.window.decorView) ?: return false
            val owner = compose.javaClass.getMethod("getSemanticsOwner").invoke(compose)
            fun visit(node: Any): Boolean {
                val config = node.javaClass.getMethod("getConfig").invoke(node) as Iterable<*>
                if (config.any { (it as? Map.Entry<*, *>)?.value.toString().contains(text) }) return true
                val children = node.javaClass.getMethod("getChildren").invoke(node) as List<*>
                return children.any { it != null && visit(it) }
            }
            val node = owner.javaClass.getMethod("getRootSemanticsNode").invoke(owner)
            return visit(node)
        }
        fun awaitRenderedText(text: String, timeoutMs: Long) {
            val deadline = SystemClock.elapsedRealtime() + timeoutMs
            while (SystemClock.elapsedRealtime() < deadline) {
                var found = false
                runOnMainSync { found = semanticsContains(text) }
                if (found) return
                Thread.sleep(20)
            }
            error("Timed out waiting for $text")
        }
        awaitRenderedText("Stream start", 5_000)
        val paragraph = "Streaming **bold** text with `inline code` and a [safe link](https://example.com). "
        val burst = buildString {
            append("Stream start\n\n# Burst heading\n\n- first item\n- second item\n\n")
            while (length < 500) append(paragraph)
            append("\n\nSTREAM_BURST_END")
        }
        val burstStarted = SystemClock.elapsedRealtimeNanos()
        runOnMainSync { messages = messages.dropLast(1) + messages.last().copy(text = burst) }
        awaitRenderedText("STREAM_BURST_END", 5_000)
        val burstMs = (SystemClock.elapsedRealtimeNanos() - burstStarted) / 1_000_000
        output.putString("burst_timing", "500-char mixed Markdown first appeared in ${burstMs}ms")
        check(burstMs < 300) { "500-char live Markdown took ${burstMs}ms; budget is 300ms" }
        val markdown = buildString {
            append(burst)
            append("\n\n```kotlin\nval ready = true\n```\n\n")
            while (length < 24_000) append(paragraph)
            append("\n\nSTREAM_RENDER_END")
        }
        val started = SystemClock.elapsedRealtimeNanos()
        runOnMainSync { messages = messages.dropLast(1) + messages.last().copy(text = markdown) }
        awaitRenderedText("STREAM_RENDER_END", 8_000)
        val renderMs = (SystemClock.elapsedRealtimeNanos() - started) / 1_000_000
        output.putString("timing", "24 KiB mixed Markdown first appeared in ${renderMs}ms")
        var mixedMarkdownPresent = false
        runOnMainSync { mixedMarkdownPresent = semanticsContains("val ready = true") }
        check(mixedMarkdownPresent) { "Mixed Markdown content was lost" }
        val bottomDeadline = SystemClock.elapsedRealtime() + 3_000
        var followed = false
        while (SystemClock.elapsedRealtime() < bottomDeadline && !followed) {
            val current = root()
            followed = find(current) { it.isVisibleToUser && it.text?.toString()?.contains("STREAM_RENDER_END") == true } != null &&
                find(current) { it.isScrollable }?.actionList?.none { it.id == AccessibilityNodeInfo.ACTION_SCROLL_FORWARD } == true
            if (!followed) Thread.sleep(20)
        }
        check(followed) { "Live Markdown did not remain anchored at the bottom" }
        output.putString("result", "PASS: bounded live Markdown rendering and bottom anchor")
        finish(-1, output)
    } catch (error: Throwable) {
        output.putString("result", "FAIL: ${error.stackTraceToString()}")
        finish(0, output)
    }
}
