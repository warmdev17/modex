package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

/** Local callbacks only: no host, provider requests or conversation mutations. */
internal fun Instrumentation.nativeSessionControlsSmoke() {
    val output = Bundle()
    try {
        sessionControlReducerChecks()
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val selected = "source"
        val sent = mutableListOf<String>()
        val modeChanges = mutableListOf<String>()
        var statusRefreshes = 0
        var changing by mutableStateOf(false)
        var uncertain by mutableStateOf(false)
        var working by mutableStateOf(false)
        var status by mutableStateOf(SessionStatus(mode = "default", model = "fixture-model", effort = "medium", workspace = "/fixture", provider = "fixture", sandbox = "workspaceWrite", approvalPolicy = "on-request", approvalsReviewer = "user", contextTokens = 1200, totalTokens = 5400, contextWindow = 10000))
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Controls fixture", connected = true, connecting = false, selected = selected,
            threads = listOf("source", "other"), titles = mapOf("source" to "Source conversation", "other" to "Other conversation"),
            messages = listOf(ChatEntry(role = "Codex", text = "Completed fixture reply", status = "completed")),
            busy = working, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture-model", sessionEffort = "medium",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
            onSend = { text, _, _, _ -> sent += text; false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            sessionStatus = status, modeChanging = changing, modeUncertain = uncertain, onSessionStatus = { statusRefreshes++ }, onSessionMode = { mode -> modeChanges += mode; changing = true },
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
        fun shot(name: String) {
            waitForIdleSync(); Thread.sleep(500)
            val image = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; image.recycle()
        }
        await("Default")
        await("Context 12% · 1,200 / 10,000")
        touch("Default"); await("Changing mode…")
        check(modeChanges == listOf("plan")) { "Single tap did not request Plan: $modeChanges" }
        check(nodes("Plan").isEmpty()) { "Mode changed before host acknowledgement" }
        runOnMainSync { status = status.copy(mode = "plan"); changing = false }
        await("Plan"); shot("native-controls-plan.png")
        touch("Plan"); await("Changing mode…")
        check(modeChanges == listOf("plan", "default")) { "Second tap did not request Default" }
        runOnMainSync { status = status.copy(mode = "default"); changing = false }
        touch("Open session status"); await("Session status"); await("Mode: Default")
        await("Context: 1,200 / 10,000 tokens"); shot("native-controls-status.png")
        check(statusRefreshes == 1 && sent.isEmpty() && modeChanges.size == 2) { "Status performed a write" }
        touch("Close")
        for (draft in listOf("/", "/plan", "/help", "/literal/path")) {
            type(draft)
            check(nodes("Android commands").isEmpty() && nodes("Close command suggestions").isEmpty()) { "Slash suggestions remain" }
            touch("Send message")
        }
        check(sent == listOf("/", "/plan", "/help", "/literal/path")) { "Slash text was intercepted: $sent" }
        runOnMainSync { status = status.copy(mode = "", contextTokens = null, contextWindow = null) }
        await("Context unavailable"); touch("Enable Plan"); await("Changing mode…")
        check(modeChanges.last() == "plan") { "Fresh unknown mode could not enable Plan" }
        runOnMainSync { changing = false; uncertain = true }
        touch("Mode unknown · Refresh")
        check(statusRefreshes == 2 && modeChanges.size == 3) { "Uncertain mode was resent rather than refreshed" }
        runOnMainSync { uncertain = false; working = true; status = status.copy(mode = "default") }
        await("Default")
        val before = modeChanges.size
        touch("Default")
        check(modeChanges.size == before) { "Mode changed while a turn was running" }
        touch("Open session status"); await("Session status")
        check(statusRefreshes == 3) { "Read-only status unavailable during running turn" }
        touch("Close"); shot("native-controls-final.png")
        output.putString("result", "PASS: one-tap native mode with ACK/pending/fresh/uncertain/busy states, tappable read-only context/status, all slash drafts are plain messages")
        finish(-1, output)
    } catch (failure: Throwable) {
        uiAutomation.takeScreenshot()?.let { image -> File(targetContext.filesDir, "native-controls-failure.png").outputStream().use { image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; image.recycle() }
        val tree = StringBuilder()
        fun dump(node: AccessibilityNodeInfo?, depth: Int = 0) {
            if (node == null) return
            tree.append(" ".repeat(depth)).append("${node.text} | ${node.contentDescription} | ${node.className} | focus=${node.isFocused} | ${Rect().also(node::getBoundsInScreen)} | ${node.actionList.map { it.id }}\n")
            for (i in 0 until node.childCount) dump(node.getChild(i), depth + 1)
        }
        dump(uiAutomation.rootInActiveWindow); File(targetContext.filesDir, "native-controls-failure-tree.txt").writeText(tree.toString())
        output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, output)
    }
}

/** Real reducer with synthetic responses; no transport or saved host writes. */
private fun Instrumentation.sessionControlReducerChecks() {
    runOnMainSync {
        val app = targetContext.applicationContext as ModexApplication
        val previous = app.session
        val vm = ModexViewModel(app)
        try {
            ModexViewModel::class.java.getDeclaredField("config").apply { isAccessible = true }.set(vm, null)
            @Suppress("UNCHECKED_CAST")
            val connected = ModexViewModel::class.java.getDeclaredField("connected\$delegate").apply { isAccessible = true }.get(vm) as MutableState<Boolean>
            @Suppress("UNCHECKED_CAST")
            val pending = ModexViewModel::class.java.getDeclaredField("pending").apply { isAccessible = true }.get(vm) as MutableMap<Long, Pair<String, String>>
            val apply = ModexViewModel::class.java.getDeclaredMethod("apply", org.json.JSONObject::class.java).apply { isAccessible = true }
            fun id(method: String) = pending.entries.last { it.value.first == method }.key
            fun reply(request: Long, json: String, failed: Boolean = false) {
                apply.invoke(vm, org.json.JSONObject().put("id", request).put(if (failed) "error" else "result", org.json.JSONObject(json)))
            }
            connected.value = true
            vm.create("fixture-model", "host-default", "medium", "/fixture", "")
            reply(id("threads/start"), """{"thread":{"id":"fresh","cwd":"/fixture"},"model":"fixture-model","effort":"medium"}""")
            check(vm.selected == "fresh" && !vm.historyLoading && vm.sessionStatus?.mode.orEmpty().isBlank())
            reply(id("threads/status"), """{"session":{"thread":{"id":"fresh","cwd":"/fixture"},"model":"fixture-model"},"token_usage":null,"rate_limits":null}""")
            vm.changeSessionMode("plan")
            val firstMode = id("threads/mode")
            check(vm.modeChanging && pending.values.none { it.first == "turn/start" }) { "Fresh unknown mode was blocked or started a turn" }
            vm.changeSessionMode("default")
            check(id("threads/mode") == firstMode) { "Duplicate pending mode was dispatched" }
            check(!vm.send("/", "fixture-model", "medium")) { "Send allowed during mode ACK wait" }
            reply(firstMode, """{"message":"request_rejected"}""", true)
            check(vm.modeUncertain && !vm.modeChanging && vm.sessionStatus?.mode.orEmpty().isBlank()) { "Uncertain mode kept false confirmed state" }
            vm.changeSessionMode("plan")
            check(pending.values.none { it.first == "threads/mode" }) { "Uncertain mode was retried without refresh" }
            vm.refreshSessionStatus()
            reply(id("threads/status"), """{"session":{"thread":{"id":"fresh","cwd":"/fixture"},"model":"fixture-model"},"token_usage":null,"rate_limits":null}""")
            check(!vm.modeUncertain)
            vm.changeSessionMode("plan")
            reply(id("threads/mode"), """{"thread":{"id":"fresh","cwd":"/fixture"},"model":"fixture-model","collaboration_mode":"plan"}""")
            check(vm.sessionStatus?.mode == "plan" && !vm.modeChanging)
            fun settings(mode: String, effort: String = "high") {
                apply.invoke(vm, org.json.JSONObject("""{"event":"thread/settings/updated","params":{"threadId":"fresh","threadSettings":{"collaborationMode":{"mode":"$mode"},"model":"terminal-model","effort":"$effort"}}}"""))
            }
            settings("default")
            check(vm.sessionStatus?.mode == "default" && vm.sessionStatus?.effort == "high") { "Terminal settings were ignored" }
            vm.refreshSessionStatus()
            val staleSettingsStatus = id("threads/status")
            settings("plan", "low")
            reply(staleSettingsStatus, """{"session":{"thread":{"id":"fresh","cwd":"/fixture"},"model":"old-model","collaboration_mode":"default"},"token_usage":null,"rate_limits":null}""")
            check(vm.sessionStatus?.mode == "plan" && vm.sessionStatus?.model == "terminal-model") { "Older status overwrote native settings event" }
            vm.changeSessionMode("default")
            val staleModeAck = id("threads/mode")
            settings("plan", "medium")
            reply(staleModeAck, """{"thread":{"id":"fresh","cwd":"/fixture"},"model":"old-model","collaboration_mode":"default"}""")
            check(vm.sessionStatus?.mode == "plan" && vm.sessionStatus?.effort == "medium" && !vm.modeChanging) { "Older mode ACK overwrote newer terminal settings" }
            vm.refreshSessionStatus()
            val oldStatus = id("threads/status")
            vm.select("other")
            reply(id("threads/resume"), """{"thread":{"id":"other","cwd":"/fixture"},"model":"fixture-model","collaboration_mode":"default"}""")
            val newStatus = id("threads/status")
            check(oldStatus != newStatus && vm.sessionStatusLoading)
            reply(oldStatus, """{"session":{"thread":{"id":"fresh","cwd":"/fixture"},"model":"old-model","collaboration_mode":"plan"}}""")
            check(vm.selected == "other" && vm.sessionStatus?.mode == "default" && vm.sessionStatusLoading) { "Old-session status overwrote current mode/loading" }
            reply(newStatus, """{"session":{"thread":{"id":"other","cwd":"/fixture"},"model":"fixture-model","collaboration_mode":"default"},"token_usage":null,"rate_limits":null}""")
            reply(id("threads/read"), """{"messages":[],"active_turn":false,"history_revision":"r1","next_cursor":null}""")
            vm.changeSessionMode("plan")
            val lateMode = id("threads/mode")
            vm.disconnect()
            reply(lateMode, """{"thread":{"id":"other","cwd":"/fixture"},"model":"fixture-model","collaboration_mode":"plan"}""")
            check(vm.selected.isBlank() && vm.sessionStatus == null && !vm.modeChanging) { "Late ACK restored disconnected mode" }
            connected.value = false
        } finally { app.session = previous }
    }
}
