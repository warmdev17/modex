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
                listOf("You" to "Make this easier to use on my phone.", "Codex" to markdown), false, false, false, "",
                { current = it; selected = it }, {}, {}, {}, { attempted = it; false }, {}, {}
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
