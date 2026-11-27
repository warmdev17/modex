package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class SharedAgentChatsTest {
    private fun JSONObject.keysSet() = keys().asSequence().toSet()

    @Test fun runtimeRoutingParsesAllAgentsAndTheirCapabilities() {
        val ready = BackendCapabilities.fromReady(JSONObject("""{
            "ready":true,"instance":"shared","backend":"codex",
            "capabilities":["agent_discovery","runtime_routing","projects","session_lifecycle"]
        }"""))
        val agents = readChatAgents(JSONObject("""{"data":[
            {"id":"codex","label":"Codex","ready":true,"capabilities":["plan_mode","permission_profiles"]},
            {"id":"opencode","label":"OpenCode","ready":true,"capabilities":["attachments"]},
            {"id":"antigravity","label":"Antigravity","ready":false,"reason":"Sign in"},
            {"id":"future","label":"Future","ready":true}
        ]}"""))

        assertTrue(ready.runtimeRouting)
        assertEquals(listOf(CODEX_AGENT, OPENCODE_AGENT, ANTIGRAVITY_AGENT), agents.map { it.id })
        assertEquals(setOf("plan_mode", "permission_profiles"), agents.first().capabilities)
        assertEquals(setOf("attachments"), agents[1].capabilities)
        assertNull(agents.last().capabilities)
        assertEquals("Sign in", agents.last().reason)
    }

    @Test fun legacyConnectionsKeepTheirExistingDefaultAndPicker() {
        val codex = BackendCapabilities.fromReady(JSONObject("""{
            "ready":true,"instance":"legacy","backend":"codex","capabilities":["agent_discovery"]
        }"""))
        val openCode = BackendCapabilities.fromReady(JSONObject("""{
            "ready":true,"instance":"open","backend":"opencode","capabilities":[]
        }"""))

        assertFalse(codex.runtimeRouting)
        assertTrue(codex.agentDiscovery)
        assertEquals(CODEX_AGENT, defaultChatAgent(codex))
        assertFalse(openCode.runtimeRouting)
        assertEquals(OPENCODE_AGENT, defaultChatAgent(openCode))
        assertEquals(OPENCODE_AGENT, connectionThreadAgent(openCode, JSONObject("""{"id":"native-session"}""")))
        assertEquals(CODEX_AGENT, connectionThreadAgent(openCode, JSONObject("""{"id":"native-session","agent":"codex"}""")))
    }

    @Test fun threadAgentPrefersMetadataThenRecognizesAllPrefixes() {
        assertEquals(CODEX_AGENT, threadAgent(JSONObject("""{"id":"oc:legacy","agent":"codex"}""")))
        assertEquals(OPENCODE_AGENT, threadAgent(JSONObject("""{"id":"oc:restored"}""")))
        assertEquals(ANTIGRAVITY_AGENT, threadAgent(JSONObject("""{"id":"agy:restored"}""")))
        assertEquals(CODEX_AGENT, threadAgent(JSONObject("""{"id":"ordinary"}""")))
    }

    @Test fun routedRequestsCarryAgentAndWorkspaceWithoutChangingLegacyRequests() {
        val agents = agentsListParams("/work/project")
        val models = modelsListParams(OPENCODE_AGENT, "/work/project", "model-next", routed = true)
        val threads = threadsListParams(ANTIGRAVITY_AGENT, archived = false, cursor = "thread-next", routed = true)
        val legacyModels = modelsListParams(OPENCODE_AGENT, "/work/project", routed = false)
        val legacyThreads = threadsListParams(CODEX_AGENT, archived = true, routed = false)

        assertEquals(setOf("workspace"), agents.keysSet())
        assertEquals(setOf("agent", "workspace", "limit", "cursor"), models.keysSet())
        assertEquals(OPENCODE_AGENT, models.getString("agent"))
        assertEquals(setOf("agent", "all_workspaces", "archived", "limit", "cursor"), threads.keysSet())
        assertFalse(legacyModels.has("agent"))
        assertFalse(legacyThreads.has("agent"))
        assertTrue(legacyThreads.getBoolean("archived"))
    }

    @Test fun routedCreationUsesExplicitRuntimeAndOnlyCodexPermissionMode() {
        val codex = newThreadParams("gpt-test", "workspace-write", "/work", "project", agent = CODEX_AGENT, explicitAgent = true)
        val openCode = newThreadParams("provider/model", "full-access", "/work", "project", agent = OPENCODE_AGENT, explicitAgent = true)

        assertEquals(CODEX_AGENT, codex.getString("agent"))
        assertEquals("gpt-test", codex.getString("model"))
        assertEquals("workspace-write", codex.getString("permission_mode"))
        assertEquals(OPENCODE_AGENT, openCode.getString("agent"))
        assertEquals("provider/model", openCode.getString("model"))
        assertFalse(openCode.has("permission_mode"))
    }

    @Test fun selectedThreadUsesItsRuntimeCapabilitiesInsteadOfCodexHostControls() {
        val host = BackendCapabilities.fromReady(JSONObject("""{
            "ready":true,"instance":"shared","backend":"codex",
            "capabilities":["runtime_routing","projects","session_status","plan_mode","permission_profiles","session_lifecycle","references","takeover"]
        }"""))
        val agents = listOf(
            ChatAgent(CODEX_AGENT, "Codex", true),
            ChatAgent(OPENCODE_AGENT, "OpenCode", true, capabilities = setOf("references")),
            ChatAgent(ANTIGRAVITY_AGENT, "Antigravity", true, capabilities = emptySet()),
        )

        assertTrue(runtimeCapabilities(host, agents, CODEX_AGENT).planMode)
        val openCode = runtimeCapabilities(host, agents, OPENCODE_AGENT)
        assertEquals("OpenCode", openCode.label)
        assertTrue(openCode.references)
        assertFalse(openCode.planMode || openCode.permissionProfiles || openCode.sessionLifecycle || openCode.takeover)
        val antigravity = runtimeCapabilities(host, agents, ANTIGRAVITY_AGENT)
        assertEquals("Antigravity", antigravity.label)
        assertFalse(antigravity.sessionStatus || antigravity.references)
    }

    @Test fun lateCatalogueResultCannotCrossAgentWorkspaceOrHostGeneration() {
        val requests = ScopedRuntimeRequests()
        requests.select("new", RuntimeScope(OPENCODE_AGENT, "/one"))
        requests.begin(1, "new", "cursor")
        requests.select("new", RuntimeScope(CODEX_AGENT, "/one"))
        assertNull(requests.take(1))

        requests.begin(2, "new")
        requests.select("session", RuntimeScope(OPENCODE_AGENT, "/two"))
        requests.begin(3, "session")
        assertNotNull(requests.take(2))
        requests.invalidate()
        assertNull(requests.take(3))
    }

    @Test fun unavailableSelectedAgentStaysSelectedUntilTheUserChangesIt() {
        val agents = listOf(
            ChatAgent(CODEX_AGENT, "Codex", true),
            ChatAgent(OPENCODE_AGENT, "OpenCode", false, "No model"),
            ChatAgent(ANTIGRAVITY_AGENT, "Antigravity", true),
        )

        assertEquals(OPENCODE_AGENT, safeChatAgent(OPENCODE_AGENT, CODEX_AGENT, agents))
        assertEquals(ANTIGRAVITY_AGENT, safeChatAgent(ANTIGRAVITY_AGENT, CODEX_AGENT, agents))
        assertEquals(CODEX_AGENT, safeChatAgent("future", CODEX_AGENT, agents))
        assertEquals(OPENCODE_AGENT, safeChatAgent(OPENCODE_AGENT, OPENCODE_AGENT, emptyList()))
    }

    @Test fun workspaceReadinessRefreshKeepsOptionalAgentVisibleButUnavailable() {
        val agents = listOf(
            ChatAgent(CODEX_AGENT, "Codex", true),
            ChatAgent(OPENCODE_AGENT, "OpenCode", true, capabilities = setOf("references")),
        )

        val checking = checkingChatAgents(agents, CODEX_AGENT)

        assertTrue(checking.first().ready)
        assertFalse(checking.last().ready)
        assertEquals("Checking availability for this workspace…", checking.last().reason)
        assertEquals(setOf("references"), checking.last().capabilities)
    }

    @Test fun sharedPagesKeepSuccessfulAgentsIndependentAndDeduplicateIds() {
        val pages = RuntimeThreadPages(listOf(CODEX_AGENT, OPENCODE_AGENT, ANTIGRAVITY_AGENT))
        assertEquals(listOf(CODEX_AGENT to ""), pages.initialTargets(archived = true))
        pages.begin(1, CODEX_AGENT)
        pages.begin(2, OPENCODE_AGENT)
        pages.begin(3, ANTIGRAVITY_AGENT)
        assertTrue(pages.accept(1, listOf("codex", "same"), "codex-next"))
        assertTrue(pages.accept(2, listOf("open", "same"), "open-next"))
        assertTrue(pages.fail(3))

        assertEquals(listOf("codex", "same", "open"), pages.ids)
        assertFalse(pages.loading)
        assertEquals(
            listOf(CODEX_AGENT to "codex-next", OPENCODE_AGENT to "open-next"),
            pages.moreTargets(),
        )
    }

    @Test fun runtimeFilterAndSearchComposeWithoutDuplicatingProjectRows() {
        val threads = listOf("codex", "open", "agy")
        val agents = mapOf("codex" to CODEX_AGENT, "open" to OPENCODE_AGENT, "agy" to ANTIGRAVITY_AGENT)
        val titles = mapOf("codex" to "Fix checkout", "open" to "Fix search", "agy" to "Review checkout")
        val workspaces = threads.associateWith { "/work/modex" }

        assertEquals(listOf("open"), visibleThreads(threads, OPENCODE_AGENT, "fix", agents, titles, workspaces))
        assertEquals(listOf("codex", "agy"), visibleThreads(threads, ALL_AGENTS, "checkout", agents, titles, workspaces))
    }
}
