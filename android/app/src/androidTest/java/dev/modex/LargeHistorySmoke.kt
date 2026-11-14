package dev.modex

import android.app.Instrumentation
import android.os.Bundle
import android.util.Base64
import org.json.JSONObject
import java.util.Timer
import kotlin.concurrent.schedule

/** Read-only real-host regression; never resumes a thread or changes the saved profile. */
fun Instrumentation.largeHistorySmoke(args: Bundle) {
    val output = Bundle()
    val transport = SshTransport()
    val timer = Timer(true)
    var key: ByteArray? = null
    try {
        val data = SavedHost(targetContext).load() ?: error("No saved emulator profile")
        val c = data.getJSONObject("connection")
        for (field in listOf("host", "user", "port", "pin", "binary", "state")) {
            check(c.get(field).toString() == args.getString(field)) { "Saved profile differs from emulator.json: $field" }
        }
        val config = Connection(c.getString("host"), c.getInt("port"), c.getString("user"), c.getString("pin"), c.getString("binary"), c.getString("state"))
        val bytes = Base64.decode(data.getString("key"), Base64.NO_WRAP)
        key = bytes
        val journal = JournalState()
        var count = 0
        var size = 0
        var healthy = false
        timer.schedule(60000) { transport.close() }
        transport.open(config, bytes, journal, onHello = {
            transport.send(JSONObject().put("id", 198001).put("method", "threads/read").put("params", JSONObject().put("thread_id", args.getString("thread")).put("include_turns", true)))
        }, onFrame = { frame ->
            journal.frame(frame) { message ->
                when (message.optInt("id")) {
                    198001 -> {
                        check(!message.has("error")) { "History request rejected" }
                        count = message.getJSONObject("result").getJSONArray("messages").length()
                        size = frame.toString().toByteArray(Charsets.UTF_8).size
                        check(count > 0 && size > 4 * 1024 * 1024) { "Fixture must exercise history over 4 MiB" }
                        transport.send(JSONObject().put("id", 198002).put("method", "status"))
                    }
                    198002 -> { healthy = message.has("result"); transport.close() }
                }
            }
        })
        check(healthy) { "Host did not answer status after large history" }
        output.putString("result", "PASS: emulator.json profile, $count messages, $size bytes, host still responds")
        finish(-1, output)
    } catch (e: Throwable) {
        // Only fixture checks/ConnectionFailure are safe to display; never echo raw transport errors.
        output.putString("result", "FAIL: large history (${e.javaClass.simpleName})")
        finish(0, output)
    } finally {
        timer.cancel(); transport.close(); key?.fill(0)
    }
}
