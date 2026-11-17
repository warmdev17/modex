package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.*
import androidx.compose.runtime.*
import java.io.File
import org.json.JSONArray
import org.json.JSONObject

/** Isolated state responses and real touch; no native conversation mutations. */
internal fun Instrumentation.sessionManagementSmoke() {
    val result = Bundle()
    var controller: ModexViewModel? = null
    try {
        val app = targetContext.applicationContext as ModexApplication
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        fun field(vm: ModexViewModel, name: String): Any? = ModexViewModel::class.java.getDeclaredField(name).apply { isAccessible = true }.get(vm)
        @Suppress("UNCHECKED_CAST")
        fun <T> state(vm: ModexViewModel, name: String, value: T) { (field(vm, name + "\$delegate") as MutableState<T>).value = value }
        runOnMainSync {
            val vm = ModexViewModel(app, "session-management-fixture"); controller = vm
            state(vm, "connected", true)
            val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
            @Suppress("UNCHECKED_CAST")
            val pending = field(vm, "pending") as MutableMap<Long, Pair<String, String>>
            fun latest(method: String) = pending.entries.last { it.value.first == method }.key
            fun reply(id: Long, result: JSONObject) { apply.invoke(vm, JSONObject().put("id", id).put("result", result)) }
            fun page(id: String) = JSONObject().put("data", JSONArray().put(JSONObject().put("id", id).put("cwd", "/fixture").put("name", id)))
            vm.refreshThreads(); val oldList = latest("threads/list")
            vm.showArchived(true); val archivedList = latest("threads/list")
            reply(oldList, page("wrong-active")); check(vm.threads.isEmpty())
            reply(archivedList, page("archived")); check(vm.threads == listOf("archived"))
            vm.lifecycle("delete", "archived"); check(pending.values.none { it.first == "threads/delete" })
            val cache = field(vm, "historyCache") as HistoryCache
            cache.remember("archived", CachedHistory(listOf(ChatEntry(role = "You", text = "keep history")), "", "revision"))
            vm.lifecycle("delete", "archived", true); val refused = latest("threads/delete")
            vm.lifecycle("delete", "archived", true); check(latest("threads/delete") == refused)
            apply.invoke(vm, JSONObject().put("id", refused).put("error", JSONObject().put("message", "thread_has_dependents")))
            check(vm.threads == listOf("archived") && cache.get("archived") != null && vm.error.contains("nothing was deleted"))
            vm.lifecycle("unarchive", "archived"); val stale = latest("threads/unarchive")
            vm.showArchived(false); val beforeRestore = latest("threads/list")
            reply(beforeRestore, page("active"))
            reply(stale, JSONObject()); check(vm.threads == listOf("active"))
            check(latest("threads/list") != beforeRestore) { "Restore must refresh the current list without removing unrelated rows" }
            reply(latest("threads/list"), page("active"))
            val sessions = field(vm, "sessions") as SessionState
            // Definitive mutation results survive other actions and list-mode changes.
            for (modeChange in listOf(false, true)) {
                state(vm, "threads", listOf("active", "other"))
                sessions.open("active"); ModexViewModel::class.java.getDeclaredField("ownedThread").apply { isAccessible = true }.set(vm, "active")
                sessions.event("turn/started", JSONObject("""{"threadId":"active","turn":{"id":"cleanup"}}"""))
                cache.remember("active", CachedHistory(listOf(ChatEntry(role = "You", text = "delete history")), "", "revision"))
                vm.lifecycle("archive", "other"); val archiveOther = latest("threads/archive")
                vm.lifecycle("delete", "active", true); val deleteActive = latest("threads/delete")
                if (modeChange) vm.showArchived(true) else reply(archiveOther, JSONObject())
                reply(deleteActive, JSONObject())
                check(vm.selected.isBlank() && field(vm, "ownedThread") == "") { "Confirmed delete must clear selection after list changes" }
                check(cache.get("active") == null && "active" !in sessions.runningThreads) { "Confirmed delete must clear cached history and activity" }
                if (modeChange) { reply(archiveOther, JSONObject()); vm.showArchived(false) }
                reply(latest("threads/list"), page("active"))
            }
            state(vm, "threads", listOf("active", "other"))
            cache.remember("active", CachedHistory(listOf(ChatEntry(role = "You", text = "retain history")), "", "revision"))
            vm.lifecycle("archive", "other"); val archiveOther = latest("threads/archive")
            vm.lifecycle("delete", "active", true); val refusedAfterArchive = latest("threads/delete")
            reply(archiveOther, JSONObject())
            apply.invoke(vm, JSONObject().put("id", refusedAfterArchive).put("error", JSONObject().put("message", "thread_has_dependents")))
            check("active" in vm.threads && cache.get("active") != null && vm.error.contains("nothing was deleted")) { "Concurrent archive must not hide delete refusal" }
            reply(latest("threads/list"), page("active"))
            sessions.event("turn/started", JSONObject("""{"threadId":"active","turn":{"id":"running"}}"""))
            vm.lifecycle("fork", "active"); check(pending.values.none { it.first == "threads/fork" })
            sessions.event("turn/completed", JSONObject("""{"threadId":"active","turn":{"id":"running","status":"completed"}}"""))
            state(vm, "decisions", listOf(JSONObject().put("thread_id", "active")))
            vm.lifecycle("fork", "active"); check(pending.values.none { it.first == "threads/fork" })
            state(vm, "decisions", emptyList<JSONObject>())
            vm.lifecycle("fork", "active"); check(pending.values.count { it.first == "threads/fork" } == 1)
            check(pending.values.none { it.first == "turn/start" })
            val fork = latest("threads/fork")
            vm.disconnect(); reply(fork, JSONObject().put("thread", JSONObject().put("id", "late-fork").put("cwd", "/fixture")))
            check(vm.selected.isBlank())
        }
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
        fun touch(label: String) {
            val bounds = Rect().also(await(label)::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = MotionEvent.obtain(time, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                sendPointerSync(event); event.recycle()
            }
            waitForIdleSync(); Thread.sleep(200)
        }
        fun screenshot(name: String) {
            waitForIdleSync(); Thread.sleep(600)
            val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; bmp.recycle()
        }
        var archived by mutableStateOf(false)
        var action = ""
        var error by mutableStateOf("")
        var notices by mutableStateOf(listOf(ChatEntry("prompt", "You", "Check interruption", status = "interrupted", turnId = "turn", notice = TurnNotice("stream_disconnected", "Stream disconnected before completion.", false))))
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Fixture host", connected = true, connecting = false, selected = "source", threads = listOf("source"), titles = mapOf("source" to "Fixture conversation"),
            messages = notices, busy = false, historyLoading = false, hasMore = true, error = error, sessionSettings = "", sessionModel = "fixture",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = { action = "more" }, onCreate = { _, _, _ -> },
            onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {}, archived = archived, onArchived = { archived = it },
            onSessionAction = { kind, target, confirmed -> check(target == "source"); if (kind == "delete") check(confirmed); action = kind; if (kind == "delete") error = "This conversation is needed by a fork. Nothing was deleted." }
        ) } } } }
        await("Interrupted"); await("Turn notice: Stream disconnected before completion."); screenshot("session-notice.png")
        runOnMainSync { notices = notices.map { it.copy(status = "inProgress", notice = TurnNotice("stream_disconnected", "Connection interrupted. Codex is retrying…", true)) } }
        await("Turn notice: Connection interrupted. Codex is retrying…")
        touch("Open sessions"); touch("Actions for Fixture conversation"); touch("Archive"); check(action == "archive")
        touch("Actions for Fixture conversation"); touch("Fork"); check(action == "fork")
        touch("Actions for Fixture conversation"); touch("Delete"); await("Delete conversation?"); screenshot("session-delete.png")
        action = ""; touch("Cancel"); check(action.isBlank())
        touch("Actions for Fixture conversation"); touch("Delete"); touch("Delete permanently"); check(action == "delete")
        await("This conversation is needed by a fork. Nothing was deleted."); await("Fixture conversation")
        touch("Archived conversations"); await("Back to sessions"); touch("Actions for Fixture conversation"); await("Restore"); check(node("Fork") == null); touch("Restore"); check(action == "unarchive")
        touch("Load more"); check(action == "more"); screenshot("session-archived.png")
        touch("Back to sessions"); await("Sessions")
        val vm = controller!!
        var returned by mutableStateOf(false)
        runOnMainSync {
            state(vm, "connected", true)
            activity.setContent { ModexTheme { if (returned) Text("Saved Hosts reached") else HostWorkspace(vm, "Fixture host", false, { returned = true }, {}, {}) } }
        }
        await("Open sessions")
        runOnMainSync { vm.foreground(); check(vm.selected.isBlank()) }
        touch("Connection menu"); touch("Disconnect"); await("Saved Hosts reached")
        // Loss/notification disconnect uses the same observed connection transition.
        runOnMainSync { returned = false; state(vm, "connected", true) }
        await("Open sessions")
        runOnMainSync { vm.disconnect() }
        await("Saved Hosts reached")
        runOnMainSync { vm.forget(); app.sessions.remove("session-management-fixture") }
        result.putString("result", "PASS: stale list/action/connection fencing, deletion refusal retains cache, fork idle guard, real-touch lifecycle/confirmation/archived pagination, retry/interrupted notices and disconnect route")
        finish(-1, result)
    } catch (failure: Throwable) {
        runOnMainSync { controller?.forget() }
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, result)
    }
}
