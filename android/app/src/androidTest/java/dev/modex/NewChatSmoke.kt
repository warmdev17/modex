package dev.modex

import android.app.Instrumentation
import android.os.Bundle
import org.json.JSONObject

/** Response fixture: a fresh Codex thread has no persisted history yet. */
internal fun Instrumentation.newChatSmoke() {
    val result = Bundle()
    try {
        runOnMainSync {
            val app = targetContext.applicationContext as ModexApplication
            val previous = app.session
            val vm = ModexViewModel(app)
            try {
                @Suppress("UNCHECKED_CAST")
                val pending = ModexViewModel::class.java.getDeclaredField("pending").apply { isAccessible = true }
                    .get(vm) as MutableMap<Long, Pair<String, String>>
                val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
                // Keep this reducer fixture isolated from the user's saved connection.
                ModexViewModel::class.java.getDeclaredField("config").apply { isAccessible = true }.set(vm, null)
                @Suppress("UNCHECKED_CAST")
                val connected = ModexViewModel::class.java.getDeclaredField("connected\$delegate").apply { isAccessible = true }.get(vm) as androidx.compose.runtime.MutableState<Boolean>
                connected.value = true
                fun reply(id: Long, payload: String, error: Boolean = false) {
                    apply.invoke(vm, JSONObject().put("id", id).put(if (error) "error" else "result", JSONObject(payload)))
                }
                fun requestId(method: String) = pending.entries.last { it.value.first == method }.key
                vm.select("old")
                reply(requestId("threads/resume"), """{"thread":{"id":"old"}}""")
                check(vm.historyLoading) { "Resumed session must still request history" }
                reply(requestId("threads/read"), """{"messages":[{"id":"old-message","role":"assistant","text":"Old session"}],"active_turn":true,"history_revision":"r1","next_cursor":null}""")
                check(vm.messages.isNotEmpty() && vm.busy)
                vm.create("", "host-default")
                reply(requestId("threads/start"), """{"thread":{"id":"new"}}""")
                check(vm.selected == "new" && !vm.historyLoading && vm.messages.isEmpty() && !vm.busy)
                vm.select("old")
                val lateResume = requestId("threads/resume")
                vm.disconnect()
                check(vm.selected.isEmpty() && vm.messages.isEmpty())
                connected.value = true
                reply(lateResume, """{"thread":{"id":"old"}}""")
                check(vm.selected.isEmpty()) { "Late resume reopened disconnected chat" }
                vm.select("old")
                reply(requestId("threads/resume"), """{"message":"active_writer"}""", true)
                check(vm.selected.isEmpty() && vm.messages.isEmpty() && vm.takeoverThread == "old") { "CLI ownership must require explicit takeover" }
                vm.select("old")
                reply(requestId("threads/resume"), """{"thread":{"id":"old"}}""")
                check(vm.messages.isNotEmpty()) { "Successful resume should reopen cache" }
                val lateHistory = requestId("threads/read")
                vm.disconnect()
                connected.value = true
                reply(lateHistory, """{"messages":[{"role":"assistant","text":"Late"}],"history_revision":"r2","next_cursor":null}""")
                check(vm.selected.isEmpty() && vm.messages.isEmpty()) { "Late history restored disconnected screen" }
                connected.value = false
            } finally { app.session = previous }
        }
        result.putString("result", "PASS: fresh chat, cached resume, disconnect clearing, stale replies and explicit CLI takeover")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.message}")
        finish(0, result)
    }
}
