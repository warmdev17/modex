package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.foundation.layout.*
import androidx.compose.ui.Modifier
import java.io.File

/** Disposable emulator fixture: compact headers and exact cross-host row routing. */
internal fun Instrumentation.activityNavigationSmoke() {
    val result = Bundle()
    try {
        val app = targetContext.applicationContext as ModexApplication
        check(app.connections.profiles.isEmpty()) { "Requires a disposable emulator without saved hosts" }
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val end = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < end) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun touch(n: AccessibilityNodeInfo) {
            val bounds = Rect().also(n::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = MotionEvent.obtain(time, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                sendPointerSync(event); event.recycle()
            }
            waitForIdleSync()
        }
        fun screenshot(name: String) {
            waitForIdleSync(); Thread.sleep(600)
            val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
            bmp.recycle()
        }
        fun compact(label: String) {
            val bounds = Rect().also(await(label)::getBoundsInScreen)
            check(bounds.height() / targetContext.resources.displayMetrics.density <= 57f) { "Oversized toolbar: $bounds" }
        }
        compact("Navigation toolbar"); screenshot("activity-home.png")
        touch(await("Add host")); await("Name")
        fun editable(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
            if (n == null) return null
            if (n.isEditable) return n
            for (i in 0 until n.childCount) editable(n.getChild(i))?.let { return it }
            return null
        }
        check(editable(uiAutomation.rootInActiveWindow)!!.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT,
            Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, "Fixture host") }))
        touch(await("Continue")); compact("Host settings toolbar"); screenshot("activity-host-settings.png")
        sendKeyDownUpSync(KeyEvent.KEYCODE_BACK); await("Add host")
        runOnMainSync { app.connections.profiles.toList().forEach { app.connections.remove(it.id) } }
        touch(await("Activity")); await("Nothing needs attention")
        var opened = ""
        runOnMainSync { activity.setContent { ModexTheme { Surface(Modifier.fillMaxSize().safeDrawingPadding(), color = Mocha.Base) {
            ActivityList(listOf(
                HostActivity("vps", "review", "Review database migration", true, 1),
                HostActivity("laptop", "build", "Build Android app", true, 0),
                HostActivity("offline", "old", "Previous task", true, 0)
            ), mapOf("vps" to "Work VPS", "laptop" to "My laptop", "offline" to "Offline computer"), setOf("vps", "laptop")) { host, thread -> opened = "$host:$thread" }
        } } } }
        await("1 waiting for input"); await("Running"); await("Reconnect to verify activity")
        screenshot("activity-sessions.png")
        touch(await("Review database migration")); runOnMainSync { check(opened == "vps:review") }
        touch(await("Build Android app")); runOnMainSync { check(opened == "laptop:build") }
        touch(await("Previous task")); runOnMainSync { check(opened == "offline:old") }
        result.putString("result", "PASS: compact Home/host settings, Back navigation, Activity empty/running/waiting/offline states and exact host/thread routing")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}")
        finish(0, result)
    }
}
