package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.InputDevice
import android.view.MotionEvent
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
        var pending by mutableStateOf(false)
        val actions = mutableListOf<String>()
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Test host", connected = true, connecting = false, selected = "source", threads = listOf("source"), titles = mapOf("source" to "Plan review"),
            messages = listOf(plan), busy = busy, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "test-model",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
            onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            handledPlans = handled, planResponding = pending,
            onPlanAction = { entry, selected -> check(entry == plan); actions += selected; pending = true }
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
        fun actionBounds(label: String): Rect {
            var target = await(label)
            while (!target.isClickable) target = target.parent ?: error("No click target for $label")
            return Rect().also(target::getBoundsInScreen)
        }
        fun tapEnd(bounds: Rect) {
            val time = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = MotionEvent.obtain(time, SystemClock.uptimeMillis(), action, bounds.right - 12f, bounds.exactCenterY(), 0)
                event.source = InputDevice.SOURCE_TOUCHSCREEN
                try { check(uiAutomation.injectInputEvent(event, true)) } finally { event.recycle() }
            }
            Thread.sleep(200)
        }
        fun absent(label: String) {
            val end = System.currentTimeMillis() + 6000
            while (node(label) != null && System.currentTimeMillis() < end) { uiAutomation.serviceInfo = uiAutomation.serviceInfo; Thread.sleep(100) }
            check(node(label) == null) { "Still visible: $label" }
        }
        val choices = listOf("Implement in this session", "Clear context and implement in new chat", "Stay in Plan mode")
        await("Implement this plan?")
        val rows = choices.map(::actionBounds)
        val minTap = (48 * activity.resources.displayMetrics.density).toInt()
        check(rows.all { it.left == rows.first().left && it.right == rows.first().right && it.height() >= minTap }) {
            "Plan choices do not share a full-width 48dp touch target: $rows"
        }
        check(node("Submit answer") == null && node("Submit") == null)
        check(node("Stop response") == null) { "Finished plan invented an active turn" }
        waitForIdleSync(); Thread.sleep(600)
        val screenshot = uiAutomation.takeScreenshot() ?: error("No screenshot")
        File(targetContext.filesDir, "plan-review.png").outputStream().use { screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; screenshot.recycle()
        for ((index, expected) in listOf("implement", "fresh", "stay").withIndex()) {
            runOnMainSync { handled = emptySet() }
            await("Implement this plan?")
            tapEnd(actionBounds(choices[index]))
            await("Checking plan… Do not resend if disconnected.")
            check(actions.size == index + 1 && actions.last() == expected)
            val otherChoice = await(choices[(index + 1) % choices.size])
            tapEnd(Rect().also(otherChoice::getBoundsInScreen))
            check(actions.size == index + 1) { "A choice was sent twice while awaiting acknowledgement" }
            runOnMainSync { pending = false; handled = setOf(planKey(plan)) }
            absent("Implement this plan?")
        }
        runOnMainSync { handled = emptySet(); busy = true }
        absent("Implement this plan?")
        output.putString("result", "PASS: full-width one-tap plan actions, pending repeat guard, acknowledgement and busy suppression")
        finish(-1, output)
    } catch (failure: Throwable) { output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, output) }
}
