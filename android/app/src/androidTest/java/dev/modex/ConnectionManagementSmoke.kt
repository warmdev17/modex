package dev.modex

import android.app.ActivityManager
import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import android.view.KeyEvent
import android.view.MotionEvent
import android.graphics.Rect
import android.os.SystemClock
import android.util.Base64
import org.json.JSONObject
import java.io.File

internal fun Instrumentation.connectionManagementSmoke(args: Bundle) {
    val result = Bundle()
    try {
        val app = targetContext.applicationContext as ModexApplication
        val restore = args.getString("stage") == "restore"
        val manager = app.connections
        if (!restore) {
            check(manager.profiles.all { it.name in listOf("Fixture laptop", "Fixture VPS") }) { "This fixture requires a disposable emulator with no user hosts" }
            runOnMainSync { manager.profiles.toList().forEach { manager.remove(it.id) } }
            manager.reconnectOnOpen(false)
            val first = manager.add("Fixture laptop")!!
            val second = manager.add("Fixture VPS")!!
            val bytes = File(targetContext.filesDir, "smoke-key").readBytes()
            try {
                runOnMainSync {
                    for ((id, state) in listOf(first to args.getString("state")!!, second to args.getString("state2")!!)) {
                        val vm = manager.controller(id)
                        vm.importKey(bytes.copyOf())
                        vm.connect(Connection("10.0.2.2", args.getString("port")!!.toInt(), args.getString("user")!!,
                            args.getString("pin")!!, args.getString("binary")!!, state))
                    }
                }
            } finally { bytes.fill(0) }
        }
        val first = manager.profiles.first { it.name == "Fixture laptop" }
        val second = manager.profiles.first { it.name == "Fixture VPS" }
        lateinit var a: ModexViewModel; lateinit var b: ModexViewModel
        runOnMainSync { a = manager.controller(first.id); b = manager.controller(second.id) }
        fun await(condition: () -> Boolean) {
            val end = System.currentTimeMillis() + 25000
            while (true) {
                var ready = false
                runOnMainSync { ready = condition() }
                if (ready) return
                check(System.currentTimeMillis() < end) { "Timed out: A=${a.error} B=${b.error}" }
                Thread.sleep(80)
            }
        }
        if (restore) runOnMainSync { manager.reconnectOnOpen(true); manager.foreground() }
        await { a.connected && b.connected }
        runOnMainSync { if (a.selected.isBlank()) a.select("fixture-thread"); if (b.selected.isBlank()) b.select("fixture-thread") }
        await { !a.historyLoading && !b.historyLoading && a.messages.isNotEmpty() && b.messages.isNotEmpty() }
        if (!restore) {
            runOnMainSync {
                check(!a.configure(a.savedConnection!!.copy(host = "other.invalid")))
                check(a.savedConnection!!.host == "10.0.2.2")
                a.chooseModel("fixture-chosen", "high")
                check(b.sessionModel == "fixture" && b.sessionEffort == "low")
                a.disconnect()
                check(b.connected && a.messages.isEmpty() && b.messages.isNotEmpty())
                b.refreshThreads()
            }
            await { !b.threadListLoading && b.threads.isNotEmpty() }
            Thread.sleep(300)
            val services = targetContext.getSystemService(ActivityManager::class.java).getRunningServices(50)
            check(services.any { it.service.className == ConnectionService::class.java.name && it.foreground }) { "Disconnecting A stopped B's foreground service" }
            runOnMainSync { a.connect(a.savedConnection!!) }
            await { a.connected && !a.historyLoading && a.selected == "fixture-thread" }
        }
        runOnMainSync { check(a.sessionModel == "fixture-chosen" && a.sessionEffort == "high"); check(b.sessionModel == "fixture" && b.sessionEffort == "low") }
        startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        Thread.sleep(700)
        fun nodes(label: String): List<AccessibilityNodeInfo> {
            val found = mutableListOf<AccessibilityNodeInfo>()
            fun walk(n: AccessibilityNodeInfo?) {
                if (n == null) return
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) found += n
                for (i in 0 until n.childCount) walk(n.getChild(i))
            }
            walk(uiAutomation.rootInActiveWindow)
            return found
        }
        fun click(label: String, index: Int = 0) {
            val deadline = System.currentTimeMillis() + 5000
            while (nodes(label).size <= index && System.currentTimeMillis() < deadline) Thread.sleep(80)
            val matches = nodes(label)
            val node = if (label in listOf("Hosts", "Connections", "Settings")) matches.last() else matches.getOrNull(index) ?: error("Missing UI: $label")
            val bounds = Rect().also(node::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            // Native navigation tabs can expose selection without isClickable; exercise real touch.
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = MotionEvent.obtain(time, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                sendPointerSync(event); event.recycle()
            }
            waitForIdleSync()
            Thread.sleep(500)
        }
        fun awaitLabel(label: String) {
            val deadline = System.currentTimeMillis() + 5000
            while (nodes(label).isEmpty() && System.currentTimeMillis() < deadline) Thread.sleep(100)
            check(nodes(label).isNotEmpty()) { "UI did not show $label" }
        }
        click("Connections"); check(nodes("Open").size == 2) { "Expected two Open nodes, found ${nodes("Open").size}" }
        click("Settings"); awaitLabel("Reconnect on open")
        click("Hosts")
        click("Open")
        awaitLabel("Open sessions")
        runOnMainSync { check(a.connected && b.connected) }
        sendKeyDownUpSync(KeyEvent.KEYCODE_BACK); Thread.sleep(400)
        click("Open", 1)
        awaitLabel("Open sessions")
        runOnMainSync { check(a.connected && b.connected) }
        sendKeyDownUpSync(KeyEvent.KEYCODE_BACK); Thread.sleep(400)
        val screenshot = uiAutomation.takeScreenshot()!!
        File(targetContext.filesDir, "connections-home.png").outputStream().use { screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
        screenshot.recycle()
        if (restore) runOnMainSync {
            manager.remove(first.id)
            check(b.connected && b.savedConnection != null)
            manager.remove(second.id)
            check(manager.profiles.isEmpty() && !a.keyLoaded && !b.keyLoaded)
        }
        result.putString("result", "PASS: two independent remotes, model isolation, aggregate foreground service, encrypted ${if (restore) "process restore and removal" else "profile persistence"}")
        finish(-1, result)
    } catch (failure: Throwable) {
        uiAutomation.takeScreenshot()?.let { bmp -> File(targetContext.filesDir, "connections-failure.png").outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; bmp.recycle() }
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message} at ${failure.stackTrace.firstOrNull { it.className.contains("ConnectionManagement") }}")
        finish(0, result)
    }
}

internal fun Instrumentation.legacyConnectionSmoke() {
    val result = Bundle()
    try {
        val app = targetContext.applicationContext as ModexApplication
        val old = SavedHost(app)
        old.save(JSONObject().put("connection", JSONObject().put("host", "fixture.invalid").put("port", 22).put("user", "fixture")
            .put("pin", "SHA256:" + "A".repeat(43)).put("binary", "/fixture/modex-host").put("state", "/fixture/state"))
            .put("key", Base64.encodeToString("SYNTHETIC_PRIVATE_KEY".toByteArray(), Base64.NO_WRAP)).put("cursor", 123).put("instance", "fixture-instance"))
        val raw = File(app.noBackupFilesDir, "host.enc").readBytes()
        lateinit var manager: ConnectionManager
        runOnMainSync {
            manager = app.connections
            check(manager.profiles.single().id == "host")
            manager.reconnectOnOpen(false)
            val vm = manager.controller("host")
            check(vm.keyLoaded && vm.savedConnection!!.host == "fixture.invalid")
        }
        check(File(app.noBackupFilesDir, "host.enc").readBytes().contentEquals(raw)) { "Migration rewrote legacy credentials" }
        check(old.load()!!.getLong("cursor") == 123L)
        startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        Thread.sleep(600)
        val shot = uiAutomation.takeScreenshot()!!
        File(app.filesDir, "legacy-host-home.png").outputStream().use { shot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; shot.recycle()
        runOnMainSync { manager.remove("host") }
        check(old.load() == null)
        result.putString("result", "PASS: legacy encrypted host and cursor retained without rewriting credentials; Home migration")
        finish(-1, result)
    } catch (failure: Throwable) { result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message} at ${failure.stackTrace.firstOrNull { it.className.contains("ConnectionManagement") }}"); finish(0, result) }
}
