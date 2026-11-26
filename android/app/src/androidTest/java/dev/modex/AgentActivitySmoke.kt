package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import android.view.MotionEvent
import android.view.WindowInsets
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableStateOf
import org.json.JSONArray
import org.json.JSONObject

internal fun Instrumentation.agentActivitySmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val selected = mutableStateOf("agents")
        val conversation = AgentConversation()
        val state = SessionState().apply { open("agents") }
        var olderLoads = 0
        fun childMessages(prefix: String, range: IntRange) = JSONArray().apply {
            range.forEach { index -> put(JSONObject().put("id", "$prefix-$index").put("role", "assistant")
                .put("turn_id", "$prefix-turn-$index").put("text", "${if (prefix == "older") "Earlier agent message" else "Read-only agent response"} $index")) }
        }
        fun event(id: String, action: String, targets: String) = state.event("item/completed", JSONObject(
            """{"threadId":"agents","turnId":"turn","item":{"id":"$id","type":"agentActivity","action":"$action","targets":$targets}}"""
        ))
        runOnMainSync {
            state.sent(ChatEntry(id = "fixture-prompt", role = "You", text = "Check reviewer and tester progress."))
            state.event("turn/started", JSONObject("""{"threadId":"agents","turn":{"id":"turn"}}"""))
            event("spawn", "started", """[{"id":"one","label":"reviewer","status":"running"},{"id":"two","label":"tester","status":"running"}]""")
            activity.setContent { ModexTheme { Surface { ChatWorkspace(
                host = "Agents fixture", connected = true, connecting = false, selected = selected.value,
                threads = listOf("agents", "other"), titles = emptyMap(), messages = state.messages,
                busy = state.busy, activeTurnId = state.activeTurnId,
                agentConversation = conversation,
                onOpenAgent = { agent ->
                    conversation.open(agent)
                    conversation.begin(1, older = false)
                    conversation.result(1, JSONObject().put("thread", JSONObject().put("id", agent.id))
                        .put("messages", childMessages("current", 0..29)).put("next_cursor", "older-page"))
                },
                onAgentHistory = { older ->
                    check(older)
                    olderLoads++
                    conversation.begin(2, older = true)
                    conversation.event("item/agentMessage/delta", JSONObject("""{"threadId":"one","turnId":"live-older","itemId":"live-older-answer","delta":"Live during older page"}"""))
                    conversation.result(2, JSONObject().put("thread", JSONObject().put("id", "one"))
                        .put("messages", childMessages("older", 0..9)))
                },
                historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture",
                decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {},
                onCreate = { _, _, _ -> }, onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
            ) } } }
        }
        fun node(label: String): AccessibilityNodeInfo? {
            fun find(info: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (info == null) return null
                if (info.text?.toString() == label || info.contentDescription?.toString() == label) return info
                for (index in 0 until info.childCount) find(info.getChild(index))?.let { return it }
                return null
            }
            return find(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = SystemClock.uptimeMillis() + 6500
            while (SystemClock.uptimeMillis() < deadline) {
                node(label)?.takeIf { it.isVisibleToUser }?.let { return it }
                Thread.sleep(100)
            }
            error("Missing $label")
        }
        fun list() = await("Agent conversation history")
        fun scrollOlder() {
            val bounds = Rect().also(list()::getBoundsInScreen)
            val x = bounds.centerX()
            val start = bounds.top + bounds.height() / 4
            val end = bounds.bottom - bounds.height() / 4
            android.os.ParcelFileDescriptor.AutoCloseInputStream(
                uiAutomation.executeShellCommand("input swipe $x $start $x $end 380"),
            ).use { it.readBytes() }
            waitForIdleSync(); Thread.sleep(500)
        }
        fun viewport(): android.graphics.Bitmap {
            val bounds = Rect().also(list()::getBoundsInScreen)
            val screen = checkNotNull(uiAutomation.takeScreenshot())
            bounds.intersect(0, 0, screen.width, screen.height)
            return android.graphics.Bitmap.createBitmap(screen, bounds.left, bounds.top, bounds.width(), bounds.height()).also { screen.recycle() }
        }
        fun tap(target: AccessibilityNodeInfo): Rect {
            val bounds = Rect().also(target::getBoundsInScreen)
            val now = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                MotionEvent.obtain(now, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0).also {
                    sendPointerSync(it)
                    it.recycle()
                }
            }
            return bounds
        }
        fun screenshot(name: String) {
            ParcelFileDescriptor.AutoCloseInputStream(uiAutomation.executeShellCommand("screencap -p /data/local/tmp/$name"))
                .use { it.readBytes() }
        }
        tap(await("Message Codex"))
        val imeDeadline = SystemClock.uptimeMillis() + 6000
        while (!activity.window.decorView.rootWindowInsets.isVisible(WindowInsets.Type.ime()) && SystemClock.uptimeMillis() < imeDeadline) Thread.sleep(100)
        check(activity.window.decorView.rootWindowInsets.isVisible(WindowInsets.Type.ime())) { "Real keyboard did not open" }
        val stop = Rect().also(await("Stop response")::getBoundsInScreen)
        val keyboard = activity.window.decorView.rootWindowInsets.getInsets(WindowInsets.Type.ime()).bottom
        check(stop.bottom <= activity.window.decorView.height - keyboard) { "Stop overlaps keyboard" }
        screenshot("agents-keyboard.png")
        var button = await("Agents 2")
        while (!button.isClickable) button = button.parent ?: error("Agents has no click action")
        check(button.stateDescription?.toString() == "2 running")
        val bounds = Rect().also(button::getBoundsInScreen)
        val x = bounds.left + (8 * activity.resources.displayMetrics.density).toInt()
        val inside = bounds.centerY()
        val outside = bounds.top + 2
        fun colors(): Pair<Int, Int> {
            val bitmap = checkNotNull(uiAutomation.takeScreenshot())
            return (bitmap.getPixel(x, inside) to bitmap.getPixel(x, outside)).also { bitmap.recycle() }
        }
        val resting = colors()
        val pressTime = SystemClock.uptimeMillis()
        fun press(action: Int) {
            MotionEvent.obtain(pressTime, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0).also {
                sendPointerSync(it)
                it.recycle()
            }
        }
        press(MotionEvent.ACTION_DOWN)
        Thread.sleep(230)
        val pressed = colors()
        press(MotionEvent.ACTION_UP)
        check(pressed.first != resting.first && pressed.second == resting.second) { "Pressed feedback must stay inside the chip" }
        val minimum = (48 * activity.resources.displayMetrics.density).toInt() - 1
        check(bounds.height() >= minimum && bounds.width() >= minimum && bounds.bottom <= stop.top)
        await("Agent activity")
        await("reviewer")
        await("tester")
        runOnMainSync {
            event("interact", "assigned", """[{"id":"one","status":"running"}]""")
            event("complete", "completed", """[{"id":"one","status":"completed"}]""")
            event("wait", "waited", """[{"id":"two","status":"errored"}]""")
            state.event("turn/completed", JSONObject("""{"threadId":"agents","turn":{"id":"turn","status":"completed"}}"""))
        }
        await("Received a task")
        await("Error")
        screenshot("agents-sheet.png")
        tap(await("reviewer"))
        await("Read-only agent response 29")
        scrollOlder()
        await("Scroll to latest")
        val before = viewport()
        runOnMainSync {
            conversation.event("item/agentMessage/delta", JSONObject("""{"threadId":"one","turnId":"child-turn","itemId":"live-answer","delta":"Live agent update"}"""))
        }
        Thread.sleep(1000)
        val after = viewport()
        check(before.sameAs(after)) { "Live child updates moved the reading viewport" }
        before.recycle(); after.recycle()
        check(node("Live agent update") == null) { "Live update pulled the child transcript to the bottom" }
        tap(await("Scroll to latest"))
        await("Live agent update")
        runOnMainSync {
            conversation.event("item/agentMessage/delta", JSONObject().put("threadId", "one").put("turnId", "child-turn")
                .put("itemId", "live-answer").put("delta", "\n\n" + (0..39).joinToString("\n\n") { "Child stream paragraph $it" } + "\n\nCHILD_STREAM_END"))
        }
        await("CHILD_STREAM_END")
        for (page in 1..40) {
            if (node("Load earlier activity")?.isVisibleToUser == true) break
            scrollOlder()
        }
        tap(await("Load earlier activity"))
        check(olderLoads == 1)
        await("Read-only agent response 0")
        check(node("Live during older page") == null) { "Older pagination pulled the viewport to a live update" }
        tap(await("Scroll to latest"))
        await("Live during older page")
        check(node("Message Codex") == null && node("Send message") == null && node("Stop response") == null)
        screenshot("agent-conversation.png")
        tap(await("Back to agents"))
        await("Agent activity")
        tap(await("Close agent activity"))
        await("Send message")
        tap(await("Agents"))
        await("Agent activity")
        runOnMainSync { state.open("other"); selected.value = "other" }
        await("Message Codex")
        check(node("Agent activity") == null && node("Agents") == null)
        result.putString("result", "PASS: compact Agents button with IME, accessible counts, live timeline, read-only transcript scroll/pagination, Back and isolated chat")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}")
        finish(0, result)
    }
}
