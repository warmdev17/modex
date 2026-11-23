package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import android.view.MotionEvent
import android.view.WindowInsets
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableStateOf
import org.json.JSONObject

internal fun Instrumentation.agentActivitySmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val selected = mutableStateOf("agents")
        val state = SessionState().apply { open("agents") }
        fun event(id: String, action: String, targets: String) = state.event("item/completed", JSONObject(
            """{"threadId":"agents","turnId":"turn","item":{"id":"$id","type":"agentActivity","action":"$action","targets":$targets}}"""
        ))
        runOnMainSync {
            state.sent(ChatEntry(id = "fixture-prompt", role = "You", text = "Check reviewer and tester progress."))
            state.event("turn/started", JSONObject("""{"threadId":"agents","turn":{"id":"turn"}}"""))
            event("spawn", "started", """[{"id":"one","label":"reviewer","status":"running"},{"id":"two","label":"tester","status":"running"}]""")
            activity.setContent { ModexTheme { Surface { ChatWorkspace(
                host = "Agents fixture", connected = true, connecting = false, selected = selected.value,
                threads = listOf("agents", "other"), titles = emptyMap(), messages = state.messages,
                busy = state.busy, activeTurnId = state.activeTurnId,
                historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture",
                decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {},
                onCreate = { _, _, _ -> }, onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            ) } } }
        }
        fun node(label: String): AccessibilityNodeInfo? {
            fun find(info: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (info == null) return null
                if (info.text?.toString() == label || info.contentDescription?.toString() == label) return info
                for (index in 0 until info.childCount) find(info.getChild(index))?.let { return it }
                return null
            }
            return find(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = SystemClock.uptimeMillis() + 6500
            while (SystemClock.uptimeMillis() < deadline) {
                node(label)?.takeIf { it.isVisibleToUser }?.let { return it }
                Thread.sleep(100)
            }
            error("Missing $label")
        }
        fun tap(target: AccessibilityNodeInfo): Rect {
            val bounds = Rect().also(target::getBoundsInScreen)
            val now = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                MotionEvent.obtain(now, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0).also {
                    sendPointerSync(it)
                    it.recycle()
                }
            }
            return bounds
        }
        fun screenshot(name: String) {
            ParcelFileDescriptor.AutoCloseInputStream(uiAutomation.executeShellCommand("screencap -p /data/local/tmp/$name"))
                .use { it.readBytes() }
        }
        tap(await("Message Codex"))
        val imeDeadline = SystemClock.uptimeMillis() + 6000
        while (!activity.window.decorView.rootWindowInsets.isVisible(WindowInsets.Type.ime()) && SystemClock.uptimeMillis() < imeDeadline) Thread.sleep(100)
        check(activity.window.decorView.rootWindowInsets.isVisible(WindowInsets.Type.ime())) { "Real keyboard did not open" }
        val stop = Rect().also(await("Stop response")::getBoundsInScreen)
        val keyboard = activity.window.decorView.rootWindowInsets.getInsets(WindowInsets.Type.ime()).bottom
        check(stop.bottom <= activity.window.decorView.height - keyboard) { "Stop overlaps keyboard" }
        screenshot("agents-keyboard.png")
        var button = await("Agents 2")
        while (!button.isClickable) button = button.parent ?: error("Agents has no click action")
        check(button.stateDescription?.toString() == "2 running")
        val bounds = tap(button)
        val minimum = (48 * activity.resources.displayMetrics.density).toInt() - 1
        check(bounds.height() >= minimum && bounds.width() >= minimum && bounds.bottom <= stop.top)
        await("Agent activity")
        await("reviewer")
        await("tester")
        runOnMainSync {
            event("interact", "assigned", """[{"id":"one","status":"running"}]""")
            event("complete", "completed", """[{"id":"one","status":"completed"}]""")
            event("wait", "waited", """[{"id":"two","status":"errored"}]""")
            state.event("turn/completed", JSONObject("""{"threadId":"agents","turn":{"id":"turn","status":"completed"}}"""))
        }
        await("Received a task")
        await("Error")
        screenshot("agents-sheet.png")
        tap(await("Close agent activity"))
        await("Send message")
        tap(await("Agents"))
        await("Agent activity")
        runOnMainSync { state.open("other"); selected.value = "other" }
        await("Message Codex")
        check(node("Agent activity") == null && node("Agents") == null)
        result.putString("result", "PASS: compact Agents button with IME, accessible counts, live timeline, retained entry and isolated chat")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}")
        finish(0, result)
    }
}
