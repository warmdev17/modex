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
        fun nodes(label: String): List<AccessibilityNodeInfo> {
            val matches = mutableListOf<AccessibilityNodeInfo>()
            fun walk(n: AccessibilityNodeInfo?) {
                if (n == null) return
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) matches += n
                for (i in 0 until n.childCount) walk(n.getChild(i))
            }
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            walk(uiAutomation.rootInActiveWindow)
            return matches
        }
        fun node(label: String): AccessibilityNodeInfo? = nodes(label).firstOrNull()
        fun await(label: String): AccessibilityNodeInfo {
            val end = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < end) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun awaitAction(label: String, action: Int): AccessibilityNodeInfo {
            val end = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < end) {
                for (match in nodes(label)) {
                    var current: AccessibilityNodeInfo? = match
                    while (current != null) {
                        if (current.actionList.any { it.id == action }) return current
                        current = current.parent
                    }
                }
                Thread.sleep(100)
            }
            error("Missing action $action for $label")
        }
        fun disabled(label: String): Boolean {
            var current: AccessibilityNodeInfo? = await(label)
            while (current != null) {
                if (!current.isEnabled) return true
                current = current.parent
            }
            return false
        }
        fun settle() {
            // A running session spinner can keep the main looper busy during a menu action.
            try { uiAutomation.waitForIdle(100, 1500) } catch (_: java.util.concurrent.TimeoutException) { }
        }
        fun touchNode(target: AccessibilityNodeInfo, holdMs: Long = 0) {
            val bounds = Rect().also(target::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            try {
                val down = MotionEvent.obtain(time, time, MotionEvent.ACTION_DOWN, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                sendPointerSync(down); down.recycle()
                if (holdMs > 0) Thread.sleep(holdMs)
                val up = MotionEvent.obtain(time, SystemClock.uptimeMillis(), MotionEvent.ACTION_UP, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                sendPointerSync(up); up.recycle()
            } catch (failure: IllegalArgumentException) { error("touchNode ${target.text}/${target.contentDescription} $bounds: ${failure.message}") }
            settle()
        }
        fun longTouchAction(label: String) = touchNode(awaitAction(label, AccessibilityNodeInfo.ACTION_LONG_CLICK), 650)
        fun touch(label: String) {
            val bounds = Rect().also(await(label)::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = MotionEvent.obtain(time, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                try { sendPointerSync(event) } catch (failure: IllegalArgumentException) { error("touch $label $bounds: ${failure.message}") } finally { event.recycle() }
            }
            settle()
        }
        fun longTouch(label: String) {
            val bounds = Rect().also(await(label)::getBoundsInScreen)
            val time = SystemClock.uptimeMillis()
            val down = MotionEvent.obtain(time, time, MotionEvent.ACTION_DOWN, bounds.exactCenterX(), bounds.exactCenterY(), 0)
            try { sendPointerSync(down) } catch (failure: IllegalArgumentException) { error("longTouch down $label $bounds: ${failure.message}") } finally { down.recycle() }; Thread.sleep(650)
            val up = MotionEvent.obtain(time, SystemClock.uptimeMillis(), MotionEvent.ACTION_UP, bounds.exactCenterX(), bounds.exactCenterY(), 0)
            try { sendPointerSync(up) } catch (failure: IllegalArgumentException) { error("longTouch up $label $bounds: ${failure.message}") } finally { up.recycle() }
            settle()
        }
        fun screenshot(name: String) {
            Thread.sleep(600)
            val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; bmp.recycle()
        }
        fun back() { check(uiAutomation.performGlobalAction(android.accessibilityservice.AccessibilityService.GLOBAL_ACTION_BACK)); settle() }
        var archived by mutableStateOf(false)
        var running by mutableStateOf(false)
        var action = ""
        var error by mutableStateOf("")
        val sessionTitle = "Fixture conversation with a deliberately long title for a narrow sidebar"
        var notices by mutableStateOf(listOf(ChatEntry("prompt", "You", "Check interruption", status = "interrupted", turnId = "turn", notice = TurnNotice("stream_disconnected", "Stream disconnected before completion.", false))))
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Fixture host", connected = true, connecting = false, selected = "source", threads = listOf("source"), titles = mapOf("source" to sessionTitle),
            messages = notices, busy = false, historyLoading = false, hasMore = true, error = error, sessionSettings = "", sessionModel = "fixture",
            decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = { action = "select" }, onRefresh = {}, onMore = { action = "more" }, onCreate = { _, _, _ -> },
            onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {}, archived = archived, onArchived = { archived = it },
            runningThreads = if (running) setOf("source") else emptySet(),
            models = listOf(ModelOption("fixture-model", "Fixture model with a deliberately long provider label", "medium", listOf("medium" to "Balanced"))), hostModel = "fixture-model",
            registeredProjects = listOf(RemoteProject("fixture-project", "Fixture project with a deliberately long name", listOf("/fixture/a/deliberately/long/project/root"))),
            projectMembership = mapOf("source" to "fixture-project"),
            onSessionAction = { kind, target, confirmed -> check(target == "source"); if (kind == "delete") check(confirmed); action = kind; if (kind == "delete") error = "This conversation is needed by a fork. Nothing was deleted." }
        ) } } } }
        await("Interrupted"); await("Turn notice: Stream disconnected before completion."); screenshot("session-notice.png")
        runOnMainSync { notices = notices.map { it.copy(status = "inProgress", notice = TurnNotice("stream_disconnected", "Connection interrupted. Codex is retrying…", true)) } }
        await("Turn notice: Connection interrupted. Codex is retrying…")
        touch("Open sessions")
        val activeRow = awaitAction(sessionTitle, AccessibilityNodeInfo.ACTION_LONG_CLICK)
        check(activeRow.actionList.any { it.id == AccessibilityNodeInfo.ACTION_CLICK }) { "Active session row must keep ordinary tap" }
        action = ""; touchNode(activeRow); check(action == "select")
        touch("Open sessions")
        val actionLabel = "Actions for $sessionTitle"
        val more = await(actionLabel)
        action = ""; touchNode(more); await("Archive")
        check(action.isBlank()) { "Opening session actions must not select the conversation" }
        back()
        touch("Collapse project Fixture project with a deliberately long name")
        val recentMore = await(actionLabel)
        action = ""; touchNode(recentMore); await("Archive")
        check(action.isBlank()) { "Recent session actions must not select the conversation" }
        back()
        runOnMainSync { running = true }; action = ""; touchNode(awaitAction(sessionTitle, AccessibilityNodeInfo.ACTION_LONG_CLICK), 650); await("Archive")
        check(action.isBlank()) { "Long press must not select the conversation" }
        check(disabled("Fork")) { "Running session must keep Fork disabled" }
        screenshot("menu-session-actions.png"); back()
        runOnMainSync { running = false }
        longTouchAction(sessionTitle); touch("Archive"); check(action == "archive")
        longTouchAction(sessionTitle); touch("Fork"); check(action == "fork")
        longTouchAction(sessionTitle); touch("Delete"); await("Delete conversation?"); screenshot("session-delete.png")
        action = ""; touch("Cancel"); check(action.isBlank())
        longTouchAction(sessionTitle); touch("Delete"); touch("Delete permanently"); check(action == "delete")
        await("This conversation is needed by a fork. Nothing was deleted."); await(sessionTitle)
        touch("Archived conversations"); await("Back to sessions")
        val archivedRow = awaitAction(sessionTitle, AccessibilityNodeInfo.ACTION_LONG_CLICK)
        check(archivedRow.actionList.any { it.id == AccessibilityNodeInfo.ACTION_LONG_CLICK } && archivedRow.actionList.none { it.id == AccessibilityNodeInfo.ACTION_CLICK }) { "Archived rows expose long-click actions without a no-op click" }
        touchNode(await(actionLabel)); await("Restore"); check(node("Fork") == null); touch("Restore"); check(action == "unarchive")
        touch("Load more"); check(action == "more"); screenshot("session-archived.png")
        touch("Back to sessions"); await("Sessions"); touchNode(awaitAction(sessionTitle, AccessibilityNodeInfo.ACTION_CLICK))
        longTouch("Check interruption"); await("Copy"); screenshot("menu-message-actions.png"); sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
        touch("Attach files"); await("From this phone"); screenshot("menu-upload.png"); back()
        touch("Connection menu"); await("Connection settings"); screenshot("menu-connection.png"); touch("Connection settings")
        touch("Open sessions"); touch("New chat"); await("New conversation")
        touch("Outside projects"); await("Fixture project with a deliberately long name"); screenshot("menu-project.png"); back()
        touch("Fixture model with a deliberately long provider label"); await("Custom model ID…"); screenshot("menu-model.png"); back()
        back()
        val vm = controller!!
        var returned by mutableStateOf(false)
        runOnMainSync {
            state(vm, "connected", true)
            activity.setContent { ModexTheme { if (returned) Text("Saved Hosts reached") else HostWorkspace(vm, "Fixture host", false, { returned = true }, {}) } }
        }
        await("Open sessions")
        runOnMainSync { vm.foreground(); check(vm.selected.isBlank()) }
        touch("Connection menu"); touch("Disconnect"); await("Saved Hosts reached")
        // Loss/notification disconnect uses the same observed connection transition.
        runOnMainSync { returned = false; state(vm, "connected", true) }
        await("Open sessions")
        runOnMainSync { vm.disconnect() }
        await("Saved Hosts reached")
        lateinit var menuHostId: String
        val keyPair = com.jcraft.jsch.KeyPair.genKeyPair(com.jcraft.jsch.JSch(), com.jcraft.jsch.KeyPair.RSA, 1024)
        val key = try {
            java.io.ByteArrayOutputStream().also { keyPair.writePrivateKey(it) }.toByteArray()
        } finally {
            keyPair.dispose()
        }
        runOnMainSync {
            val connection = Connection("fixture.invalid", 22, "fixture", "SHA256:" + "A".repeat(43), "", "")
            menuHostId = app.connections.add("Menu preview fixture host", connection, key).getOrThrow()
            key.fill(0)
            activity.setContent { ModexTheme { ConnectionHome() } }
        }
        touch("Manage Menu preview fixture host"); await("Edit host"); screenshot("menu-host.png")
        runOnMainSync { app.connections.remove(menuHostId) }
        runOnMainSync { vm.forget(); app.sessions.remove("session-management-fixture") }
        result.putString("result", "PASS: stale list/action/connection fencing, deletion refusal retains cache, fork idle guard, real-touch lifecycle/confirmation/archived pagination, retry/interrupted notices and disconnect route")
        finish(-1, result)
    } catch (failure: Throwable) {
        runOnMainSync { controller?.forget() }
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}"); finish(0, result)
    }
}
