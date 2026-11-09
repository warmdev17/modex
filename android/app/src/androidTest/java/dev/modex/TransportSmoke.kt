package dev.modex

import android.app.Instrumentation
import android.os.Bundle
import java.io.File
import org.json.JSONObject

/** Explicitly launched by scripts/test_android_transport.py against an ephemeral sshd. */
class TransportSmoke : Instrumentation() {
    private lateinit var args: Bundle
    override fun onCreate(arguments: Bundle) { super.onCreate(arguments); args = arguments; start() }
    override fun onStart() {
        if (args.getString("preview") == "approval") { approvalPreview(); return }
        val result = Bundle()
        try {
            val config = Connection("10.0.2.2", args.getString("port")!!.toInt(), args.getString("user")!!,
                args.getString("pin")!!, args.getString("binary")!!, args.getString("state")!!)
            val key = File(targetContext.filesDir, "smoke-key").readBytes()
            try {
                val journal = JournalState()
                for (id in 900001..900002) {
                    var received = false
                    val transport = SshTransport()
                    try {
                        transport.open(config, key, journal, onHello = {
                            transport.send(JSONObject().put("id", id).put("method", "status"))
                        }, onFrame = { frame ->
                            journal.frame(frame) { message ->
                                if (message.optInt("id") == id) {
                                    check(message.getJSONObject("result").getJSONObject("config").getString("provider") == "fixture")
                                    received = true
                                }
                            }
                            if (received) transport.close()
                        })
                    } catch (e: Exception) { if (!received) throw e }
                    finally { transport.close() }
                    check(received && journal.cursor > 0)
                    Thread.sleep(200)
                }
                var trusted = false
                val wrong = SshTransport()
                try {
                    wrong.open(config.copy(pin = fingerprint(byteArrayOf())), key, JournalState(), { trusted = true }, {})
                } catch (_: Exception) { /* Expected host key rejection. */ }
                finally { wrong.close() }
                check(!trusted)
                result.putString("result", "PASS: Android SSH status, cursor reconnect and wrong-pin rejection")
            } finally { key.fill(0); File(targetContext.filesDir, "smoke-key").delete() }
            finish(-1, result)
        } catch (_: Throwable) {
            result.putString("result", "FAIL: Android transport smoke; inspect fixture and protocol")
            finish(0, result)
        }
    }
}
