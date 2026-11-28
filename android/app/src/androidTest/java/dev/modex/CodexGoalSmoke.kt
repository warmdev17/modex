package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import java.io.File
import org.json.JSONObject

/** Real goal reducer plus the focused Compose modal; synthetic frames never reach a provider. */
internal fun Instrumentation.codexGoalSmoke() {
    val output = Bundle()
    val app = targetContext.applicationContext as ModexApplication
    val previous = app.session
    val storage = "codex-goal-fixture"
    val vm = ModexViewModel(app, storage)
    try {
        fun field(name: String) = ModexViewModel::class.java.getDeclaredField(name).apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        fun <T> state(name: String, value: T) { (field(name + "\$delegate").get(vm) as MutableState<T>).value = value }
        val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val pending = field("pending").get(vm) as MutableMap<Long, Pair<String, String>>
        fun id(method: String) = pending.entries.last { it.value.first == method }.key
        fun reply(request: Long, result: String, failed: Boolean = false) {
            apply.invoke(vm, JSONObject().put("id", request).put(if (failed) "error" else "result", JSONObject(result)))
        }
        fun goal(thread: String, status: String, used: Long, updated: Long = 2) = """{"goal":{"thread_id":"$thread","objective":"Ship goal modal","status":"$status","token_budget":1200,"tokens_used":$used,"time_used_seconds":67,"created_at":1,"updated_at":$updated}}"""
        fun event(thread: String, status: String, used: Long, updated: Long = 2) {
            apply.invoke(vm, JSONObject("""{"event":"thread/goal/updated","params":{"threadId":"$thread","goal":{"threadId":"$thread","objective":"Ship goal modal","status":"$status","tokenBudget":1200,"tokensUsed":$used,"timeUsedSeconds":67,"createdAt":1,"updatedAt":$updated}}}"""))
        }
        val host = BackendCapabilities.fromReady(JSONObject("""{"backend":"codex","capabilities":["goals"]}"""))
        val thread = "goal-thread"
        runOnMainSync {
            field("config").set(vm, null)
            state("connected", true)
            state("backendCapabilities", host)
            state("threadAgents", mapOf(thread to CODEX_AGENT))
            state("selectedWorkspace", "/fixture")
            (field("sessions").get(vm) as SessionState).open(thread)
            field("ownedThread").set(vm, thread)

            vm.refreshGoal()
            reply(id("threads/goal/get"), """{"message":"request_rejected"}""", failed = true)
            check(vm.goalUncertain && !vm.goalLoading)
            vm.setGoal("Must not overwrite", "100")
            check(pending.values.none { it.first == "threads/goal/set" }) { "Unknown goal state allowed an upsert" }
            vm.refreshGoal()
            reply(id("threads/goal/get"), """{"goal":null}""")
            check(vm.goal == null && !vm.goalLoading && !vm.goalUncertain)

            vm.setGoal("Ship goal modal", "1200")
            val create = id("threads/goal/set")
            vm.setGoal("Duplicate", "200")
            check(id("threads/goal/set") == create) { "Duplicate goal mutation was dispatched" }
            reply(create, goal(thread, "active", 10))
            check(vm.goal?.status == "active" && vm.goal?.tokenBudget == 1200L)

            vm.refreshGoal()
            val stale = id("threads/goal/get")
            event(thread, "active", 42, 2)
            reply(stale, goal(thread, "active", 5, 2))
            check(vm.goal?.tokensUsed == 42L) { "Stale goal refresh replaced a native update" }
            event("other", "paused", 99)
            check(vm.goal?.tokensUsed == 42L) { "Foreign goal event changed this chat" }

            vm.setGoal(status = "paused")
            reply(id("threads/goal/set"), """{"message":"request_rejected"}""", failed = true)
            check(vm.goalUncertain && !vm.goalWorking)
            vm.setGoal(status = "paused")
            check(pending.values.none { it.first == "threads/goal/set" }) { "Uncertain mutation was retried" }
            vm.refreshGoal()
            reply(id("threads/goal/get"), goal(thread, "paused", 42, 4))
            check(!vm.goalUncertain && vm.goal?.status == "paused")
            vm.setGoal(status = "active")
            reply(id("threads/goal/set"), goal(thread, "active", 42, 5))
            check(vm.goal?.status == "active")

            vm.clearGoal()
            val clear = id("threads/goal/clear")
            apply.invoke(vm, JSONObject("""{"event":"thread/goal/cleared","params":{"threadId":"$thread"}}"""))
            reply(clear, """{"cleared":true}""")
            check(vm.goal == null && !vm.goalWorking)

            vm.refreshGoal()
            val oldGet = id("threads/goal/get")
            vm.select("other")
            reply(id("threads/resume"), """{"thread":{"id":"other","cwd":"/fixture","agent":"codex"}}""")
            val newGet = id("threads/goal/get")
            reply(oldGet, goal(thread, "active", 500))
            check(vm.selected == "other" && vm.goal == null && vm.goalLoading) { "Old chat goal response leaked" }
            reply(newGet, goal("other", "paused", 7))
            reply(id("threads/read"), """{"messages":[],"active_turn":false,"history_revision":"r1","next_cursor":null}""")
            check(vm.goal?.threadId == "other" && vm.goal?.tokensUsed == 7L)

            vm.setGoal(status = "active")
            val lateMutation = id("threads/goal/set")
            vm.disconnect()
            check(vm.selected.isBlank() && vm.goal == null && !vm.goalWorking && !vm.goalLoading)
            reply(lateMutation, goal("other", "active", 8))
            check(vm.selected.isBlank() && vm.goal == null) { "Late mutation acknowledgement restored disconnected goal state" }

            state("connected", true)
            state("backendCapabilities", host)
            state("threadAgents", mapOf("reopened" to CODEX_AGENT))
            state("selectedWorkspace", "/fixture")
            (field("sessions").get(vm) as SessionState).open("reopened")
            field("ownedThread").set(vm, "reopened")
            vm.refreshGoal()
            check(pending.values.count { it.first == "threads/goal/get" } == 1 && pending.values.none { it.first == "threads/goal/set" || it.first == "threads/goal/clear" }) { "Reconnect replayed a mutation" }
            reply(id("threads/goal/get"), """{"goal":null}""")
            check(vm.goal == null && !vm.goalLoading)
        }

        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var renderedGoal by mutableStateOf<Goal?>(null)
        var action = ""
        var submittedObjective = ""
        var submittedBudget = ""
        runOnMainSync {
            activity.setContent { ModexTheme { ChatWorkspace(
                host = "Fixture", connected = true, connecting = false, selected = thread,
                threads = listOf(thread), titles = mapOf(thread to "Goal fixture"), messages = emptyList(),
                busy = true, historyLoading = false, hasMore = false, error = "", sessionSettings = "",
                sessionModel = "fixture", decisions = emptyList(), answeringDecisionIds = emptySet(),
                onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
                onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
                backendCapabilities = host, sessionCapabilities = host,
                goal = renderedGoal, onGoalRefresh = { action = "refresh" },
                onGoalSet = { objective, budget, status ->
                    action = status.ifBlank { "create" }; submittedObjective = objective; submittedBudget = budget
                    renderedGoal = Goal(thread, objective.ifBlank { "Ship goal modal" }, status.ifBlank { "active" }, budget.toLongOrNull() ?: 1200, 345, 67, 1, 2)
                },
                onGoalClear = { action = "clear"; renderedGoal = null },
            ) } }
        }
        waitForIdleSync()

        fun find(match: (AccessibilityNodeInfo) -> Boolean): AccessibilityNodeInfo? {
            fun walk(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (node == null) return null
                if (match(node)) return node
                for (index in 0 until node.childCount) walk(node.getChild(index))?.let { return it }
                return null
            }
            val deadline = SystemClock.uptimeMillis() + 5000
            while (SystemClock.uptimeMillis() < deadline) {
                walk(uiAutomation.rootInActiveWindow)?.let { return it }
                SystemClock.sleep(100)
            }
            return null
        }
        fun node(text: String) = find { it.text?.toString() == text || it.contentDescription?.toString() == text }
        fun touch(text: String) {
            var target = node(text) ?: error("Missing $text")
            while (!target.isClickable && target.parent != null) target = target.parent
            check(target.performAction(AccessibilityNodeInfo.ACTION_CLICK)) { "Could not tap $text" }
            waitForIdleSync()
        }
        fun type(hint: String, value: String) {
            fun containsLabel(node: AccessibilityNodeInfo): Boolean =
                node.text?.contains(hint) == true || node.contentDescription?.contains(hint) == true ||
                    node.hintText?.contains(hint) == true ||
                    (0 until node.childCount).any { node.getChild(it)?.let(::containsLabel) == true }
            val target = find { it.isEditable && containsLabel(it) } ?: error("Missing $hint field")
            val args = Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, value) }
            check(target.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args))
            waitForIdleSync()
        }

        touch("Connection menu"); touch("Goal")
        check(node("No goal is configured for this chat.") != null)
        type("Objective", "Ship goal modal")
        type("Token budget (optional)", "1200")
        touch("Create goal")
        check(action == "create" && submittedObjective == "Ship goal modal" && submittedBudget == "1200")
        check(node("Active") != null && node("345 / 1,200 tokens") != null && node("1m 7s") != null)
        check(node("Active goals may continue when reopening this chat; Pause stops future continuation, Stop interrupts the current turn.") != null)
        uiAutomation.takeScreenshot()?.let { image ->
            File(targetContext.filesDir, "codex-goal-modal.png").outputStream().use {
                image.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
            }
            image.recycle()
        }
        touch("Pause")
        check(action == "paused" && node("Paused") != null)
        touch("Resume")
        check(action == "active" && node("Active") != null)
        touch("Clear goal"); touch("Clear")
        check(action == "clear" && node("No goal is configured for this chat.") != null)

        output.putString("result", "PASS: Codex goal create/budget, live/stale isolation, uncertain refresh, pause/resume, clear and focused modal")
        finish(-1, output)
    } catch (failure: Throwable) {
        output.putString("result", "FAIL: Codex goal: ${failure.message}")
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
