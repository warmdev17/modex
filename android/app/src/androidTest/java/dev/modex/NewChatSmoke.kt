package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.mutableStateOf
import org.json.JSONArray
import org.json.JSONObject

internal fun Instrumentation.antigravityPickerSmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var chosen = ""
        var answer = ""
        val showQuestion = mutableStateOf(false)
        val question = ChatEntry(id = "call_fixture", role = "Codex", text = "Choose a platform",
            queuedQuestions = readQueuedQuestions("call_fixture", JSONArray("""[{"title":"Platform?","options":["Android","Linux"]}]""")))
        val transcript = mutableStateOf(listOf(question))
        runOnMainSync {
            activity.setContent { ModexTheme { Surface {
                ChatWorkspace(
                    "fixture", true, false, if (showQuestion.value) "codex-fixture" else "", emptyList(), emptyMap(),
                    if (showQuestion.value) transcript.value else emptyList(),
                    false, false, false, "", "", "fixture-model", emptyList(), emptySet(),
                    {}, {}, {}, { _, _, _ -> }, { _, _, _, _ -> false },
                    { card, response -> answer = queuedQuestionReply(card, response).orEmpty() }, {}, {},
                    chatAgents = listOf(ChatAgent(CODEX_AGENT, "Codex", true), ChatAgent(ANTIGRAVITY_AGENT, "Antigravity", true)),
                    backendCapabilities = BackendCapabilities.LegacyCodex.copy(agentDiscovery = true),
                    onCreateWithAgent = { agent, _, _, _, _, _ -> chosen = agent },
                )
            } } }
        }
        fun find(node: AccessibilityNodeInfo?, title: String): AccessibilityNodeInfo? {
            if (node == null || node.text?.toString() == title) return node
            for (i in 0 until node.childCount) find(node.getChild(i), title)?.let { return it }
            return null
        }
        fun click(title: String) {
            val deadline = SystemClock.elapsedRealtime() + 5000
            var target: AccessibilityNodeInfo? = null
            while (target == null && SystemClock.elapsedRealtime() < deadline) {
                target = find(uiAutomation.rootInActiveWindow, title)
                if (target == null) SystemClock.sleep(100)
            }
            while (target != null && !target.isClickable) target = target.parent
            check(target?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $title" }
            waitForIdleSync()
        }
        click("New chat")
        click("Antigravity")
        click("Create chat")
        runOnMainSync {
            check(chosen == ANTIGRAVITY_AGENT)
            showQuestion.value = true
        }
        click("Android")
        click("Submit answer")
        runOnMainSync {
            check(readQueuedReply(answer).single().answer == "Android")
            val cliReply = """<send_user_message_question_reply>
                |{"answer":"Linux","question":"Platform?","questionItemId":"[\"request_user_input_async\",\"call_fixture\",0]"}
                |</send_user_message_question_reply>""".trimMargin()
            val item = JSONObject().put("id", "reply").put("type", "userMessage")
                .put("content", JSONArray().put(JSONObject().put("type", "text").put("text", cliReply)))
            transcript.value = updateChat(transcript.value, "item/completed", JSONObject().put("item", item))
            check(queuedQuestionCards(transcript.value).isEmpty()) { "CLI reply did not resolve the fixture in the transcript" }
        }
        val deadline = SystemClock.elapsedRealtime() + 5000
        while (find(uiAutomation.rootInActiveWindow, "Queued question") != null && SystemClock.elapsedRealtime() < deadline) {
            SystemClock.sleep(100)
        }
        check(find(uiAutomation.rootInActiveWindow, "Queued question") == null) { "CLI reply left the question card visible" }
        check(find(uiAutomation.rootInActiveWindow, "Platform?\nLinux") != null) { "CLI reply is missing from chat" }
        result.putString("result", "PASS: Antigravity picker and Codex queued question dismissal")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.message}")
        finish(0, result)
    }
}

/** Response fixture: a fresh Codex thread has no persisted history yet. */
internal fun Instrumentation.newChatSmoke() {
    val result = Bundle()
    try {
        runOnMainSync {
            val app = targetContext.applicationContext as ModexApplication
            val previous = app.session
            val vm = ModexViewModel(app)
            try {
                @Suppress("UNCHECKED_CAST")
                val pending = ModexViewModel::class.java.getDeclaredField("pending").apply { isAccessible = true }
                    .get(vm) as MutableMap<Long, Pair<String, String>>
                val apply = ModexViewModel::class.java.getDeclaredMethod("apply", JSONObject::class.java).apply { isAccessible = true }
                // Keep this reducer fixture isolated from the user's saved connection.
                ModexViewModel::class.java.getDeclaredField("config").apply { isAccessible = true }.set(vm, null)
                @Suppress("UNCHECKED_CAST")
                val connected = ModexViewModel::class.java.getDeclaredField("connected\$delegate").apply { isAccessible = true }.get(vm) as androidx.compose.runtime.MutableState<Boolean>
                connected.value = true
                fun reply(id: Long, payload: String, error: Boolean = false) {
                    apply.invoke(vm, JSONObject().put("id", id).put(if (error) "error" else "result", JSONObject(payload)))
                }
                fun requestId(method: String) = pending.entries.last { it.value.first == method }.key
                vm.select("old")
                reply(requestId("threads/resume"), """{"thread":{"id":"old"}}""")
                check(vm.historyLoading) { "Resumed session must still request history" }
                reply(requestId("threads/read"), """{"messages":[{"id":"old-message","role":"assistant","text":"Old session"}],"active_turn":true,"history_revision":"r1","next_cursor":null}""")
                check(vm.messages.isNotEmpty() && vm.busy)
                vm.createProject("Remote project", "/remote/other")
                check(vm.projectCreating)
                reply(requestId("projects/create"), """{"project":{"id":"p1","name":"Remote project","roots":[{"path":"/remote/other"}]}}""")
                check(!vm.projectCreating && vm.projects.single().id == "p1")
                vm.create("", "host-default", "", "/remote/other", "p1")
                reply(requestId("threads/start"), """{"thread":{"id":"new","cwd":"/remote/other"}}""")
                check(vm.selectedWorkspace == "/remote/other")
                check(vm.selected == "new" && !vm.historyLoading && vm.messages.isEmpty() && !vm.busy)
                @Suppress("UNCHECKED_CAST")
                val capabilities = ModexViewModel::class.java.getDeclaredField("backendCapabilities\$delegate").apply { isAccessible = true }
                    .get(vm) as androidx.compose.runtime.MutableState<BackendCapabilities>
                capabilities.value = BackendCapabilities.LegacyCodex.copy(agentDiscovery = true)
                pending[900L] = "agents/list" to ""
                reply(900L, """{"data":[{"id":"antigravity","label":"Antigravity","ready":true}]}""")
                vm.create("", "host-default", workspace = "/remote/other", projectId = "p1", agent = ANTIGRAVITY_AGENT)
                reply(requestId("threads/start"), """{"thread":{"id":"agy:fixture","cwd":"/remote/other","agent":"antigravity","projectId":"p1"}}""")
                check(vm.sessionAgent == ANTIGRAVITY_AGENT && vm.selected == "agy:fixture" && !vm.historyLoading)
                vm.select("old")
                val lateResume = requestId("threads/resume")
                vm.disconnect()
                check(vm.selected.isEmpty() && vm.messages.isEmpty())
                connected.value = true
                reply(lateResume, """{"thread":{"id":"old"}}""")
                check(vm.selected.isEmpty()) { "Late resume reopened disconnected chat" }
                vm.select("old")
                reply(requestId("threads/resume"), """{"message":"active_writer"}""", true)
                check(vm.selected.isEmpty() && vm.messages.isEmpty() && vm.takeoverThread == "old") { "CLI ownership must require explicit takeover" }
                vm.select("old")
                reply(requestId("threads/resume"), """{"thread":{"id":"old"}}""")
                check(vm.messages.isNotEmpty()) { "Successful resume should reopen cache" }
                val lateHistory = requestId("threads/read")
                vm.disconnect()
                connected.value = true
                reply(lateHistory, """{"messages":[{"role":"assistant","text":"Late"}],"history_revision":"r2","next_cursor":null}""")
                check(vm.selected.isEmpty() && vm.messages.isEmpty()) { "Late history restored disconnected screen" }
                connected.value = false
            } finally { app.session = previous }
        }
        result.putString("result", "PASS: fresh chat, cached resume, disconnect clearing, stale replies and explicit CLI takeover")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.message}")
        finish(0, result)
    }
}
