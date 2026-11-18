package dev.modex

import android.app.Instrumentation
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

/** Local callbacks only: no host, provider requests or conversation mutations. */
internal fun Instrumentation.slashCommandsSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var selected by mutableStateOf("source")
        var busy by mutableStateOf(false)
        val sent = mutableListOf<String>()
        var sentReferences = emptyList<PromptReference>()
        val actions = mutableListOf<Triple<String, String, Boolean>>()
        val skill = PromptReference("skill", "fixture", "/fixture/SKILL.md", "Fixture skill", "Synthetic reference", "fixture", true, "\$fixture")
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Command fixture", connected = true, connecting = false, selected = selected,
            threads = listOf("source", "other"), titles = mapOf("source" to "Source conversation", "other" to "Other conversation"),
            messages = listOf(ChatEntry(role = "Codex", text = "Completed fixture reply", status = "completed")),
            busy = busy, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture-model",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = { selected = it }, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
            onSend = { text, _, _, references -> sent += text; sentReferences = references; false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            onSessionAction = { kind, id, confirmed -> actions += Triple(kind, id, confirmed) },
            referenceCatalog = ReferenceCatalog(listOf(skill), listOf("skill")),
            models = listOf(ModelOption("fixture-model", "Fixture model", "medium", listOf("medium" to "Balanced")))
        ) } } } }
        fun nodes(label: String): List<AccessibilityNodeInfo> {
            val result = mutableListOf<AccessibilityNodeInfo>()
            fun walk(n: AccessibilityNodeInfo?) {
                if (n == null) return
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) result += n
                for (i in 0 until n.childCount) walk(n.getChild(i))
            }
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            walk(uiAutomation.rootInActiveWindow); return result
        }
        fun await(label: String): AccessibilityNodeInfo {
            val end = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < end) { nodes(label).firstOrNull()?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun touch(label: String) {
            val target = await(label)
            check(target.refresh()) { "Stale touch target $label" }
            val bounds = Rect().also(target::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = MotionEvent.obtain(time, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                sendPointerSync(event); event.recycle()
            }
            waitForIdleSync(); Thread.sleep(250)
        }
        fun field(): AccessibilityNodeInfo {
            var node: AccessibilityNodeInfo? = await("Message Codex")
            while (node != null && node.className?.toString() != "android.widget.EditText") node = node.parent
            return node ?: error("Missing editable composer")
        }
        fun type(value: String) {
            touch("Message Codex")
            check(field().performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, value) }))
            waitForIdleSync(); Thread.sleep(300)
        }
        fun back() { sendKeyDownUpSync(KeyEvent.KEYCODE_BACK); Thread.sleep(250) }
        fun shot(name: String) {
            waitForIdleSync(); Thread.sleep(500)
            val image = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; image.recycle()
        }
        type("/"); await("Android commands"); shot("slash-commands.png")
        check(nodes("/vim").isEmpty() && nodes("/keymap").isEmpty())
        type("/mo"); await("/model"); check(nodes("/delete").isEmpty()); shot("slash-filtered.png"); touch("/model")
        await("Model for next turn"); touch("Cancel"); check(field().text.isNullOrEmpty())
        type("/new"); touch("Send message"); await("New conversation"); touch("Cancel")
        type("/help"); touch("/help"); await("Close"); touch("Close")
        type("/skills"); touch("/skills"); await("Prompt references"); check(field().text.isNullOrEmpty()); touch("Close reference suggestions")
        type("/skills"); touch("/skills"); touch("Select skill fixture")
        check(field().text.toString() == skill.marker + " "); touch("Send message"); check(sentReferences == listOf(skill)) { "Manual skill selection lost its binding" }
        type("/delete"); touch("/delete"); await("Delete conversation?"); touch("Cancel"); check(actions.isEmpty())
        type("/delete"); touch("Send message"); touch("Delete permanently")
        check(actions == listOf(Triple("delete", "source", true)))
        type("/archive"); touch("/archive"); check(actions.last() == Triple("archive", "source", false))
        type("/fork"); touch("Send message"); check(actions.last() == Triple("fork", "source", false))
        type("Keep this draft"); touch("Open sessions"); touch("Other conversation")
        type("/resume"); touch("/resume"); await("Sessions"); touch("Source conversation")
        check(field().text.toString() == "Keep this draft") { "Session draft was lost" }
        type("/literal/path"); touch("Send message"); check(sent == listOf(skill.marker + " ", "/literal/path")) { "A command leaked into a model request: $sent" }
        runOnMainSync { busy = true }
        type("/fork")
        var fork: AccessibilityNodeInfo? = await("/fork")
        var disabled = false
        while (fork != null) { if (!fork.isEnabled) disabled = true; fork = fork.parent }
        check(disabled) { "Busy Fork must be disabled" }
        shot("slash-disabled.png")
        runOnMainSync { busy = false }
        type("/mo"); touch("Open sessions"); await("Sessions"); back(); await("/model")
        touch("Close command suggestions"); check(field().text.toString() == "/mo") { "Dismissing commands lost the draft" }
        type("/copy"); touch("/copy")
        val clipboard = targetContext.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        runOnMainSync { check(clipboard.primaryClip!!.getItemAt(0).text.toString() == "Completed fixture reply") }
        output.putString("result", "PASS: slash filtering/actions, exact-command Send interception, draft retention, skills without fake marker, clipboard, confirmation and busy gates; no command sent as prompt")
        finish(-1, output)
    } catch (failure: Throwable) {
        uiAutomation.takeScreenshot()?.let { image -> File(targetContext.filesDir, "slash-failure.png").outputStream().use { image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; image.recycle() }
        val tree = StringBuilder()
        fun dump(node: AccessibilityNodeInfo?, depth: Int = 0) {
            if (node == null) return
            tree.append(" ".repeat(depth)).append("${node.text} | ${node.contentDescription} | ${node.className} | focus=${node.isFocused} | ${Rect().also(node::getBoundsInScreen)} | ${node.actionList.map { it.id }}\n")
            for (i in 0 until node.childCount) dump(node.getChild(i), depth + 1)
        }
        dump(uiAutomation.rootInActiveWindow); File(targetContext.filesDir, "slash-failure-tree.txt").writeText(tree.toString())
        output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, output)
    }
}
