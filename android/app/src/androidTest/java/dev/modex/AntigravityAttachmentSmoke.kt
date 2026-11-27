package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import org.json.JSONObject

/** Render-only fixture: no host, file access, or provider request. */
internal fun Instrumentation.antigravityAttachmentSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val file = Attachment("/fixture/note.txt", "note.txt", "file", 4)
        val files = mutableStateOf(listOf(file))
        var submitted: List<Any>? = null
        val host = BackendCapabilities.fromReady(org.json.JSONObject("""{
            "backend":"codex","capabilities":["agent_discovery","antigravity_session_options","antigravity_attachments","antigravity_steer"]
        }"""))
        val connectedHost = mutableStateOf(host)
        runOnMainSync { activity.setContent {
            ModexTheme { Surface { ChatWorkspace(
                host = "fixture", connected = true, connecting = false, selected = "agy:fixture",
                threads = listOf("agy:fixture"), titles = emptyMap(), messages = emptyList(), busy = false,
                historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "",
                decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {},
                onCreate = { _, _, _ -> }, onSend = { text, model, effort, references ->
                    submitted = listOf(text, model, effort, references)
                    false
                }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
                backendCapabilities = connectedHost.value,
                sessionCapabilities = runtimeCapabilities(connectedHost.value, emptyList(), ANTIGRAVITY_AGENT),
                sessionAgent = ANTIGRAVITY_AGENT, chatAgents = listOf(ChatAgent(ANTIGRAVITY_AGENT, "Antigravity", true)),
                agyOptions = listOf(AntigravityOption("model", "fixture", linkedMapOf("fixture" to "Fixture"))),
                attachments = files.value, onRemoveAttachment = { path -> files.value = files.value.filterNot { it.path == path } },
            ) } }
        } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(current: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (current == null) return null
                if (current.text?.toString() == label || current.contentDescription?.toString() == label) return current
                for (index in 0 until current.childCount) walk(current.getChild(index))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        waitForIdleSync()
        val deadline = System.currentTimeMillis() + 5_000
        while ((node("note.txt") == null || node("Send message") == null) && System.currentTimeMillis() < deadline) Thread.sleep(100)
        check(node("note.txt") != null) { "Antigravity file chip missing" }
        var send = node("Send message")
        while (send != null && !send.isClickable) send = send.parent
        check(send?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Attachment-only send is disabled" }
        waitForIdleSync()
        check(submitted == listOf("", "", "", emptyList<PromptReference>())) { "Antigravity send leaked Codex-only fields: $submitted" }
        check(node("note.txt") != null) { "Rejected or unconfirmed send cleared the attachment" }
        runOnMainSync { connectedHost.value = host.copy(agyAttachments = false) }
        waitForIdleSync()
        val downgradeDeadline = System.currentTimeMillis() + 5_000
        while (node(ANTIGRAVITY_ATTACHMENTS_UNAVAILABLE) == null && System.currentTimeMillis() < downgradeDeadline) Thread.sleep(100)
        check(node("note.txt") != null) { "Host downgrade hid the removable attachment draft" }
        check(node("Remove note.txt") != null) { "Host downgrade made the attachment draft impossible to remove" }
        check(node(ANTIGRAVITY_ATTACHMENTS_UNAVAILABLE) != null) { "Host downgrade has no actionable attachment message" }
        val attach = node("Attach files") ?: error("Attachment button missing after host downgrade")
        val bounds = android.graphics.Rect().also(attach::getBoundsInScreen)
        val tap = uiAutomation.executeShellCommand("input tap ${bounds.centerX()} ${bounds.centerY()}")
        android.os.ParcelFileDescriptor.AutoCloseInputStream(tap).use { it.readBytes() }
        waitForIdleSync()
        check(node("From this phone") == null && node("From connected computer") == null) { "Older host still allows adding Antigravity files" }
        antigravityAttachmentAcknowledgements(host)
        output.putString("result", "PASS: Antigravity file-only send, legacy gate, active attachment block, and exact-turn draft acknowledgement")
        finish(-1, output)
    } catch (failure: Throwable) {
        output.putString("result", "FAIL: Antigravity attachment composer: ${failure.message}")
        finish(0, output)
    }
}

private fun Instrumentation.antigravityAttachmentAcknowledgements(host: BackendCapabilities) {
    runOnMainSync {
        val app = targetContext.applicationContext as ModexApplication
        val previous = app.session
        val storage = "agy-attachment-ack-fixture"
        val vm = ModexViewModel(app, storage)
        fun field(name: String) = ModexViewModel::class.java.getDeclaredField(name).apply { isAccessible = true }
        @Suppress("UNCHECKED_CAST")
        val connected = field("connected\$delegate").get(vm) as MutableState<Boolean>
        @Suppress("UNCHECKED_CAST")
        val capabilities = field("backendCapabilities\$delegate").get(vm) as MutableState<BackendCapabilities>
        @Suppress("UNCHECKED_CAST")
        val pending = field("pending").get(vm) as Map<Long, Pair<String, String>>
        val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
        val thread = "agy:attachment-ack-fixture"
        try {
            field("config").set(vm, null)
            (field("sessions").get(vm) as SessionState).open(thread)
            field("ownedThread").set(vm, thread)
            connected.value = true
            capabilities.value = host
            val note = RemoteFile("note.txt", "/fixture/note.txt", false, 4)
            val later = RemoteFile("later.txt", "/fixture/later.txt", false, 5)
            vm.attachments.addRemote(thread, note)
            val draft = PromptDraft("Explain the attachment")
            vm.promptDrafts.update(thread, draft)
            fun submit(): Long {
                check(!vm.send(draft.text, "")) { "AGY cleared the draft before acknowledgement" }
                return pending.entries.single { it.value == ("turn/start" to thread) }.key
            }
            fun reply(id: Long, key: String, value: Any) =
                apply.invoke(vm, JSONObject().put("id", id).put(key, value))
            fun checkRetained() {
                check(!vm.busy && vm.promptDrafts[thread] == draft)
                check(vm.attachments.drafts[thread]?.map { it.path } == listOf(note.path))
            }
            reply(submit(), "error", JSONObject().put("message", "attachment is not readable"))
            checkRetained()
            reply(submit(), "result", JSONObject.NULL)
            checkRetained()
            check(vm.error.contains("did not confirm"))
            val sent = submit()
            vm.attachments.addRemote(thread, later)
            reply(sent, "result", JSONObject("""{"turn":{"id":"agy-attachment-turn","status":"inProgress"}}"""))
            check(vm.promptDrafts[thread] == null)
            check(vm.attachments.drafts[thread]?.map { it.path } == listOf(later.path))

            val sessions = field("sessions").get(vm) as SessionState
            sessions.event("turn/started", JSONObject("""{"threadId":"$thread","turn":{"id":"exact-turn","status":"inProgress"}}"""))
            vm.promptDrafts.update(thread, PromptDraft("Follow up"))
            check(!vm.send("Follow up", ""))
            check(pending.values.none { it == ("turn/steer" to thread) })
            check(vm.error.contains("Remove selected files") && vm.attachments.drafts[thread]?.map { it.path } == listOf(later.path))
            vm.attachments.remove(thread, later.path)
            check(!vm.send("Follow up", ""))
            val steer = pending.entries.single { it.value == ("turn/steer" to thread) }.key
            reply(steer, "result", JSONObject().put("turnId", "wrong-turn"))
            check(vm.promptDrafts[thread]?.text == "Follow up")
            check(!vm.send("Follow up", ""))
            val retry = pending.entries.single { it.value == ("turn/steer" to thread) }.key
            vm.promptDrafts.update(thread, PromptDraft("Newer draft"))
            reply(retry, "result", JSONObject().put("turnId", "exact-turn"))
            check(vm.promptDrafts[thread]?.text == "Newer draft")
        } finally {
            connected.value = false
            vm.promptDrafts.update(thread, PromptDraft())
            vm.flushDrafts()
            app.session = previous
        }
    }
}
