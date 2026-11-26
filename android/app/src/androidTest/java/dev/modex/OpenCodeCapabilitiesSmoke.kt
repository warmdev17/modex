package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import org.json.JSONObject

/** Render-only capability fixture: no host, provider request or conversation mutation. */
internal fun Instrumentation.openCodeCapabilitiesSmoke(projectsRead: Boolean = false) {
    val output = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        val capabilityList = if (projectsRead) "[\"projects_read\"]" else "[]"
        val capabilities = BackendCapabilities.fromReady(JSONObject("""{"ready":true,"instance":"open","backend":"opencode","capabilities":$capabilityList}"""))
        val approval = JSONObject("""{"decision_id":"approval","thread_id":"thread","context":{"command":"echo core-chat","availableDecisions":["accept","decline"]}}""")
        var started = emptyList<String>()
        runOnMainSync { activity.setContent { ModexTheme { Surface { ChatWorkspace(
            host = "Fixture", connected = true, connecting = false, selected = "thread",
            threads = listOf("thread"), titles = mapOf("thread" to "OpenCode conversation"),
            messages = listOf(ChatEntry(role = "You", text = "Keep core chat", status = "completed", durationMs = 2_000)),
            busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "open-model",
            decisions = listOf(approval), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> },
            onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {}, onHome = {},
            usageWarning = "Codex usage warning must stay hidden", takeoverSession = "thread",
            registeredProjects = if (projectsRead) listOf(RemoteProject("project-1", "OpenCode project", listOf("/work/modex"))) else listOf(RemoteProject("cached", "Cached project must stay hidden", listOf("/cached"))),
            projectMembership = if (projectsRead) mapOf("thread" to "project-1") else emptyMap(),
            threadWorkspaces = if (projectsRead) mapOf("thread" to "/work/modex") else emptyMap(),
            backendCapabilities = capabilities,
            models = listOf(ModelOption("open-model", "Open model", "", emptyList())), hostModel = "open-model",
            onCreateInProject = { model, mode, effort, root, project -> started = listOf(model, mode, effort, root, project) },
        ) } } } }

        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(value: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (value == null) return null
                if (value.text?.toString() == label || value.contentDescription?.toString() == label) return value
                for (index in 0 until value.childCount) walk(value.getChild(index))?.let { return it }
                return null
            }
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = System.currentTimeMillis() + 6_000
            while (System.currentTimeMillis() < deadline) { node(label)?.let { return it }; Thread.sleep(100) }
            error("Missing $label")
        }
        fun absent(vararg labels: String) = labels.forEach { check(node(it) == null) { "Unsupported OpenCode control is visible: $it" } }
        fun touch(label: String) {
            var target: AccessibilityNodeInfo? = await(label)
            while (target != null && !target.isClickable) target = target.parent
            check(target?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            waitForIdleSync(); Thread.sleep(250)
        }

        await("Fixture · OpenCode · Connected · SSH")
        await("Message OpenCode")
        await("Worked · 2s")
        await("Pending approval")
        await("Choose model")
        await("Attach files")
        await("Send message")
        absent("Default", "Enable Plan", "Mode unknown · Refresh", "Open session status", "Codex usage warning must stay hidden", "Take over on phone")

        touch("Attach files")
        await("From this phone"); await("From connected computer")
        absent("Skills & plugins")
        check(uiAutomation.performGlobalAction(android.accessibilityservice.AccessibilityService.GLOBAL_ACTION_BACK))

        touch("Connection menu")
        absent("Session settings")
        await("Connection settings")
        check(uiAutomation.performGlobalAction(android.accessibilityservice.AccessibilityService.GLOBAL_ACTION_BACK))

        touch("Open sessions")
        await("New chat")
        if (projectsRead) {
            await("Search sessions or projects")
            await("Projects")
            await("OpenCode project")
            absent("New project", "Archived conversations", "Actions for OpenCode conversation")
            touch("New chat")
            await("New conversation")
            touch("Outside projects")
            await("/work/modex")
            touch("OpenCode project")
            touch("Create chat")
            runOnMainSync { check(started.takeLast(2) == listOf("/work/modex", "project-1")) { "Project selection was not preserved: $started" } }
        } else {
            await("Search sessions")
            absent("New project", "Archived conversations", "Projects", "Cached project must stay hidden", "Actions for OpenCode conversation")
            touch("New chat")
            await("New conversation")
            await("Open model")
            absent("Outside projects", "Host default", "Read-only", "Workspace write", "Full access")
        }

        output.putString("result", if (projectsRead) "PASS: OpenCode read-only projects list and select without project creation" else "PASS: OpenCode keeps core chat, files, models and approvals while optional controls remain absent")
        finish(-1, output)
    } catch (failure: Throwable) {
        output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}")
        finish(0, output)
    }
}
