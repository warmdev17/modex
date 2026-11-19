package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Rect
import android.os.Bundle
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.view.MotionEvent
import android.view.WindowInsets
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import java.io.File
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/** Reproduce tall composer + IME against real history, without a host or provider. */
internal fun Instrumentation.composerResponsivenessSmoke(args: Bundle) {
    val result = Bundle()
    val trace = File(targetContext.filesDir, "composer-watchdog.txt").apply { delete() }
    fun heartbeat(phase: String) {
        val latch = CountDownLatch(1)
        Handler(Looper.getMainLooper()).post { latch.countDown() }
        if (!latch.await(3, TimeUnit.SECONDS)) {
            trace.writeText("UI unresponsive: $phase\n" + Looper.getMainLooper().thread.stackTrace.joinToString("\n"))
            sendStatus(0, Bundle().apply { putString("watchdog", "FAIL: UI unresponsive: $phase") })
            error("UI unresponsive: $phase")
        }
    }
    try {
        val picture = File(targetContext.cacheDir, "composer-fixture.png")
        Bitmap.createBitmap(1, 1, Bitmap.Config.ARGB_8888).let { bitmap ->
            picture.outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }; bitmap.recycle()
        }
        val references = (0..15).map { PromptReference("skill", "fixture$it", "/fixture/$it/SKILL.md", "Fixture skill $it", "Synthetic reference", "fixture", true, "@fixture$it") }
        val history = (0..39).map { ChatEntry("past-$it", "You", "History message $it\nAnother line") } + ChatEntry("last", "Codex", "RESPONSIVENESS_END", status = "completed")
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Responsiveness fixture", connected = true, connecting = false, selected = "source", threads = listOf("source"), titles = emptyMap(),
            messages = history, busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
            onSend = { _, _, _, _ -> error("Fixture must never send") }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            referenceCatalog = ReferenceCatalog(references, listOf("skill")),
            attachments = listOf(Attachment(picture.path, picture.name, "image")), loadAttachment = { picture }
        ) } } } }
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
        val deadline = System.currentTimeMillis() + 6000
        var field: AccessibilityNodeInfo? = null
        while (field == null && System.currentTimeMillis() < deadline) { field = node("Message Codex"); if (field == null) Thread.sleep(100) }
        while (field != null && field.className?.toString() != "android.widget.EditText") field = field.parent
        val editor = field ?: error("Missing composer")
        val bounds = Rect().also(editor::getBoundsInScreen)
        val downTime = SystemClock.uptimeMillis()
        for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
            val event = MotionEvent.obtain(downTime, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
            sendPointerSync(event); event.recycle()
        }
        Thread.sleep(900); heartbeat("keyboard")
        fun screenshot(name: String) {
            uiAutomation.takeScreenshot()?.let { bitmap -> File(targetContext.filesDir, name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }; bitmap.recycle() }
        }
        screenshot("composer-keyboard.png")
        check(editor.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, args.getString("token") ?: "/") }))
        Thread.sleep(600); heartbeat("suggestions with attachment/history")
        if (args.getString("token") == "@") check(node("Prompt references") != null) { "Reference suggestions were not opened" }
        else check(node("Android commands") == null) { "Slash must remain plain text" }
        if (args.getString("token") == "@") {
            val first = node("Select skill fixture0") ?: error("Reference choices have no room")
            first.performAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SHOW_ON_SCREEN.id)
            Thread.sleep(300); heartbeat("reference choice visible")
            val choice = node("Select skill fixture0") ?: error("Reference choice disappeared")
            val choiceBounds = Rect().also(choice::getBoundsInScreen)
            check(choice.isVisibleToUser && choiceBounds.height() >= (48 * activity.resources.displayMetrics.density).toInt() - 1) { "Reference choice has no usable touch area: $choiceBounds" }
        }
        screenshot("composer-suggestions.png")
        var send = node("Send message") ?: error("Send control missing behind suggestions")
        while (!send.isClickable) send = send.parent ?: error("Send button has no action owner")
        val sendBounds = Rect().also(send::getBoundsInScreen)
        check(send.isVisibleToUser && sendBounds.height() >= (48 * activity.resources.displayMetrics.density).toInt() - 1) { "Send control obscured by suggestions" }
        if (Build.VERSION.SDK_INT >= 30) {
            val keyboard = activity.window.decorView.rootWindowInsets.getInsets(WindowInsets.Type.ime()).bottom
            check(sendBounds.bottom <= activity.window.decorView.height - keyboard) { "Send control is behind the keyboard" }
        }
        check(editor.refresh())
        check(editor.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, "") }))
        Thread.sleep(600); heartbeat("suggestions closed")
        check(node("RESPONSIVENESS_END")?.isVisibleToUser == true) { "Auto-follow did not recover after closing suggestions" }
        result.putString("result", "PASS: history + image + IME + ${args.getString("token")} input remains responsive; slash is plain text and @ suggestions bounded; auto-follow recovers")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, result)
    }
}
