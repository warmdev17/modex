package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import org.json.JSONObject

/** Drives the existing APK card with the two-field native form projection. */
internal fun Instrumentation.openCodeTextFormPreview() {
    val report = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val decision = JSONObject("""{"decision_id":"ses_test/frm_test","method":"item/tool/requestUserInput","context":{"questions":[{"id":"target","question":"Target?"},{"id":"reason","question":"Reason?"}]}}""")
        var result: JSONObject? = null
        runOnMainSync {
            activity.setContent { ModexTheme { InlineDecisionCard(decision, false, true) { result = it } } }
        }
        fun nodes(start: AccessibilityNodeInfo?): List<AccessibilityNodeInfo> =
            if (start == null) emptyList() else listOf(start) + (0 until start.childCount).flatMap { nodes(start.getChild(it)) }
        fun await(predicate: (List<AccessibilityNodeInfo>) -> Boolean): List<AccessibilityNodeInfo> {
            repeat(50) {
                val current = nodes(uiAutomation.rootInActiveWindow)
                if (predicate(current)) return current
                Thread.sleep(100)
            }
            error("Timed out waiting for form controls")
        }
        fun editors() = await { list -> list.count { it.isEditable } == 2 }.filter { it.isEditable }
        fun setText(index: Int, text: String) {
            val args = Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, text) }
            check(editors()[index].performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args))
            waitForIdleSync()
        }
        fun submit(enabled: Boolean): AccessibilityNodeInfo {
            fun button(list: List<AccessibilityNodeInfo>): AccessibilityNodeInfo? {
                var node = list.firstOrNull { it.text?.toString() == "Submit answer" } ?: return null
                while (!node.isClickable && node.parent != null) node = node.parent
                return node.takeIf { it.isEnabled == enabled }
            }
            return checkNotNull(button(await { button(it) != null }))
        }
        val labels = await { list -> list.count { it.isEditable } == 2 }.map { it.text?.toString() }
        check("Target?" in labels && "Reason?" in labels)
        setText(0, "android")
        submit(enabled = false)
        setText(1, "fixture reason")
        check(submit(enabled = true).performAction(AccessibilityNodeInfo.ACTION_CLICK)) { "Cannot click enabled Submit answer" }
        waitForIdleSync()
        runOnMainSync {
            val answers = checkNotNull(result).getJSONObject("answers")
            check(answers.length() == 2)
            check(answers.getJSONObject("target").getJSONArray("answers").getString(0) == "android")
            check(answers.getJSONObject("reason").getJSONArray("answers").getString(0) == "fixture reason")
        }
        report.putString("result", "PASS: OpenCode two-field form renders and submits both answers")
        finish(-1, report)
    } catch (error: Throwable) {
        report.putString("result", "FAIL: OpenCode text form: ${error.message} at ${error.stackTrace.firstOrNull()}")
        finish(0, report)
    }
}
