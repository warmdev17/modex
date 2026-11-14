package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

/** Render-only fixture. No host connection, credentials, or prompt is submitted. */
fun Instrumentation.chatUiSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var markdown by mutableStateOf("## Ready to build\n\nUse **native Android** with `Jetpack Compose`.\n\n- Keep the session ID\n- Render Markdown cleanly\n\n```kotlin\nval message = \"Hello, Modex\"\nprintln(message)\n```")
        var selected = "fixture-one"
        var attachmentFiles by mutableStateOf(listOf(Attachment("/fixture/reference.png", "reference.png", "image")))
        var phoneChosen = false
        var remoteChosen = false
        var chosenModel by mutableStateOf("gpt-5.6-sol")
        var chosenEffort by mutableStateOf("medium")
        var attempted = ""
        var attemptedModel = ""
        var attemptedEffort = ""
        runOnMainSync { activity.setContent {
            var current by remember { mutableStateOf(selected) }
            ModexTheme { Surface { ChatWorkspace(
                "laptop.local", true, false, current,
                listOf("fixture-one","fixture-two","standalone"), mapOf("standalone" to "Standalone chat", "fixture-one" to "Modex mobile redesign", "fixture-two" to "SSH approval flow"),
                listOf(ChatEntry(role = "You", text = "Make this easier to use on my phone.", status = "interrupted", durationMs = 1200), ChatEntry(role = "Codex", text = markdown)), false, false, false, "", "",
                chosenModel, emptyList(), emptySet(),
                { current = it; selected = it }, {}, {}, { _, _, _ -> }, { text, model, effort -> attempted = text; attemptedModel = model; attemptedEffort = effort; false }, { _, _ -> }, {}, {}, registeredProjects = listOf(RemoteProject("p1", "modex", listOf("/work/modex")), RemoteProject("p2", "other", listOf("/work/other"))), projectMembership = mapOf("fixture-one" to "p1", "fixture-two" to "p2"), threadWorkspaces = mapOf("fixture-one" to "/work/modex", "fixture-two" to "/work/other", "standalone" to "/generated/ssh-x20"), attachments = attachmentFiles, onPhoneFiles = { phoneChosen = true }, onRemoteFiles = { remoteChosen = true }, onRemoveAttachment = { path -> attachmentFiles = attachmentFiles.filter { it.path != path } }, sessionEffort = chosenEffort, onModelChoice = { m, e -> chosenModel = m; chosenEffort = e }, models = listOf(ModelOption("fixture-custom", "Fixture custom", "medium", listOf("low" to "Fast", "medium" to "Balanced", "high" to "Deep")))
            ) } }
        } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        waitForIdleSync()
        val modelDeadline = System.currentTimeMillis() + 5000
        while (node("Choose model") == null && System.currentTimeMillis() < modelDeadline) Thread.sleep(100)
        check(node("Choose model") != null) { "Composer has no model control" }
        fun click(label: String) {
            val deadline = System.currentTimeMillis()+5000
            var n: AccessibilityNodeInfo? = null
            while (n == null && System.currentTimeMillis()<deadline) { n = node(label); if(n == null) Thread.sleep(100) }
            check(n != null) { "Missing $label" }
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            Thread.sleep(500)
        }
        fun shot(name: String) {
            waitForIdleSync(); Thread.sleep(500)
            val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir,name).outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG,100,it) }; bmp.recycle()
        }
        check(node("Interrupted · 1s") != null) { "Interrupted turn displayed as completed" }
        var userMessage = node("Make this easier to use on my phone.")
        while (userMessage != null && !userMessage.isLongClickable) userMessage = userMessage.parent
        check(userMessage?.performAction(AccessibilityNodeInfo.ACTION_LONG_CLICK) == true)
        Thread.sleep(500)
        check(node("Edit") != null && node("Copy") != null && node("Select text") != null)
        android.os.ParcelFileDescriptor.AutoCloseInputStream(uiAutomation.executeShellCommand("input keyevent 4")).use { it.readBytes() }
        Thread.sleep(500)
        check(node("reference.png") != null) { "Attachment chip missing" }
        click("Attach files")
        check(node("From this phone") != null && node("From connected computer") != null) { "Attachment sources missing" }
        shot("attachment-sources.png")
        click("From this phone")
        check(phoneChosen)
        click("Attach files"); click("From connected computer"); check(remoteChosen)
        click("Remove reference.png")
        check(attachmentFiles.isEmpty()) { "Attachment removal failed" }
        shot("chat-mocha.png")
        click("Copy code")
        check(node("Copied") != null)
        Thread.sleep(3500)
        click("Open sessions")
        shot("drawer-mocha.png")
        val projectsDeadline = System.currentTimeMillis() + 5000
        while (node("Projects") == null && System.currentTimeMillis() < projectsDeadline) Thread.sleep(100)
        check(node("Projects") != null) { "Project section missing" }
        check(node("/work/modex") != null) { "Project path missing" }
        check(node("/work/other") != null) { "Second project missing" }
        check(node("ssh-x20") == null && node("/generated/ssh-x20") == null) { "Standalone chat generated a project" }
        shot("drawer-mocha.png")
        click("SSH approval flow")
        runOnMainSync { check(selected == "fixture-two") }
        click("Choose model")
        click("gpt-5.6-sol")
        click("Fixture custom")
        shot("model-reasoning.png")
        val sliderDeadline = System.currentTimeMillis() + 5000
        while (node("Reasoning effort") == null && System.currentTimeMillis() < sliderDeadline) Thread.sleep(100)
        val slider = node("Reasoning effort") ?: error("Reasoning slider missing")
        val initialConfirmBounds = android.graphics.Rect().also { node("Use model")!!.getBoundsInScreen(it) }
        val sliderArgs = Bundle().apply { putFloat(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE, 2f) }
        check(slider.performAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SET_PROGRESS.id, sliderArgs))
        waitForIdleSync(); Thread.sleep(500)
        val changedConfirmBounds = android.graphics.Rect().also { node("Use model")!!.getBoundsInScreen(it) }
        check(initialConfirmBounds == changedConfirmBounds) { "Reasoning description resized modal: $initialConfirmBounds -> $changedConfirmBounds" }
        click("Use model")
        check(node("fixture-custom · high") != null) { "Queued model is not shown in composer" }
        fun shell(command: String) {
            android.os.ParcelFileDescriptor.AutoCloseInputStream(uiAutomation.executeShellCommand(command)).use { it.readBytes() }
        }
        runOnMainSync { chosenModel = "m"; chosenEffort = "low" }
        waitForIdleSync(); Thread.sleep(500)
        val modelBounds = android.graphics.Rect().also { node("Choose model")!!.getBoundsInScreen(it) }
        val sendBounds = android.graphics.Rect().also { node("Send message")!!.getBoundsInScreen(it) }
        check(modelBounds.right < sendBounds.left - 20) { "Model button fills unused composer space" }
        click("Choose model")
        shell("input keyevent 4")
        Thread.sleep(500)
        check(node("Use model") == null && node("Choose model") != null) { "Back did not dismiss model dialog" }
        runOnMainSync { chosenModel = "fixture-custom"; chosenEffort = "high" }
        runOnMainSync { attachmentFiles = listOf(Attachment("/fixture/retry.txt", "retry.txt", "file")) }
        click("Message Codex")
        val field = node("Message Codex") ?: error("Composer inaccessible after focus")
        val bounds = android.graphics.Rect(); field.getBoundsInScreen(bounds)
        shell("input tap ${bounds.centerX()} ${bounds.centerY()}")
        Thread.sleep(500)
        shell("input text Keep%sthis%sdraft")
        Thread.sleep(500)
        click("Send message")
        runOnMainSync {
            check(attempted == "Keep this draft") { "Fixture did not type draft" }
            check(attemptedModel == "fixture-custom")
            check(attemptedEffort == "high") { "Queued reasoning did not reach send" }
            attempted = ""
        }
        click("Send message")
        runOnMainSync { check(attempted == "Keep this draft") { "Rejected send lost draft" }; check(attachmentFiles.single().name == "retry.txt") { "Rejected send lost attachment" } }
        Thread.sleep(700)
        shot("composer-keyboard.png")
        shell("input keyevent 4")
        runOnMainSync { markdown = "## Structured output\n\n| File | Status |\n| --- | --- |\n| MainActivity.kt | Updated |\n\n> Keep the existing session ID.\n\n1. Read **history**\n2. Resume safely\n\n[Documentation](https://example.com)" }
        shot("markdown-table.png")
        check(File(targetContext.filesDir, "markdown-table.png").length() > 0) { "Table screenshot missing" }
        output.putString("result", "PASS: Mocha chat, model control, code copy, drawer selection, rejected-send draft retained, composer focus")
        finish(-1,output)
    } catch (e: Throwable) { output.putString("result","FAIL: ${e.javaClass.simpleName}: ${e.message} at ${e.stackTrace.firstOrNull { it.fileName == "ChatUiSmoke.kt" }}"); finish(0,output) }
}

/** Regression: history arrives after the list has already laid out at its beginning. */
fun Instrumentation.historyScrollSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var loading by mutableStateOf(false)
        var messages by mutableStateOf(listOf(ChatEntry(role = "You", text = "Old message")))
        var paged by mutableStateOf(false)
        var olderLoading by mutableStateOf(false)
        var olderRequests = 0
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            "fixture", true, false, "history", listOf("history"), emptyMap(),
            messages, false, loading, false, "", "", "fixture-model", emptyList(), emptySet(), {}, {}, {}, { _, _, _ -> }, { _, _, _ -> false }, { _, _ -> }, {}, {},
            hasOlderHistory = paged, olderHistoryLoading = olderLoading, onOlderHistory = { olderRequests++; olderLoading = true }
        ) } } } }
        waitForIdleSync(); Thread.sleep(700)
        runOnMainSync { loading = true }
        waitForIdleSync(); Thread.sleep(500)
        runOnMainSync {
            messages = (0..199).map { ChatEntry(id = "history-$it", role = "You", text = if (it == 199) "LATEST_HISTORY_MARKER" else "Earlier message $it") }
            loading = false
        }
        fun visibleMarker(n: AccessibilityNodeInfo?): Boolean {
            if (n == null) return false
            if (n.isVisibleToUser && n.text?.toString()?.endsWith("LATEST_HISTORY_MARKER") == true) return true
            return (0 until n.childCount).any { visibleMarker(n.getChild(it)) }
        }
        val deadline = System.currentTimeMillis() + 5000
        while (!visibleMarker(uiAutomation.rootInActiveWindow) && System.currentTimeMillis() < deadline) Thread.sleep(100)
        check(visibleMarker(uiAutomation.rootInActiveWindow)) {
            fun labels(n: AccessibilityNodeInfo?): String = if (n == null) "null" else "[${n.text}|${n.contentDescription}]" + (0 until n.childCount).joinToString { labels(n.getChild(it)) }
            "History opened at the beginning: ${labels(uiAutomation.rootInActiveWindow)}"
        }
        fun scrollable(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
            if (n == null) return null
            if (n.isScrollable) return n
            for (i in 0 until n.childCount) scrollable(n.getChild(i))?.let { return it }
            return null
        }
        repeat(1) {
            check(scrollable(uiAutomation.rootInActiveWindow)?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD) == true) { "Cannot scroll history" }
            Thread.sleep(700)
        }
        check(!visibleMarker(uiAutomation.rootInActiveWindow)) { "Fixture did not scroll to older messages" }
        fun earlierMessage(n: AccessibilityNodeInfo?): String? {
            if (n == null) return null
            if (n.isVisibleToUser && n.text?.toString()?.contains("Earlier message") == true) return n.text.toString()
            for (i in 0 until n.childCount) earlierMessage(n.getChild(i))?.let { return it }
            return null
        }
        val reading = earlierMessage(uiAutomation.rootInActiveWindow) ?: error("No older message visible")
        runOnMainSync { messages = messages + ChatEntry(role = "You", text = "New streaming message") }
        waitForIdleSync(); Thread.sleep(700)
        check(earlierMessage(uiAutomation.rootInActiveWindow) == reading) { "New message stole the older-history position" }
        runOnMainSync { loading = true }
        waitForIdleSync(); Thread.sleep(500)
        runOnMainSync {
            messages = messages.dropLast(1) + ChatEntry(role = "You", text = ("Tall final message\n".repeat(100) + "LATEST_HISTORY_MARKER"))
            loading = false
        }
        val tallDeadline = System.currentTimeMillis() + 5000
        while (!visibleMarker(uiAutomation.rootInActiveWindow) && System.currentTimeMillis() < tallDeadline) Thread.sleep(100)
        check(visibleMarker(uiAutomation.rootInActiveWindow)) { "Reload did not reach tall final message" }
        runOnMainSync { loading = true }
        waitForIdleSync(); Thread.sleep(500)
        runOnMainSync { messages = (0..29).map { ChatEntry(id = "history-$it", role = "You", text = "Earlier message $it") }; paged = true; loading = false }
        waitForIdleSync(); Thread.sleep(700)
        fun freshRoot(): AccessibilityNodeInfo? {
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return uiAutomation.rootInActiveWindow
        }
        runOnMainSync { check(olderRequests == 0) { "History loaded before scrolling near the beginning" } }
        repeat(14) {
            var requested = false
            runOnMainSync { requested = olderRequests > 0 }
            if (!requested) {
                scrollable(freshRoot())?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD)
                Thread.sleep(700)
            }
        }
        runOnMainSync { check(olderRequests == 1 && olderLoading) { "Expected one automatic request near the top, got $olderRequests" } }
        Thread.sleep(700)
        runOnMainSync { check(olderRequests == 1) { "Duplicate automatic history request" } }
        val anchor = earlierMessage(freshRoot()) ?: error("No reading anchor before prepend")
        runOnMainSync {
            messages = (-10..-1).map { ChatEntry(id = "older-$it", role = "You", text = "Older page $it") } + messages
            olderLoading = false; paged = false
        }
        waitForIdleSync(); Thread.sleep(700)
        check(earlierMessage(freshRoot()) == anchor) { "Automatic prepend lost the reading anchor" }
        output.putString("result", "PASS: async history, append, tall final message, automatic older loading, duplicate guard and prepend anchor")
        finish(-1, output)
    } catch (e: Throwable) { output.putString("result", "FAIL: ${e.message} at ${e.stackTrace.firstOrNull { it.fileName == "ChatUiSmoke.kt" }}"); finish(0, output) }
}

/** Native command disclosure, driven by the same reducer as live Codex events. */
fun Instrumentation.commandTimelineSmoke() {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var messages by mutableStateOf(listOf(ChatEntry("a", "Codex", "Checking the workspace.")))
        runOnMainSync {
            messages = updateChat(messages, "item/started", org.json.JSONObject("""{"item":{"id":"c","type":"commandExecution","command":"go test ./...","cwd":"/work","status":"inProgress"}}"""))
            messages = updateChat(messages, "item/commandExecution/outputDelta", org.json.JSONObject("""{"itemId":"c","delta":"TOOL_OUTPUT_MARKER"}"""))
            activity.setContent { ModexTheme { Surface { ChatWorkspace(
                "fixture", true, false, "tools", listOf("tools"), emptyMap(), messages,
                false, false, false, "", "", "fixture-model", emptyList(), emptySet(), {}, {}, {}, { _, _, _ -> }, { _, _, _ -> false }, { _, _ -> }, {}, {}
            ) } } }
        }
        fun node(label: String, n: AccessibilityNodeInfo? = uiAutomation.rootInActiveWindow): AccessibilityNodeInfo? {
            if (n == null) return null
            if (n.isVisibleToUser && (n.text?.toString() == label || n.contentDescription?.toString() == label)) return n
            for (i in 0 until n.childCount) node(label, n.getChild(i))?.let { return it }
            return null
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = System.currentTimeMillis() + 5000
            while (System.currentTimeMillis() < deadline) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun click(label: String) {
            var n: AccessibilityNodeInfo? = await(label)
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Could not toggle $label" }
            Thread.sleep(500)
        }
        fun shot(name: String) {
            val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
            File(targetContext.filesDir, name).outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }
            bmp.recycle()
        }
        await("Terminal activity"); await("Running"); await("go test ./...")
        check(node("TOOL_OUTPUT_MARKER") == null) { "Output exposed while collapsed" }
        shot("command-collapsed.png")
        click("Expand command")
        await("TOOL_OUTPUT_MARKER")
        shot("command-expanded.png")
        click("Collapse command")
        val collapseDeadline = System.currentTimeMillis() + 5000
        while (node("TOOL_OUTPUT_MARKER") != null && System.currentTimeMillis() < collapseDeadline) Thread.sleep(100)
        shot("command-recollapsed.png")
        check(node("TOOL_OUTPUT_MARKER") == null) { "Output still exposed after collapse" }
        runOnMainSync { messages = updateChat(messages, "item/completed", org.json.JSONObject("""{"item":{"id":"c","type":"commandExecution","command":"go test ./...","cwd":"/work","status":"completed","aggregatedOutput":"TOOL_OUTPUT_MARKER","exitCode":1}}""")) }
        await("Failed · exit 1")
        click("Expand command"); await("TOOL_OUTPUT_MARKER")
        output.putString("result", "PASS: running card, hidden output, expand/collapse, failed exit status and retained output")
        finish(-1, output)
    } catch (e: Throwable) { output.putString("result", "FAIL: ${e.message} at ${e.stackTrace.firstOrNull { it.fileName == "ChatUiSmoke.kt" }}"); finish(0, output) }
}

/** Remote picker selection and path navigation, without reading any remote machine. */
fun Instrumentation.remoteFilePickerSmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var attached = emptyList<RemoteFile>()
        var browsed = ""
        runOnMainSync { activity.setContent { ModexTheme {
            RemoteFilePicker("/work", RemoteDirectory("/work", "/", listOf(
                RemoteFile("src", "/work/src", true, 0),
                RemoteFile("notes.txt", "/work/notes.txt", false, 48),
                RemoteFile("photo.png", "/work/photo.png", false, 1024),
                RemoteFile("huge.zip", "/work/huge.zip", false, MAX_ATTACHMENT_BYTES + 1)
            ), false), false, "", true, 8, { browsed = it }, { attached = it }, {})
        } } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun click(label: String) {
            val deadline = System.currentTimeMillis() + 5000
            var n = node(label)
            while (n == null && System.currentTimeMillis() < deadline) { Thread.sleep(100); n = node(label) }
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            waitForIdleSync(); Thread.sleep(400)
        }
        click("notes.txt"); click("photo.png")
        check(node("Attach (2)") != null)
        check(node("Over 20 MiB limit") != null)
        click("src"); check(browsed == "/work/src")
        click("Up"); check(browsed == "/")
        val bitmap = uiAutomation.takeScreenshot() ?: error("No picker screenshot")
        File(targetContext.filesDir,"remote-file-picker.png").outputStream().use { bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG,100,it) }; bitmap.recycle()
        click("Attach (2)")
        check(attached.map { it.name } == listOf("notes.txt","photo.png"))
        result.putString("result","PASS: remote picker multi-selection, folder navigation, oversized-file state and attachment confirmation")
        finish(-1,result)
    } catch(e: Throwable) { result.putString("result","FAIL: remote picker: ${e.message}"); finish(0,result) }
}
