package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableStateOf

internal fun Instrumentation.antigravityOptionsSmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val options = mutableStateOf(listOf(
            AntigravityOption("model", "a", mapOf("a" to "Fixture model A", "b" to "Fixture model B")),
            AntigravityOption("thought_level", "high", mapOf("low" to "Low", "high" to "High")),
        ))
        val chat = "agy:fixture"
        val invocation = AgentActivity("completed", listOf(AgentTarget("agy-call:one", "Reviewer", "invocationCompleted")))
        runOnMainSync {
            activity.setContent { ModexTheme { Surface {
                ChatWorkspace(
                    host = "fixture", connected = true, connecting = false, selected = chat,
                    threads = listOf(chat), titles = emptyMap(),
                    messages = listOf(ChatEntry(id = "call-fixture", role = "agent_activity", agentActivity = invocation)),
                    busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "",
                    sessionModel = "", decisions = emptyList(), answeringDecisionIds = emptySet(),
                    onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
                    onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
                    sessionAgent = ANTIGRAVITY_AGENT,
                    backendCapabilities = BackendCapabilities.LegacyCodex.copy(agySessionOptions = true),
                    agyOptions = options.value,
                    onSetAgyOption = { id, value ->
                        options.value = options.value.map { if (it.id == id) it.copy(current = value) else it }
                    },
                    agentConversation = AgentConversation(),
                    onOpenAgent = { error("Invocations have no child conversation") },
                )
            } } }
        }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(info: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (info == null) return null
                if (info.isVisibleToUser && (info.text?.toString() == label || info.contentDescription?.toString() == label)) return info
                for (index in 0 until info.childCount) walk(info.getChild(index))?.let { return it }
                return null
            }
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = SystemClock.elapsedRealtime() + 6500
            while (SystemClock.elapsedRealtime() < deadline) {
                node(label)?.let { return it }
                SystemClock.sleep(100)
            }
            error("Missing $label")
        }
        fun tap(label: String) {
            var target = await(label)
            while (!target.isClickable) target = target.parent ?: error("Cannot click $label")
            check(target.performAction(AccessibilityNodeInfo.ACTION_CLICK))
            waitForIdleSync()
        }
        tap("Choose model")
        tap("Fixture model A")
        tap("Fixture model B")
        tap("High")
        tap("Low")
        tap("Done")
        await("Fixture model B · Low")
        tap("Agents")
        await("Reviewer")
        await("Invocation completed")
        check(node("Open agent conversation") == null)
        result.putString("result", "PASS: Antigravity acknowledged model and thinking choices, read-only invocation status")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.message}")
        finish(0, result)
    }
}
