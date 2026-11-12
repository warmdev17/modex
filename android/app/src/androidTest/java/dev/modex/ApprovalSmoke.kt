package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import java.io.File
import org.json.JSONObject

/** Local UI fixture only; no SSH connection or command is executed. */
fun Instrumentation.approvalPreview() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val approval = JSONObject("""{"decision_id":"fixture","method":"item/commandExecution/requestApproval","thread_id":"approval-preview","context":{"command":"printf MODEX_APPROVAL_OK","cwd":"/home/warmdev/Workspace/modex","reason":"Run a harmless command outside the read-only sandbox.","availableDecisions":["accept","cancel"]}}""")
        val question = JSONObject("""{"decision_id":"question","method":"item/tool/requestUserInput","thread_id":"approval-preview","context":{"questions":[{"id":"choice","question":"Which approach should Codex use?","options":[{"label":"Safe","description":"Keep approval prompts."}]}]}}""")
        val hidden = JSONObject("""{"decision_id":"hidden","method":"item/tool/requestUserInput","thread_id":"another-thread","context":{"questions":[{"id":"hidden","question":"HIDDEN_OTHER_THREAD"}]}}""")
        fun findLabel(label: String, node: AccessibilityNodeInfo? = uiAutomation.rootInActiveWindow): AccessibilityNodeInfo? {
            if (node == null) return null
            if (node.text?.toString() == label || node.contentDescription?.toString() == label) return node
            return (0 until node.childCount).firstNotNullOfOrNull { findLabel(label, node.getChild(it)) }
        }
        fun awaitLabel(label: String): AccessibilityNodeInfo? {
            val deadline = System.currentTimeMillis() + 5000
            var node = findLabel(label)
            while (node == null && System.currentTimeMillis() < deadline) {
                Thread.sleep(100)
                node = findLabel(label)
            }
            return node
        }
        fun scrollable(node: AccessibilityNodeInfo? = uiAutomation.rootInActiveWindow): AccessibilityNodeInfo? {
            if (node == null) return null
            if (node.isScrollable) return node
            return (0 until node.childCount).firstNotNullOfOrNull { scrollable(node.getChild(it)) }
        }
        fun render(connected: Boolean, decisions: List<JSONObject>) = runOnMainSync {
            activity.setContent {
                ModexTheme { Surface { ChatWorkspace(
                    "fixture", connected, false, "approval-preview", listOf("approval-preview"), emptyMap(),
                    emptyList(), false, false, false, "", "", "fixture-model", decisions, emptySet(),
                    {}, {}, {}, { _, _, _ -> }, { _, _, _ -> false }, { _, _ -> }, {}, {}
                ) } }
            }
        }
        fun screenshot(name: String) {
            waitForIdleSync()
            val image = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use {
                check(image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it))
            }
            image.recycle()
        }

        render(true, listOf(approval, hidden))
        check(awaitLabel("Pending approval") != null) { "Approval is not rendered inline" }
        check(findLabel("HIDDEN_OTHER_THREAD") == null) { "Decision from another thread leaked into transcript" }
        screenshot("approval-inline.png")

        render(true, listOf(question, hidden))
        check(awaitLabel("Agent input request") != null) { "Question request is not inline" }
        repeat(4) {
            if (findLabel("Your answer") == null) {
                check(scrollable()?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD) == true) { "Question input is not reachable" }
                Thread.sleep(300)
            }
        }
        check(awaitLabel("Your answer") != null) { "Question input is not inline" }
        check(findLabel("HIDDEN_OTHER_THREAD") == null) { "Question from another thread leaked into transcript" }
        screenshot("question-inline.png")

        render(false, listOf(approval))
        check(awaitLabel("Reconnect to respond") != null) { "Disconnected decision has no reconnect state" }
        var action = awaitLabel("Allow once")
        while (action?.parent != null && action?.isEnabled == true) action = action?.parent
        check(action?.isEnabled == false) { "Disconnected approval action must be disabled" }
        output.putString("result", "PASS: inline approval/question, thread filtering and disconnected controls")
        finish(-1, output)
    } catch (error: Throwable) {
        output.putString("result", "FAIL: ${error.javaClass.simpleName}: ${error.message} at ${error.stackTrace.firstOrNull { it.fileName == "ApprovalSmoke.kt" }}")
        finish(0, output)
    }
}
