package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class AntigravityTest {
    private fun JSONObject.keysSet() = keys().asSequence().toSet()

    @Test fun agentCatalogKeepsKnownRuntimesAndReadinessReason() {
        val agents = readChatAgents(JSONObject("""{"data":[
            {"id":"codex","label":"Codex","ready":true},
            {"id":"antigravity","label":"Antigravity","ready":false,"reason":"Sign in to Antigravity"},
            {"id":"future","label":"Future","ready":true}
        ]}"""))

        assertEquals(listOf("codex", "antigravity"), agents.map { it.id })
        assertTrue(agents.first().ready)
        assertEquals("Sign in to Antigravity", agents.last().reason)
    }

    @Test fun antigravityChatCreationOmitsCodexSettings() {
        val params = newThreadParams(
            model = "gpt-test",
            mode = "full-access",
            workspace = "/work/project",
            projectId = "project-1",
            agent = "antigravity",
        )

        assertEquals(setOf("agent", "workspace", "project_id"), params.keysSet())
        assertEquals("antigravity", params.getString("agent"))
    }

    @Test fun standaloneAntigravityChatKeepsStandaloneContract() {
        val params = newThreadParams("gpt-test", "read-only", standalone = true, agent = "antigravity")

        assertEquals(setOf("agent", "standalone"), params.keysSet())
        assertTrue(params.getBoolean("standalone"))
    }

    @Test fun antigravityAttachmentTurnsOmitCodexOnlyFields() {
        val reference = PromptReference("skill", "review", "/work/SKILL.md", "Review", "", "review", true, "@review")
        val params = turnStartParams(
            "agy:thread",
            "hello @review",
            "gpt-test",
            "high",
            attachments = listOf(Attachment("/tmp/a.txt", "a.txt", "file", 1)),
            references = listOf(reference),
            agent = "antigravity",
        )

        assertEquals(setOf("thread_id", "text", "attachments"), params.keysSet())
        assertEquals("/tmp/a.txt", params.getJSONArray("attachments").getJSONObject(0).getString("path"))
        assertFalse(params.has("model"))
        assertFalse(params.has("effort"))
        assertFalse(params.has("references"))
    }

    @Test fun antigravityFollowUpContainsOnlyTheExactTurnTextAndWorkspace() {
        val params = turnSteerParams("agy:thread", "exact-turn", "continue", "/work/project")

        assertEquals(setOf("thread_id", "turn_id", "text", "workspace"), params.keysSet())
        assertEquals("agy:thread", params.getString("thread_id"))
        assertEquals("exact-turn", params.getString("turn_id"))
        assertEquals("continue", params.getString("text"))
        assertEquals("/work/project", params.getString("workspace"))
    }

    @Test fun antigravityFollowUpAcknowledgementRequiresTheCapturedTurn() {
        assertTrue(validSteerAcknowledgement(JSONObject("""{"turnId":"exact-turn"}"""), "exact-turn"))
        assertFalse(validSteerAcknowledgement(JSONObject("""{"turnId":"other-turn"}"""), "exact-turn"))
        assertFalse(validSteerAcknowledgement(JSONObject("""{"turn":{"id":"exact-turn"}}"""), "exact-turn"))
        assertFalse(validSteerAcknowledgement(JSONObject(), "exact-turn"))
    }

    @Test fun threadMetadataWinsBeforeAntigravityPrefixFallback() {
        assertEquals("codex", threadAgent(JSONObject("""{"id":"agy:legacy","agent":"codex"}""")))
        assertEquals("antigravity", threadAgent(JSONObject("""{"id":"agy:restored"}""")))
        assertEquals("codex", threadAgent(JSONObject("""{"id":"ordinary"}""")))
    }

    @Test fun nativeSessionOptionsUseAcknowledgedChoicesAndLabels() {
        val options = readAntigravityOptions(JSONObject("""{"options":[
            {"id":"model","currentValue":"fixture-alt","options":[
                {"value":"fixture-default","name":"Default"},{"value":"fixture-alt","name":"Alternate"}]},
            {"id":"thought_level","currentValue":"high","options":[{"value":"high","name":"High"}]},
            {"id":"unsupported","currentValue":"a","options":[{"value":"a","name":"Private"}]}
        ]}"""))

        assertEquals(listOf("model", "thought_level"), options.map { it.id })
        assertEquals(listOf("Alternate", "High"), options.map { it.currentLabel })
        assertEquals(setOf("fixture-default", "fixture-alt"), options.first().values.keys)
        assertTrue(readAntigravityOptions(JSONObject("""{"options":[]}""")).isEmpty())
    }

    @Test fun legacyAgentHostCanAdvertiseOptionsWithoutEnablingSharedRouting() {
        val host = BackendCapabilities.fromReady(JSONObject("""{
            "backend":"codex","capabilities":["agent_discovery","antigravity_session_options","plan_mode"]
        }"""))
        val agy = runtimeCapabilities(host, emptyList(), ANTIGRAVITY_AGENT)
        assertTrue(agy.supports("threads/options"))
        assertFalse(agy.planMode)
        assertFalse(host.runtimeRouting)
    }
}
