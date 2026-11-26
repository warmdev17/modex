package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.runtime.mutableStateOf
import java.io.File

internal fun Instrumentation.promptDraftSmoke(args: Bundle) {
    val result = Bundle()
    try {
        val app = targetContext.applicationContext as ModexApplication
        val activity = startActivitySync(Intent(app, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val initialDisk = SavedHost(app, "draft-smoke-a-prompts").load()
        lateinit var first: ModexViewModel
        lateinit var second: ModexViewModel
        val showing = mutableStateOf<ModexViewModel?>(null)
        fun open(vm: ModexViewModel, thread: String) {
            runOnMainSync {
                val sessions = ModexViewModel::class.java.getDeclaredField("sessions").apply { isAccessible = true }.get(vm) as SessionState
                sessions.open(thread)
                showing.value = vm
            }
            waitForIdleSync()
            Thread.sleep(180)
        }
        runOnMainSync {
            fun fixture(name: String): ModexViewModel = ModexViewModel(app, name).also { vm ->
                @Suppress("UNCHECKED_CAST")
                val connected = ModexViewModel::class.java.getDeclaredField("connected\$delegate").apply { isAccessible = true }.get(vm) as androidx.compose.runtime.MutableState<Boolean>
                connected.value = true
            }
            first = fixture("draft-smoke-a")
            second = fixture("draft-smoke-b")
            activity.setContent {
                ModexTheme {
                    showing.value?.let { vm ->
                        androidx.compose.runtime.key(vm) {
                            HostWorkspace(vm, "Draft fixture", false, onHome = { showing.value = null }, onName = {})
                        }
                    }
                }
            }
        }
        fun editor(): AccessibilityNodeInfo {
            fun walk(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (node == null) return null
                if (node.isVisibleToUser && node.className?.toString() == "android.widget.EditText") return node
                for (i in 0 until node.childCount) walk(node.getChild(i))?.let { return it }
                return null
            }
            val end = System.currentTimeMillis() + 5000
            do {
                walk(uiAutomation.rootInActiveWindow)?.let { return it }
                Thread.sleep(50)
            } while (System.currentTimeMillis() < end)
            error("Composer is missing")
        }
        fun type(text: String) {
            check(editor().performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply {
                putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, text)
            })) { "Cannot type draft" }
            waitForIdleSync()
        }
        fun expect(text: String) {
            val end = System.currentTimeMillis() + 3000
            do {
                val value = editor().text?.toString().orEmpty()
                if (value == text || (text.isEmpty() && value == "Message Codex")) return
                Thread.sleep(80)
            } while (System.currentTimeMillis() < end)
            error("Draft did not survive navigation or relaunch")
        }

        if (args.getString("stage") == "steering") {
            val thread = "steer-test"
            open(first, thread)
            val vmClass = ModexViewModel::class.java
            val sessions = vmClass.getDeclaredField("sessions").apply { isAccessible = true }.get(first) as SessionState
            val apply = vmClass.getDeclaredMethod("apply", org.json.JSONObject::class.java).apply { isAccessible = true }
            fun reply(id: Long, key: String, body: org.json.JSONObject) {
                runOnMainSync { apply.invoke(first, org.json.JSONObject().put("id", id).put(key, body)) }
                waitForIdleSync()
            }
            fun click(label: String) {
                fun find(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                    if (node?.contentDescription?.toString() == label) return node
                    if (node != null) for (i in 0 until node.childCount) find(node.getChild(i))?.let { return it }
                    return null
                }
                var action = find(uiAutomation.rootInActiveWindow) ?: error("Missing $label")
                while (!action.isClickable) action = action.parent ?: error("$label is not actionable")
                check(action.isEnabled && action.performAction(AccessibilityNodeInfo.ACTION_CLICK))
                waitForIdleSync()
            }
            runOnMainSync {
                vmClass.getDeclaredField("ownedThread").apply { isAccessible = true }.set(first, thread)
                sessions.event("turn/started", org.json.JSONObject("""{"threadId":"steer-test","turn":{"id":"exact-turn","status":"inProgress"}}"""))
            }
            type("first follow-up")
            click("Send follow-up")
            expect("first follow-up")
            fun requestId(): Long {
                @Suppress("UNCHECKED_CAST")
                val pending = vmClass.getDeclaredField("pending").apply { isAccessible = true }.get(first) as Map<Long, Pair<String, String>>
                return pending.entries.single { it.value == ("turn/steer" to thread) }.key
            }
            reply(requestId(), "error", org.json.JSONObject().put("message", "fixture rejection"))
            expect("first follow-up")
            click("Send follow-up")
            type("newer draft")
            reply(requestId(), "result", org.json.JSONObject().put("turnId", "exact-turn"))
            expect("newer draft")
            click("Send follow-up")
            reply(requestId(), "result", org.json.JSONObject().put("turnId", "exact-turn"))
            expect("")
            click("Stop response")
            check(first.stopping && first.activeTurnId == "exact-turn")
            runOnMainSync { first.forget(); second.forget() }
            result.putString("result", "PASS: follow-up sent during active turn, draft held until confirmation, newer draft preserved, Stop remains available")
            finish(-1, result)
            return
        }

        if (args.getString("stage") == "safety") {
            open(first, "send-test")
            type("SEND TEST")
            fun send() {
                fun find(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                    if (node == null) return null
                    if (node.contentDescription?.toString() == "Send message") return node
                    for (i in 0 until node.childCount) find(node.getChild(i))?.let { return it }
                    return null
                }
                var target = find(uiAutomation.rootInActiveWindow) ?: error("Missing Send")
                while (!target.isClickable) target = target.parent ?: error("Send is not actionable")
                check(target.performAction(AccessibilityNodeInfo.ACTION_CLICK))
                waitForIdleSync()
            }
            send()
            expect("SEND TEST")
            runOnMainSync {
                ModexViewModel::class.java.getDeclaredField("ownedThread").apply { isAccessible = true }.set(first, "send-test")
            }
            send()
            expect("")
            runOnMainSync {
                val pending = File(app.noBackupFilesDir, "draft-smoke-a-prompts.enc.pending")
                check(pending.mkdir())
                val blocker = File(pending, "blocker").apply { writeText("fixture") }
                try {
                    first.promptDrafts.update("retry", PromptDraft("RETRY TEST"))
                    first.flushDrafts()
                    check(first.promptDrafts.error.isNotBlank() && first.promptDrafts["retry"]?.text == "RETRY TEST")
                } finally {
                    check(blocker.delete() && pending.delete())
                }
                first.flushDrafts()
                check(first.promptDrafts.error.isBlank())
                check(SavedHost(app, "draft-smoke-a-prompts").load()?.getJSONObject("retry")?.getString("text") == "RETRY TEST")
                first.promptDrafts.update("queued", PromptDraft("QUEUED BEFORE FORGET"))
                first.forget()
                second.forget()
            }
            Thread.sleep(350)
            check(SavedHost(app, "draft-smoke-a-prompts").load() == null)
            check(SavedHost(app, "draft-smoke-b-prompts").load() == null)
            result.putString("result", "PASS: rejected Send retains draft, accepted Send clears, storage failure retry and forget")
            finish(-1, result)
            return
        }

        open(first, "same-thread")
        if (args.getString("stage") == "seed") {
            runOnMainSync { first.promptDrafts.clear(); second.promptDrafts.clear() }
            type("FIRST HOST: café\nSecond line  ")
            open(first, "other-thread")
            type("SECOND CHAT")
            open(first, "same-thread")
            expect("FIRST HOST: café\nSecond line  ")
            runOnMainSync { showing.value = null }
            waitForIdleSync()
            open(second, "same-thread")
            expect("")
            type("OTHER HOST, SAME THREAD ID")
            open(first, "same-thread")
            expect("FIRST HOST: café\nSecond line  ")
            open(first, "")
            type("EMPTY CHAT DRAFT")
            expect("EMPTY CHAT DRAFT")
            runOnMainSync {
                check(first.promptDrafts[""]?.text == "EMPTY CHAT DRAFT") { "Empty chat was not captured" }
                val reference = PromptReference("skill", "fixture", "/fixture/SKILL.md", "Fixture", "", "fixture", true, "@fixture")
                first.promptDrafts.update("with-reference", PromptDraft("Use @fixture", listOf(reference)))
            }
            open(first, "same-thread")
            expect("FIRST HOST: café\nSecond line  ")
            uiAutomation.executeShellCommand("input keyevent KEYCODE_HOME").close()
            waitForIdleSync()
            val saved = SavedHost(app, "draft-smoke-a-prompts").load()
            check(saved?.optJSONObject("")?.optString("text") == "EMPTY CHAT DRAFT") { "Encrypted empty-chat draft was not committed" }
        } else {
            check(first.promptDrafts[""]?.text == "EMPTY CHAT DRAFT") {
                "Empty-chat draft was not loaded after relaunch. Disk keys: ${initialDisk?.keys()?.asSequence()?.toList()}, store error: ${first.promptDrafts.error}"
            }
            expect("FIRST HOST: café\nSecond line  ")
            open(first, "other-thread")
            if (args.getString("stage") == "cleared") expect("") else {
                expect("SECOND CHAT")
                type("")
            }
            open(first, "")
            expect("EMPTY CHAT DRAFT")
            open(second, "same-thread")
            expect("OTHER HOST, SAME THREAD ID")
            check(first.promptDrafts["with-reference"]?.references?.single()?.path == "/fixture/SKILL.md")
            val raw = File(app.noBackupFilesDir, "draft-smoke-a-prompts.enc").readBytes()
            check(!String(raw, Charsets.ISO_8859_1).contains("FIRST HOST"))
            if (args.getString("stage") == "restore") {
                uiAutomation.executeShellCommand("input keyevent KEYCODE_HOME").close()
                waitForIdleSync()
            }
        }
        result.putString("result", "PASS: prompt drafts ${args.getString("stage")}")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message} at ${failure.stackTrace.filter { it.fileName == "PromptDraftSmoke.kt" }.take(3)}")
        finish(0, result)
    }
}
