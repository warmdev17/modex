package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import androidx.lifecycle.ViewModelProvider
import java.io.File

fun Instrumentation.continuitySmoke(args: Bundle) {
    val result = Bundle()
    var vm: ModexViewModel? = null
    try {
        val restore = args.getString("stage") == "restore"
        if (!restore) SavedHost(targetContext).clear()
        startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        val app = targetContext.applicationContext as ModexApplication
        runOnMainSync { vm = ViewModelProvider(app, ViewModelProvider.AndroidViewModelFactory.getInstance(app))[ModexViewModel::class.java] }
        val session = vm!!
        if (restore) runOnMainSync { session.foreground() }
        if (!restore) runOnMainSync {
            session.importKey(File(targetContext.filesDir, "smoke-key").readBytes())
            session.connect(Connection("10.0.2.2", args.getString("port")!!.toInt(), args.getString("user")!!, args.getString("pin")!!, args.getString("binary")!!, args.getString("state")!!))
        }
        fun await(condition: () -> Boolean) {
            val deadline = System.currentTimeMillis() + 20000
            while (true) {
                var ready = false
                runOnMainSync { ready = condition() }
                if (ready) return
                check(System.currentTimeMillis() < deadline) { "State timeout" }
                Thread.sleep(100)
            }
        }
        await { session.connected }
        if (!restore) {
            runOnMainSync { session.select("fixture-thread") }
        }
        await { session.selected == "fixture-thread" && !session.historyLoading && session.messages.any { it.second == "Stored assistant reply" } }
        Thread.sleep(700)
        val screenshot = uiAutomation.takeScreenshot() ?: error("No screenshot")
        File(targetContext.filesDir, "continuity-chat.png").outputStream().use { screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
        screenshot.recycle()
        if (restore) runOnMainSync { check(session.keyLoaded); session.forget() }
        result.putString("result", "PASS: ${if (restore) "process restore and history" else "history and saved host"}")
        finish(-1, result)
    } catch (failure: Throwable) {
        runOnMainSync { vm?.forget() }
        result.putString("result", "FAIL: continuity smoke " + failure.javaClass.simpleName + ": " + failure.message)
        finish(0, result)
    }
}
