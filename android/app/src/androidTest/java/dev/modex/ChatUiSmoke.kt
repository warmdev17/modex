package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

/** Render-only fixture. No host connection, credentials, or prompt is submitted. */
fun Instrumentation.chatUiSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var markdown by mutableStateOf("## Ready to build\n\nUse **native Android** with `Jetpack Compose`.\n\n- Keep the session ID\n- Render Markdown cleanly\n\n```kotlin\nval message = \"Hello, Modex\"\nprintln(message)\n```")
        var selected = "fixture-one"
        var attempted = ""
        runOnMainSync { activity.setContent {
            var current by remember { mutableStateOf(selected) }
            ModexTheme { Surface { ChatWorkspace(
                "laptop.local", true, false, current,
                listOf("fixture-one","fixture-two"), mapOf("fixture-one" to "Modex mobile redesign", "fixture-two" to "SSH approval flow"),
                listOf("You" to "Make this easier to use on my phone.", "Codex" to markdown), false, false, false, "", "",
                { current = it; selected = it }, {}, {}, { _, _ -> }, { attempted = it; false }, {}, {}
            ) } }
        } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun click(label: String) {
            val deadline = System.currentTimeMillis()+5000
            var n: AccessibilityNodeInfo? = null
            while (n == null && System.currentTimeMillis()<deadline) { n = node(label); if(n == null) Thread.sleep(100) }
            check(n != null) { "Missing $label" }
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            Thread.sleep(500)
        }
        fun shot(name: String) {
            waitForIdleSync(); Thread.sleep(500)
            val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir,name).outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG,100,it) }; bmp.recycle()
        }
        shot("chat-mocha.png")
        click("Copy code")
        check(node("Copied") != null)
        Thread.sleep(3500)
        click("Open sessions")
        shot("drawer-mocha.png")
        click("SSH approval flow")
        runOnMainSync { check(selected == "fixture-two") }
        fun shell(command: String) {
            android.os.ParcelFileDescriptor.AutoCloseInputStream(uiAutomation.executeShellCommand(command)).use { it.readBytes() }
        }
        click("Message Codex")
        val field = node("Message Codex") ?: error("Composer inaccessible after focus")
        val bounds = android.graphics.Rect(); field.getBoundsInScreen(bounds)
        shell("input tap ${bounds.centerX()} ${bounds.centerY()}")
        Thread.sleep(500)
        shell("input text Keep%sthis%sdraft")
        Thread.sleep(500)
        click("Send message")
        runOnMainSync { check(attempted == "Keep this draft") { "Fixture did not type draft" }; attempted = "" }
        click("Send message")
        runOnMainSync { check(attempted == "Keep this draft") { "Rejected send lost draft" } }
        Thread.sleep(700)
        shot("composer-keyboard.png")
        shell("input keyevent 4")
        runOnMainSync { markdown = "## Structured output\n\n| File | Status |\n| --- | --- |\n| MainActivity.kt | Updated |\n\n> Keep the existing session ID.\n\n1. Read **history**\n2. Resume safely\n\n[Documentation](https://example.com)" }
        shot("markdown-table.png")
        check(File(targetContext.filesDir, "markdown-table.png").length() > 0) { "Table screenshot missing" }
        output.putString("result", "PASS: Mocha chat, code copy, drawer selection, rejected-send draft retained, composer focus")
        finish(-1,output)
    } catch (e: Throwable) { output.putString("result","FAIL: ${e.javaClass.simpleName}: ${e.message} at ${e.stackTrace.firstOrNull { it.fileName == "ChatUiSmoke.kt" }}"); finish(0,output) }
}

/** Regression: history arrives after the list has already laid out at its beginning. */
fun Instrumentation.historyScrollSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var loading by mutableStateOf(false)
        var messages by mutableStateOf(listOf("You" to "Old message"))
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            "fixture", true, false, "history", listOf("history"), emptyMap(),
            messages, false, loading, false, "", "", {}, {}, {}, { _, _ -> }, { false }, {}, {}
        ) } } } }
        waitForIdleSync(); Thread.sleep(700)
        runOnMainSync { loading = true }
        waitForIdleSync(); Thread.sleep(500)
        runOnMainSync {
            messages = (0..199).map { "You" to if (it == 199) "LATEST_HISTORY_MARKER" else "Earlier message $it" }
            loading = false
        }
        fun visibleMarker(n: AccessibilityNodeInfo?): Boolean {
            if (n == null) return false
            if (n.isVisibleToUser && n.text?.toString()?.endsWith("LATEST_HISTORY_MARKER") == true) return true
            return (0 until n.childCount).any { visibleMarker(n.getChild(it)) }
        }
        val deadline = System.currentTimeMillis() + 5000
        while (!visibleMarker(uiAutomation.rootInActiveWindow) && System.currentTimeMillis() < deadline) Thread.sleep(100)
        check(visibleMarker(uiAutomation.rootInActiveWindow)) {
            fun labels(n: AccessibilityNodeInfo?): String = if (n == null) "null" else "[${n.text}|${n.contentDescription}]" + (0 until n.childCount).joinToString { labels(n.getChild(it)) }
            "History opened at the beginning: ${labels(uiAutomation.rootInActiveWindow)}"
        }
        fun scrollable(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
            if (n == null) return null
            if (n.isScrollable) return n
            for (i in 0 until n.childCount) scrollable(n.getChild(i))?.let { return it }
            return null
        }
        repeat(1) {
            check(scrollable(uiAutomation.rootInActiveWindow)?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD) == true) { "Cannot scroll history" }
            Thread.sleep(700)
        }
        check(!visibleMarker(uiAutomation.rootInActiveWindow)) { "Fixture did not scroll to older messages" }
        fun earlierMessage(n: AccessibilityNodeInfo?): String? {
            if (n == null) return null
            if (n.isVisibleToUser && n.text?.toString()?.contains("Earlier message") == true) return n.text.toString()
            for (i in 0 until n.childCount) earlierMessage(n.getChild(i))?.let { return it }
            return null
        }
        val reading = earlierMessage(uiAutomation.rootInActiveWindow) ?: error("No older message visible")
        runOnMainSync { messages = messages + ("You" to "New streaming message") }
        waitForIdleSync(); Thread.sleep(700)
        check(earlierMessage(uiAutomation.rootInActiveWindow) == reading) { "New message stole the older-history position" }
        runOnMainSync { loading = true }
        waitForIdleSync(); Thread.sleep(500)
        runOnMainSync {
            messages = messages.dropLast(1) + ("You" to ("Tall final message\n".repeat(100) + "LATEST_HISTORY_MARKER"))
            loading = false
        }
        val tallDeadline = System.currentTimeMillis() + 5000
        while (!visibleMarker(uiAutomation.rootInActiveWindow) && System.currentTimeMillis() < tallDeadline) Thread.sleep(100)
        check(visibleMarker(uiAutomation.rootInActiveWindow)) { "Reload did not reach tall final message" }
        output.putString("result", "PASS: 200-message async history, upward reading preserved on append, reload with tall final message")
        finish(-1, output)
    } catch (e: Throwable) { output.putString("result", "FAIL: ${e.message}"); finish(0, output) }
}
