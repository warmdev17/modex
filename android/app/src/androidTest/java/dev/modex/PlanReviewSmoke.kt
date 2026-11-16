package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

internal fun Instrumentation.planReviewSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val plan = ChatEntry("plan", "plan", "## Approved plan\n\n1. Build native UI.\n2. Verify reconnect.", status = "completed", turnId = "turn")
        var handled by mutableStateOf(emptySet<String>())
        var busy by mutableStateOf(false)
        var action = ""
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Test host", connected = true, connecting = false, selected = "source", threads = listOf("source"), titles = mapOf("source" to "Plan review"),
            messages = listOf(plan), busy = busy, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "test-model",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
            onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            handledPlans = handled, onPlanAction = { entry, selected -> check(entry == plan); action = selected; handled = setOf(planKey(entry)) }
        ) } } } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val end = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < end) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun click(label: String) {
            var n: AccessibilityNodeInfo? = await(label)
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true)
            waitForIdleSync(); Thread.sleep(300)
        }
        fun absent(label: String) {
            val end = System.currentTimeMillis() + 6000
            while (node(label) != null && System.currentTimeMillis() < end) { uiAutomation.serviceInfo = uiAutomation.serviceInfo; Thread.sleep(100) }
            check(node(label) == null) { "Still visible: $label" }
        }
        val choices = listOf("Implement in this session", "Clear context and implement in new chat", "Stay in Plan mode")
        await("Implement this plan?"); choices.forEach { await(it) }
        check(node("Stop") == null) { "Finished plan invented an active turn" }
        waitForIdleSync(); Thread.sleep(600)
        val screenshot = uiAutomation.takeScreenshot() ?: error("No screenshot")
        File(targetContext.filesDir, "plan-review.png").outputStream().use { screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; screenshot.recycle()
        for ((index, expected) in listOf("implement", "fresh", "stay").withIndex()) {
            runOnMainSync { handled = emptySet() }
            await("Implement this plan?"); click(choices[index])
            check(action == expected)
            absent("Implement this plan?")
        }
        runOnMainSync { handled = emptySet(); busy = true }
        absent("Implement this plan?")
        output.putString("result", "PASS: Markdown plan, three exact actions, dismissal, busy guard and no fake Stop")
        finish(-1, output)
    } catch (failure: Throwable) { output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, output) }
}
