package dev.modex

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class AgentActivityTest {
    private fun activity(id: String, action: String, targets: String, thread: String = "chat", turn: String = "turn") =
        JSONObject("""{"threadId":"$thread","turnId":"$turn","item":{"id":"$id","type":"agentActivity","action":"$action","targets":$targets}}""")

    @Test fun invocationCompletionDoesNotClaimSubagentCompleted() {
        val frame = activity("invoke", "completed", """[{"id":"agy-call:one","label":"Reviewer","status":"invocationCompleted"}]""")
        val timeline = agentTimeline(updateChat(emptyList(), "item/completed", frame), "", true).single()
        assertEquals("invocationCompleted", timeline.status)
        assertEquals(listOf("Invocation completed"), timeline.milestones)
    }

    @Test fun liveAndHistoryShareAnIsolatedTimelineWithoutDoubleCounting() {
        val state = SessionState().apply { open("chat") }
        val start = activity("spawn", "started", """[{"id":"one","label":"reviewer","status":"running"},{"id":"two","status":"running"}]""")
        state.event("turn/started", JSONObject("""{"threadId":"chat","turn":{"id":"turn"}}"""))
        state.event("item/started", start)
        state.event("item/completed", start)
        state.event("item/completed", activity("elsewhere", "started", """[{"id":"foreign","status":"running"}]""", thread = "other"))
        assertEquals(2, agentTimeline(state.messages, "turn", true).count { it.status == "running" })
        state.event("item/completed", activity("follow", "interacted", """[{"id":"one"}]"""))
        state.event("turn/completed", JSONObject("""{"threadId":"chat","turn":{"id":"turn","status":"completed"}}"""))
        state.event("item/completed", activity("done", "completed", """[{"id":"one","status":"completed"}]"""))
        state.event("item/completed", activity("wait", "waited", """[{"id":"two","status":"errored"}]"""))
        state.event("item/started", start)
        val completed = agentTimeline(state.messages, "", true)
        assertEquals(listOf("reviewer", "Agent two"), completed.map { it.label })
        assertEquals(listOf("completed", "errored"), completed.map { it.status })
        assertEquals(listOf("Started", "Interaction", "Completed"), completed[0].milestones)
        assertEquals(listOf("Started", "Error"), completed[1].milestones)

        val history = readChat(JSONArray().apply { state.messages.forEach { put(it.toJson()) } })
        assertEquals(completed, agentTimeline(mergeHistoryEntries(history, state.messages), "", true))
        state.open("other")
        assertTrue(agentTimeline(state.messages, "", true).isEmpty())
        state.reset()
        assertTrue(state.messages.isEmpty())
    }

    @Test fun loadingEarlierHistoryDoesNotRenameAnUnnamedAgent() {
        val older = updateChat(emptyList(), "item/completed", activity("old", "started", """[{"id":"older","status":"running"}]"""))
        val recent = updateChat(emptyList(), "item/completed", activity("new", "started", """[{"id":"recent","status":"running"}]"""))
        assertEquals(agentTimeline(recent, "turn", true).single().label,
            agentTimeline(mergeHistoryEntries(older, recent), "turn", true).last().label)
    }

    @Test fun unfinishedCachedObservationsAreUnknownUntilAnActiveTurnIsConfirmed() {
        val frame = activity("spawn", "started", """[{"id":"one","status":"running"}]""")
        val live = updateChat(emptyList(), "item/completed", frame)
        assertEquals("running", agentTimeline(live, "", true).single().status)
        val cache = HistoryCache()
        cache.remember("chat", CachedHistory(live, "", ""))
        assertEquals("unknown", agentTimeline(cache.get("chat")!!.entries, "", true).single().status)
        val history = readChat(JSONArray().put(JSONObject().put("id", "spawn").put("role", "agent_activity")
            .put("turn_id", "turn").put("activity_item", frame.getJSONObject("item"))))
        assertEquals("running", agentTimeline(history, "turn", true).single().status)
        assertEquals("unknown", agentTimeline(history, "", true).single().status)
        assertEquals("unknown", agentTimeline(history, "turn", false).single().status)
    }
}
