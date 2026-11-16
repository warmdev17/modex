package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.util.Base64
import java.io.File
import org.json.JSONArray
import org.json.JSONObject

/** Only the isolated SSH/fake-provider fixture supplies this profile. */
internal fun Instrumentation.journalRecoverySmoke(args: Bundle) {
    val output = Bundle()
    var vm: ModexViewModel? = null
    try {
        val config = Connection("10.0.2.2", args.getString("port")!!.toInt(), args.getString("user")!!,
            args.getString("pin")!!, args.getString("binary")!!, args.getString("state")!!)
        val connection = JSONObject().put("host", config.host).put("port", config.port).put("user", config.user)
            .put("pin", config.pin).put("binary", config.binary).put("state", config.state)
        val store = SavedHost(targetContext, "journal-fixture")
        store.clear()
        val keyPath = args.getString("keyPath")!!
        require(keyPath.startsWith("/data/local/tmp/modex-journal-") && keyPath.matches(Regex("[A-Za-z0-9_./-]+")))
        val key = android.os.ParcelFileDescriptor.AutoCloseInputStream(uiAutomation.executeShellCommand("cat " + keyPath)).use { it.readBytes() }
        store.save(JSONObject().put("connection", connection)
            .put("key", Base64.encodeToString(key, Base64.NO_WRAP))
            .put("instance", args.getString("instance")).put("cursor", 1).put("selected", "fixture-thread")
            .put("model_choices", JSONObject().put("fixture-thread", ModelChoice("fixture-chosen", "high").toJson()))
            .put("pending", JSONArray().put(JSONObject().put("id", 1234).put("method", "turn/start").put("tag", "fixture-thread"))))
        key.fill(0)
        startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        val app = targetContext.applicationContext as ModexApplication
        runOnMainSync { vm = ModexViewModel(app, "journal-fixture") }
        val session = vm!!
        // Kotlin internal getter names differ between debug tests and the release APK.
        val messages = session.javaClass.methods.single { it.name.substringBefore('$') == "getMessages" }
        fun restoredReply() = (messages.invoke(session) as List<*>).any { it is ChatEntry && it.text == "Stored assistant reply" }
        fun await(condition: () -> Boolean) {
            val deadline = System.currentTimeMillis() + 20000
            while (true) {
                var ready = false
                runOnMainSync { ready = condition() }
                if (ready) return
                check(System.currentTimeMillis() < deadline) { "Recovery timeout: connected=" + session.connected + " loading=" + session.historyLoading + " error=" + session.error }
                Thread.sleep(100)
            }
        }
        runOnMainSync { session.foreground() }
        await { !session.connected && !session.connecting && session.error.contains("Reset session cursor") }
        runOnMainSync { session.reset(); session.connect(config) }
        await { session.connected && session.selected == "fixture-thread" && !session.historyLoading &&
            restoredReply() && session.decisions.size == 2 }
        runOnMainSync {
            check(session.busy) { "Running turn was not restored" }
            check(session.sessionModel == "fixture-chosen" && session.sessionEffort == "high")
            val approval = session.decisions.single { it.getString("method") == "item/commandExecution/requestApproval" }
            check(approval.getJSONObject("context").getString("command") == "printf JOURNAL_REPLAY_OK")
            val question = session.decisions.single { it.getString("method") == "item/tool/requestUserInput" }
            check(question.getJSONObject("context").getJSONArray("questions").getJSONObject(0).getString("id") == "choice")
            session.answer(approval, JSONObject().put("decision", "accept"))
        }
        await { session.decisions.size == 1 && session.answeringDecisionIds.isEmpty() }
        runOnMainSync { session.disconnect() }
        await { !session.connected && !session.connecting }
        // Reset again while retention is still exceeded: the resolved approval must stay gone.
        runOnMainSync { session.reset(); session.connect(config) }
        await { session.connected && session.selected == "fixture-thread" && !session.historyLoading && session.decisions.size == 1 }
        runOnMainSync {
            check(session.busy && restoredReply())
            check(session.decisions.single().getString("method") == "item/tool/requestUserInput")
            check(session.sessionModel == "fixture-chosen" && session.sessionEffort == "high")
        }
        val screenshot = uiAutomation.takeScreenshot() ?: error("No screenshot")
        File(targetContext.getExternalFilesDir(null), "journal-recovery.png").outputStream().use {
            screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
        }
        screenshot.recycle()
        output.putString("result", "PASS: expired cursor, reset/checkpoint, active history, model/effort, pending approval/question and resolved-decision reconciliation")
        finish(-1, output)
    } catch (failure: Throwable) {
        output.putString("result", "FAIL: journal recovery " + failure.javaClass.simpleName + ": " + failure.message)
        finish(0, output)
    } finally {
        runOnMainSync { vm?.forget() }
    }
}
