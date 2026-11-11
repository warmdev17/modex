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
                fun respond(id: Long, method: String, payload: String, tag: String = "") {
                    pending[id] = method to tag
                    apply.invoke(vm, JSONObject().put("id", id).put("result", JSONObject(payload)))
                }
                respond(1, "threads/resume", """{"thread":{"id":"old"}}""")
                check(vm.historyLoading) { "Resumed session must still request history" }
                respond(2, "threads/read", """{"messages":[{"role":"assistant","text":"Old session"}],"active_turn":true}""", "old")
                check(vm.messages.isNotEmpty() && vm.busy)
                respond(3, "threads/start", """{"thread":{"id":"new"}}""")
                check(vm.selected == "new")
                check(!vm.historyLoading) { "New chat must not read unpersisted history" }
                check(vm.messages.isEmpty()) { "New chat retained previous session messages" }
                check(!vm.busy) { "New chat retained previous session running status" }
                check(vm.error.isEmpty())
            } finally { app.session = previous }
        }
        result.putString("result", "PASS: new chat opens empty without history read; resume still loads history")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.message}")
        finish(0, result)
    }
}
