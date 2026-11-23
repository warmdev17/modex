package dev.modex

import android.app.Instrumentation
import android.os.Bundle
import androidx.compose.runtime.MutableState
import org.json.JSONArray
import org.json.JSONObject

/** Synthetic history and real ViewModel reducer; no SSH, keys or provider calls. */
internal fun Instrumentation.streamingPerformanceSmoke() {
    val output = Bundle()
    val storage = SavedHost(targetContext, "stream-performance-fixture")
    storage.clear()
    var failure: Throwable? = null
    var subject: ModexViewModel? = null
    val history = JSONArray().apply {
        repeat(1500) { put(JSONObject().put("id", "past-$it").put("role", "assistant").put("text", "History $it " + "sample ".repeat(140)).put("status", "completed").put("turn_id", "past-turn-$it")) }
    }
    runOnMainSync {
        val app = targetContext.applicationContext as ModexApplication
        val previous = app.session
        val vm = ModexViewModel(app, "stream-performance-fixture")
        subject = vm
        try {
            ModexViewModel::class.java.getDeclaredField("config").apply { isAccessible = true }.set(vm, null)
            @Suppress("UNCHECKED_CAST")
            val connected = ModexViewModel::class.java.getDeclaredField("connected\$delegate").apply { isAccessible = true }.get(vm) as MutableState<Boolean>
            @Suppress("UNCHECKED_CAST")
            val pending = ModexViewModel::class.java.getDeclaredField("pending").apply { isAccessible = true }.get(vm) as MutableMap<Long, Pair<String, String>>
            val journal = JournalState("stream-fixture", 100)
            ModexViewModel::class.java.getDeclaredField("journal").apply { isAccessible = true }.set(vm, journal)
            val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
            fun reply(method: String, result: JSONObject) {
                val id = pending.entries.last { it.value.first == method }.key
                apply.invoke(vm, JSONObject().put("id", id).put("result", result))
            }
            fun event(method: String, params: JSONObject) {
                val message = JSONObject().put("event", method).put("params", params)
                journal.frame(JSONObject().put("seq", journal.cursor + 1).put("message", message)) { apply.invoke(vm, it) }
            }
            connected.value = true
            vm.select("stream")
            reply("threads/resume", JSONObject("""{"thread":{"id":"stream","cwd":"/fixture"},"model":"fixture","collaboration_mode":"default"}"""))
            reply("threads/read", JSONObject().put("messages", history).put("active_turn", false).put("history_revision", "initial").put("next_cursor", JSONObject.NULL))
            event("turn/started", JSONObject("""{"threadId":"stream","turn":{"id":"live","status":"inProgress"}}"""))
            val samples = mutableListOf<Double>()
            repeat(80) {
                val params = JSONObject().put("threadId", "stream").put("turnId", "live").put("itemId", "answer").put("delta", "Streaming sample $it. ")
                val start = System.nanoTime()
                event("item/agentMessage/delta", params)
                samples += (System.nanoTime() - start) / 1_000_000.0
            }
            val p95 = samples.sorted()[75]
            val total = samples.sum()
            output.putString("timing", "80 deltas/1500 history: total=${total.toInt()}ms p95=${p95.toInt()}ms max=${samples.max().toInt()}ms")
            check(vm.messages.last().text.endsWith("Streaming sample 79. ")) { "Streaming text was lost" }
            check(p95 < 32 && total < 1600) { "Main-thread streaming budget exceeded: total=${total.toInt()}ms p95=${p95.toInt()}ms" }
            vm.disconnect()
            connected.value = true
            vm.select("stream")
            reply("threads/resume", JSONObject("""{"thread":{"id":"stream","cwd":"/fixture"},"model":"fixture"}"""))
            check(vm.messages.last().text.endsWith("Streaming sample 79. ")) { "Navigating away lost the streamed cache" }
            ModexViewModel::class.java.getDeclaredField("config").apply { isAccessible = true }.set(vm, Connection("fixture.invalid", 22, "fixture", "synthetic-pin", "/fixture/modex-host", "/fixture/state"))
            ModexViewModel::class.java.getDeclaredField("key").apply { isAccessible = true }.set(vm, "SYNTHETIC_STREAM_KEY".toByteArray())
            vm.disconnect()
        } catch (error: Throwable) { failure = error }
        finally { app.session = previous }
    }
    if (failure == null) {
        try {
            val deadline = System.currentTimeMillis() + 8000
            var restored: JSONObject? = null
            while (System.currentTimeMillis() < deadline && restored == null) {
                restored = storage.load()
                if (restored == null) Thread.sleep(50)
            }
            val snapshot = restored ?: error("VM snapshot was not persisted")
            val page = snapshot.getJSONObject("history_cache").getJSONObject("stream")
            check(page.getJSONArray("messages").getJSONObject(page.getJSONArray("messages").length() - 1).getString("text").endsWith("Streaming sample 79. ")) { "Persisted prefix was stale" }
            check(snapshot.getLong("cursor") == 181L && snapshot.getString("instance") == "stream-fixture" && page.getString("cursor").isEmpty()) { "Snapshot and cursor disagree" }
            runOnMainSync {
                val vm = checkNotNull(subject)
                ModexViewModel::class.java.getDeclaredMethod("persist").apply { isAccessible = true }.invoke(vm)
                vm.forget()
            }
            Thread.sleep(500)
            check(storage.load() == null) { "A pending VM save resurrected forgotten data" }
        } catch (error: Throwable) { failure = error }
    }
    storage.clear()
    if (failure == null) { output.putString("result", "PASS: large-history streaming reducer budget, exact final text, navigation cache, encrypted VM snapshot and forget cancellation"); finish(-1, output) }
    else { output.putString("result", "FAIL: ${failure?.message}"); finish(0, output) }
}
