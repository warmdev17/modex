package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableStateOf
import java.io.File

internal fun Instrumentation.antigravityOptionsSmoke() {
    val result = Bundle()
    var stage = "open model picker"
    val requests = mutableListOf<Pair<String, String>>()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val opus = "Claude Opus 4.6"
        val gemini = "Gemini 3 Pro · Extended context preview model"
        val options = mutableStateOf(listOf(
            AntigravityOption("model", "a", mapOf("a" to opus, "b" to gemini)),
            AntigravityOption("thought_level", "high", mapOf("low" to "Low", "medium" to "Medium", "high" to "High")),
        ))
        val settingsLoading = mutableStateOf(false)
        val settingsError = mutableStateOf("")
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
                    agyOptionsLoading = settingsLoading.value,
                    agyOptionsUncertain = settingsLoading.value || settingsError.value.isNotEmpty(),
                    agyOptionsError = settingsError.value,
                    onRefreshAgyOptions = { settingsError.value = "" },
                    onSetAgyOption = { id, value ->
                        requests += id to value
                        if (id == "model") settingsLoading.value = true
                        else if (value == "medium") settingsError.value = "Settings change unconfirmed. Refresh before sending."
                        else options.value = options.value.map { if (it.id == id) it.copy(current = value) else it }
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
        tap(opus)
        await(gemini)
        uiAutomation.takeScreenshot()?.let { image ->
            File(targetContext.filesDir, "model-picker.png").outputStream().use {
                image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
            }
            image.recycle()
        }
        tap(gemini)
        stage = "pending model acknowledgement"
        await("Checking session settings…")
        check(node(gemini) == null)
        check(!await("Thinking level").isEnabled)
        runOnMainSync {
            check(requests == listOf("model" to "b"))
            options.value = listOf(
                options.value.first().copy(current = "b"),
                AntigravityOption("thought_level", "high", mapOf("low" to "Low", "medium" to "Medium", "high" to "Extended")),
            )
            settingsLoading.value = false
        }
        await("Thinking level · Extended")
        waitForIdleSync()
        val slider = await("Thinking level")
        val bounds = Rect().also(slider::getBoundsInScreen)
        stage = "drag thinking slider at $bounds"
        uiAutomation.takeScreenshot()?.let { image ->
            File(targetContext.filesDir, "model-level.png").outputStream().use {
                image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
            }
            image.recycle()
        }
        val y = bounds.centerY()
        val inset = bounds.width() / 8
        val drag = uiAutomation.executeShellCommand("input swipe ${bounds.right - inset} $y ${bounds.left + inset} $y 750")
        android.os.ParcelFileDescriptor.AutoCloseInputStream(drag).use { it.readBytes() }
        await("Thinking level · Low")
        runOnMainSync { check(requests == listOf("model" to "b", "thought_level" to "low")) }
        stage = "refresh after unconfirmed thinking change"
        check(await("Thinking level").performAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SET_PROGRESS.id,
            Bundle().apply { putFloat(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE, 1f) }))
        await("Settings change unconfirmed. Refresh before sending.")
        tap("Refresh")
        await("Thinking level · Low")
        stage = "recover an unlisted thinking level"
        runOnMainSync {
            options.value = options.value.map { if (it.id == "thought_level") it.copy(current = "auto") else it }
        }
        tap("Thinking level · auto")
        tap("Low")
        await("Thinking level · Low")
        stage = "close model settings and open agent activity"
        tap("Done")
        await("$gemini · Low")
        tap("Agents")
        await("Reviewer")
        await("Invocation completed")
        check(node("Open agent conversation") == null)
        result.putString("result", "PASS: Antigravity acknowledged model and thinking choices, read-only invocation status")
        finish(-1, result)
    } catch (failure: Throwable) {
        val calls = requests.toList()
        result.putString("result", "FAIL ($stage, requests=$calls): ${failure.message}")
        finish(0, result)
    }
}
