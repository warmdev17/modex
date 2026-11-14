package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

internal fun Instrumentation.referencePickerSmoke() {
    val result = Bundle()
    try {
        val skill = PromptReference("skill", "review", "/home/demo/.agents/skills/review/SKILL.md", "Review changes", "Review code and explain issues", "user", true, "\$review")
        val disabled = skill.copy(name = "disabled", title = "Disabled skill", path = "/disabled/SKILL.md", marker = "\$disabled", enabled = false)
        val plugin = PromptReference("plugin", "review", "plugin://review@curated", "Review plugin", "Review using an installed plugin", "curated", true, "@review", "review@curated")
        val disabledPlugin = plugin.copy(name = "blocked", title = "Blocked plugin", path = "plugin://blocked@curated", marker = "@blocked", enabled = false)
        var sentText = ""
        var sentReferences = emptyList<PromptReference>()
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        runOnMainSync { activity.setContent {
            var current by remember { mutableStateOf("one") }
            ModexTheme { Surface {
                ChatWorkspace(host = "skill-preview", connected = true, connecting = false, selected = current,
                    threads = listOf("one", "two"), titles = mapOf("one" to "First session", "two" to "Second session"), messages = emptyList(),
                    busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture", decisions = emptyList(), answeringDecisionIds = emptySet(),
                    onSelect = { current = it }, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
                    onSend = { text, _, _, refs -> sentText = text; sentReferences = refs; true }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
                    referenceCatalog = ReferenceCatalog(listOf(skill, disabled, plugin, disabledPlugin), listOf("skill", "plugin")))
            } }
        } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.contentDescription?.toString() == label || n.text?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun waitNode(label: String): AccessibilityNodeInfo {
            val until = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < until) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun click(label: String) {
            var n: AccessibilityNodeInfo? = waitNode(label)
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            waitForIdleSync(); Thread.sleep(200)
        }
        fun field(): AccessibilityNodeInfo {
            var n: AccessibilityNodeInfo? = waitNode("Message Codex")
            while (n != null && n.className?.toString() != "android.widget.EditText") n = n.parent
            return n ?: error("Composer EditText missing")
        }
        fun input(text: String) {
            click("Message Codex")
            check(field().performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, text) })) { "Cannot enter prompt" }
            waitForIdleSync(); Thread.sleep(300)
        }
        fun screenshot(name: String) {
            val bitmap = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
            bitmap.recycle()
        }
        input("@")
        waitNode("Select skill review")
        check(!waitNode("Select skill disabled").parent.isEnabled) { "Disabled skill is selectable" }
        screenshot("skill-picker.png")
        click("Close reference suggestions")
        check(node("Prompt references") == null) { "Close did not hide suggestions" }
        input("")
        input("@")
        waitNode("Select skill review")
        click("Close reference suggestions")
        input("@r")
        waitNode("Select skill review")
        input("@")
        waitNode("Select skill review")
        click("Select skill review")
        waitNode("Remove skill review")
        check(field().text.toString().contains("\$review"))
        screenshot("skill-selected.png")
        click("Open sessions"); click("Second session")
        check(field().text.toString().isBlank())
        input("Other draft")
        click("Open sessions"); click("First session")
        waitNode("Remove skill review")
        click("Remove skill review")
        check(!field().text.toString().contains("\$review")) { "Removing chip retained implicit invocation" }
        input("me@review")
        check(node("Prompt references") == null) { "Email opened suggestions" }
        input("Please @rev")
        click("Select skill review")
        click("Send message")
        check(sentText.contains("\$review") && sentReferences.single().path == skill.path) { "Structured reference lost at send" }
        check(field().text.toString().isBlank())
        input("@blocked")
        check(!waitNode("Select plugin blocked").parent.isEnabled) { "Disabled plugin is selectable" }
        input("@curated")
        click("Select plugin review")
        waitNode("Remove plugin review")
        check(field().text.toString().contains("@review"))
        check(node("Prompt references") == null) { "Selected plugin reopened suggestions" }
        screenshot("plugin-selected.png")
        click("Open sessions"); click("Second session")
        check(node("Remove plugin review") == null)
        click("Open sessions"); click("First session")
        waitNode("Remove plugin review")
        click("Remove plugin review")
        check(!field().text.toString().contains("@review"))
        input("@curated")
        click("Select plugin review")
        click("Send message")
        check(sentText.contains("@review") && sentReferences.single() == plugin) { "Plugin identity lost at send" }
        result.putString("result", "PASS: installed plugin selection, disabled state, draft restoration, deletion and send; @ discovery, reopen after dismissal and retyping, disabled state, exact reference selection, per-session drafts, chip deletion, email guard and send")
        finish(-1, result)
    } catch (failure: Throwable) {
        uiAutomation.takeScreenshot()?.let { bitmap -> File(targetContext.filesDir, "skill-failure.png").outputStream().use { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; bitmap.recycle() }
        fun dump(n: AccessibilityNodeInfo?): String {
            if (n == null) return ""
            return "${n.className} text=${n.text} desc=${n.contentDescription} focus=${n.isFocused} actions=${n.actionList}\n" + (0 until n.childCount).joinToString("") { dump(n.getChild(it)) }
        }
        File(targetContext.filesDir, "skill-failure-tree.txt").writeText(dump(uiAutomation.rootInActiveWindow))
        result.putString("result", "FAIL: skill picker: ${failure.stackTraceToString()}"); finish(0, result)
    }
}
