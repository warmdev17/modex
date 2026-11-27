package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.runtime.MutableState
import org.json.JSONObject

/** Real reducer and UI with synthetic host responses; no native conversation mutations. */
internal fun Instrumentation.antigravityTitleSmoke() {
    val result = Bundle()
    val app = targetContext.applicationContext as ModexApplication
    val previous = app.session
    val storage = "antigravity-title-fixture"
    val vm = ModexViewModel(app, storage)
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        fun field(name: String) = ModexViewModel::class.java.getDeclaredField(name).apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        fun <T> state(name: String, value: T) { (field(name + "\$delegate").get(vm) as MutableState<T>).value = value }
        runOnMainSync {
            field("config").set(vm, null)
            state("connected", true)
            val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
            @Suppress("UNCHECKED_CAST")
            val pending = field("pending").get(vm) as MutableMap<Long, Pair<String, String>>
            fun latest(method: String) = pending.entries.last { it.value.first == method }.key
            fun reply(id: Long, payload: String) { apply.invoke(vm, JSONObject().put("id", id).put("result", JSONObject(payload))) }
            fun event(thread: String, title: Any) {
                apply.invoke(vm, JSONObject().put("event", "thread/name/updated").put("params", JSONObject().put("threadId", thread).put("threadName", title)))
            }
            val thread = "agy:title-fixture"
            vm.select(thread)
            reply(latest("threads/resume"), """{"thread":{"id":"$thread","cwd":"/fixture","agent":"antigravity","name":"Opened by Antigravity"}}""")
            check(vm.titles[thread] == "Opened by Antigravity") { "AGY open title was ignored: ${vm.titles}" }
            reply(latest("threads/read"), """{"thread":{"id":"agy:other","name":"Wrong read title"},"messages":[],"active_turn":false,"history_revision":"wrong-title-r1","next_cursor":null}""")
            check("agy:other" !in vm.titles) { "Mismatched AGY read renamed another chat" }
            vm.select(thread)
            reply(latest("threads/resume"), """{"thread":{"id":"$thread","cwd":"/fixture","agent":"antigravity"}}""")
            reply(latest("threads/read"), """{"thread":{"id":"$thread","name":"Read from Antigravity"},"messages":[],"active_turn":false,"history_revision":"title-r1","next_cursor":null}""")
            check(vm.titles[thread] == "Read from Antigravity") { "AGY read title was ignored: ${vm.titles}" }
            vm.select(thread)
            val staleResume = latest("threads/resume")
            event(thread, "Live before resume")
            reply(staleResume, """{"thread":{"id":"$thread","cwd":"/fixture","agent":"antigravity","name":"Stale resume title"}}""")
            check(vm.titles[thread] == "Live before resume") { "In-flight resume replaced the live title: ${vm.titles}" }
            reply(latest("threads/read"), """{"thread":{"id":"$thread"},"messages":[],"active_turn":false,"history_revision":"title-r2","next_cursor":null}""")
            event("agy:unknown", "Unknown title")
            check("agy:unknown" !in vm.titles) { "Unknown AGY event created a title" }
            state("threadAgents", vm.threadAgents + ("codex:known" to CODEX_AGENT))
            event("codex:known", "Wrong runtime title")
            check("codex:known" !in vm.titles) { "AGY title event renamed another runtime" }
            vm.select(thread)
            reply(latest("threads/resume"), """{"thread":{"id":"$thread","cwd":"/fixture","agent":"antigravity"}}""")
            val staleRead = latest("threads/read")
            vm.refreshThreads()
            val staleList = latest("threads/list")
            event(thread, "Live Antigravity title")
            for (invalid in listOf<Any>(7, "bad\nname", "é".repeat(257))) {
                event(thread, invalid)
                check(vm.titles[thread] == "Live Antigravity title") { "Invalid AGY event replaced the title: ${vm.titles}" }
            }
            reply(staleRead, """{"thread":{"id":"$thread","name":"Stale read title"},"messages":[],"active_turn":false,"history_revision":"title-r3","next_cursor":null}""")
            check(vm.titles[thread] == "Live Antigravity title") { "In-flight read replaced the live title: ${vm.titles}" }
            reply(staleList, """{"data":[{"id":"$thread","cwd":"/fixture","agent":"antigravity","name":"Stale list title"},{"id":"codex:preview","cwd":"/fixture","agent":"codex","name":"","preview":"First preview"}],"next_cursor":null}""")
            check(vm.titles[thread] == "Live Antigravity title") { "In-flight list replaced the live title: ${vm.titles}" }
            check(vm.titles["codex:preview"] == "First preview")
            vm.refreshThreads()
            reply(latest("threads/list"), """{"data":[{"id":"$thread","cwd":"/fixture","agent":"antigravity","name":"","preview":"Snapshot fallback"},{"id":"codex:preview","cwd":"/fixture","agent":"codex","name":"","preview":"Updated preview"}],"next_cursor":null}""")
            check(vm.titles[thread] == "Live Antigravity title") { "Blank snapshot replaced the live title: ${vm.titles}" }
            check(vm.titles["codex:preview"] == "Updated preview") { "Blank-name Codex preview was frozen: ${vm.titles}" }
            activity.setContent { ModexTheme { HostWorkspace(vm, "Fixture host", false, {}, {}) } }
        }
        fun nodes(label: String): List<AccessibilityNodeInfo> {
            val matches = mutableListOf<AccessibilityNodeInfo>()
            fun walk(node: AccessibilityNodeInfo?) {
                if (node == null) return
                if (node.text?.toString() == label || node.contentDescription?.toString() == label) matches += node
                for (index in 0 until node.childCount) walk(node.getChild(index))
            }
            walk(uiAutomation.rootInActiveWindow)
            return matches
        }
        fun awaitNode(label: String, action: Int? = null): AccessibilityNodeInfo {
            val end = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < end) {
                for (match in nodes(label)) {
                    var current: AccessibilityNodeInfo? = match
                    while (current != null) {
                        if (action == null || current.actionList.any { it.id == action }) return current
                        current = current.parent
                    }
                }
                Thread.sleep(100)
            }
            error("Missing $label${action?.let { " (action $it)" }.orEmpty()}")
        }
        fun touch(node: AccessibilityNodeInfo) {
            val bounds = Rect().also(node::getBoundsInScreen)
            val downTime = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val motion = MotionEvent.obtain(downTime, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                try { sendPointerSync(motion) } finally { motion.recycle() }
            }
        }
        awaitNode("Live Antigravity title")
        touch(awaitNode("Open sessions", AccessibilityNodeInfo.ACTION_CLICK))
        awaitNode("Close sessions")
        awaitNode("Live Antigravity title", AccessibilityNodeInfo.ACTION_CLICK)
        result.putString("result", "PASS: AGY open/read/live titles, stale snapshots, validation, Codex preview refresh, header and sidebar")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}")
        finish(0, result)
    } finally {
        runOnMainSync { vm.forget(); app.sessions.remove(storage); app.session = previous }
    }
}
