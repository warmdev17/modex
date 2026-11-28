package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.runtime.MutableState
import org.json.JSONObject

/** Real reducer and shared status UI with synthetic host frames; no provider or host mutations. */
internal fun Instrumentation.antigravityContextSmoke() {
    val output = Bundle()
    val app = targetContext.applicationContext as ModexApplication
    val previous = app.session
    val storage = "antigravity-context-fixture"
    val vm = ModexViewModel(app, storage)
    try {
        val host = BackendCapabilities.fromReady(JSONObject("""{
          "ready":true,"instance":"fixture","backend":"codex",
          "capabilities":["session_status","agent_discovery","antigravity_context_usage"]
        }"""))
        fun field(name: String) = ModexViewModel::class.java.getDeclaredField(name).apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        fun <T> state(name: String, value: T) { (field(name + "\$delegate").get(vm) as MutableState<T>).value = value }
        val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val pending = field("pending").get(vm) as MutableMap<Long, Pair<String, String>>
        val thread = "agy:context-fixture"
        fun statusRequest() = pending.entries.last { it.value.first == "threads/status" }.key
        fun event(target: String, usage: Any) = apply.invoke(vm, JSONObject().put("event", "thread/tokenUsage/updated").put(
            "params", JSONObject().put("threadId", target).put("turnId", "turn-1").put("tokenUsage", usage)
        ))
        fun status(request: Long, target: String, used: Long) = apply.invoke(vm, JSONObject().put("id", request).put("result", JSONObject("""{
          "session":{"thread":{"id":"$target","cwd":"/fixture"}},
          "token_usage":{"last":{"totalTokens":$used},"modelContextWindow":1048576},"rate_limits":null
        }""")))

        runOnMainSync {
            field("config").set(vm, null)
            state("connected", true)
            state("backendCapabilities", host)
            state("threadAgents", mapOf(thread to ANTIGRAVITY_AGENT))
            state("selectedWorkspace", "/fixture")
            (field("sessions").get(vm) as SessionState).open(thread)
            field("ownedThread").set(vm, thread)

            vm.refreshSessionStatus()
            val stale = statusRequest()
            event(thread, JSONObject("""{"last":{"totalTokens":15709},"modelContextWindow":1048576}"""))
            check(vm.sessionStatus?.contextTokens == 15709L && vm.sessionStatus?.contextWindow == 1048576L) { "Live AGY context was not applied" }
            status(stale, thread, 200)
            check(vm.sessionStatus?.contextTokens == 15709L) { "Stale status replaced live AGY context" }

            event("agy:other", JSONObject("""{"last":{"totalTokens":99},"modelContextWindow":100}"""))
            check(vm.sessionStatus?.contextTokens == 15709L) { "Foreign AGY context was applied" }
            event(thread, JSONObject.NULL)
            check(vm.sessionStatus?.contextTokens == null && vm.sessionStatus?.contextWindow == null) { "AGY context reset was ignored" }

            vm.refreshSessionStatus()
            status(statusRequest(), thread, 700)
            check(vm.sessionStatus?.contextTokens == 700L && vm.sessionStatus?.workspace == "/fixture") { "AGY status values were not applied" }
            vm.refreshSessionStatus()
            status(statusRequest(), "agy:other", 900)
            check(vm.sessionStatus?.contextTokens == 700L) { "Mismatched AGY status was applied" }
        }

        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        runOnMainSync {
            activity.setContent { ModexTheme { ChatWorkspace(
                host = "Fixture", connected = true, connecting = false, selected = thread,
                threads = listOf(thread), titles = mapOf(thread to "Context fixture"), messages = emptyList(),
                busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "",
                sessionModel = "", decisions = emptyList(), answeringDecisionIds = emptySet(),
                onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
                onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
                backendCapabilities = host, sessionCapabilities = runtimeCapabilities(host, emptyList(), ANTIGRAVITY_AGENT),
                sessionAgent = ANTIGRAVITY_AGENT,
                sessionStatus = SessionStatus(workspace = "/fixture", contextTokens = 15709, contextWindow = 1048576),
                usageWarning = "Codex 5-hour usage limit reached.",
            ) } }
        }
        waitForIdleSync()
        fun node(text: String): AccessibilityNodeInfo? {
            fun walk(current: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (current == null) return null
                if (current.text?.toString() == text || current.contentDescription?.toString() == text) return current
                for (index in 0 until current.childCount) walk(current.getChild(index))?.let { return it }
                return null
            }
            val deadline = SystemClock.uptimeMillis() + 5000
            while (SystemClock.uptimeMillis() < deadline) {
                walk(uiAutomation.rootInActiveWindow)?.let { return it }
                SystemClock.sleep(100)
            }
            return null
        }
        check(node("Context 1% · 15,709 / 1,048,576") != null) { "AGY context missing from header" }
        check(node("Codex 5-hour usage limit reached.") == null) { "Codex quota banner leaked into AGY" }
        val opener = node("Open session status") ?: error("AGY status action missing")
        var clickable: AccessibilityNodeInfo? = opener
        while (clickable != null && !clickable.isClickable) clickable = clickable.parent
        check(clickable?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true)
        waitForIdleSync()
        check(node("Context window") != null && node("99% left") != null && node("15,709 used / 1,048,576 tokens") != null)
        check(node("Workspace: /fixture") != null)
        check(node("5h limit") == null && node("Approvals: Unavailable") == null && node("Mode: Unavailable") == null) { "Codex-only status rows leaked into AGY" }
        output.putString("result", "PASS: Antigravity context capability, live/status/reset/stale isolation, header and focused dialog")
        finish(-1, output)
    } catch (failure: Throwable) {
        output.putString("result", "FAIL: Antigravity context: ${failure.message}")
        finish(0, output)
    } finally {
        runOnMainSync {
            @Suppress("UNCHECKED_CAST")
            ((ModexViewModel::class.java.getDeclaredField("connected\$delegate").apply { isAccessible = true }.get(vm)) as MutableState<Boolean>).value = false
            vm.forget()
            app.sessions.remove(storage)
            app.session = previous
        }
    }
}
