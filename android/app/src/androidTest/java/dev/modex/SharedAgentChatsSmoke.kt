package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface

internal fun Instrumentation.sharedAgentChatsSmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var selectedScope = "" to ""
        var created = emptyList<String>()
        val ids = listOf("codex-fixture", "oc:fixture", "agy:fixture")
        val agents = listOf("codex", "opencode", "antigravity")
        val names = listOf("Codex", "OpenCode", "Antigravity")
        val workspace = "/fixture/modex"
        runOnMainSync {
            activity.setContent { ModexTheme { Surface {
                ChatWorkspace(
                    host = "fixture", connected = true, connecting = false, selected = "",
                    threads = ids, titles = ids.zip(names.map { "$it fixture chat" }).toMap(),
                    messages = emptyList(), busy = false, historyLoading = false, hasMore = false,
                    error = "", sessionSettings = "", sessionModel = "", decisions = emptyList(),
                    answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {},
                    onCreate = { _, _, _ -> error("Agent selection lost") }, onSend = { _, _, _, _ -> false },
                    onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {},
                    hostModel = "local-fixture/fixture",
                    chatAgents = agents.zip(names).map { (id, name) -> ChatAgent(id, name, true) },
                    threadAgents = ids.zip(agents).toMap(),
                    threadWorkspaces = ids.associateWith { workspace },
                    projectMembership = ids.associateWith { "shared-project" },
                    registeredProjects = listOf(RemoteProject("shared-project", "Shared fixture", listOf(workspace))),
                    backendCapabilities = BackendCapabilities.LegacyCodex.copy(agentDiscovery = true, runtimeRouting = true),
                    newChatModels = listOf(ModelOption("local-fixture/fixture", "Fixture model", "", emptyList())),
                    newChatModelCatalogStatus = "",
                    onNewChatScope = { agent, path -> selectedScope = agent to path },
                    onCreateWithAgent = { agent, model, _, _, path, project -> created = listOf(agent, model, path, project) },
                )
            } } }
        }
        fun find(label: String): List<AccessibilityNodeInfo> {
            fun walk(node: AccessibilityNodeInfo?): List<AccessibilityNodeInfo> {
                if (node == null) return emptyList()
                val match = node.text?.toString() == label || node.contentDescription?.toString() == label
                return (if (match) listOf(node) else emptyList()) + (0 until node.childCount).flatMap { walk(node.getChild(it)) }
            }
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = SystemClock.elapsedRealtime() + 5500
            while (SystemClock.elapsedRealtime() < deadline) {
                find(label).firstOrNull()?.let { return it }
                SystemClock.sleep(80)
            }
            error("Missing $label")
        }
        fun click(label: String) {
            var node: AccessibilityNodeInfo? = await(label)
            while (node != null && !node.isClickable) node = node.parent
            check(node?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            waitForIdleSync()
            SystemClock.sleep(280)
        }
        click("Open sessions")
        for (agent in names) {
            await("$agent fixture chat")
            check(find("$agent fixture chat").size == 1) { "Duplicate $agent chat" }
        }
        click("OpenCode")
        await("OpenCode fixture chat")
        check(find("Codex fixture chat").isEmpty() && find("Antigravity fixture chat").isEmpty())
        click("All")
        await("Codex fixture chat")
        click("New chat in Shared fixture")
        click("OpenCode")
        runOnMainSync { check(selectedScope == ("opencode" to workspace)) }
        click("Create chat")
        runOnMainSync { check(created == listOf("opencode", "local-fixture/fixture", workspace, "shared-project")) }
        result.putString("result", "PASS: shared project rows, agent filter and scoped OpenCode chat creation")
        finish(-1, result)
    } catch (failure: Throwable) {
        result.putString("result", "FAIL: ${failure.message}")
        finish(0, result)
    }
}
