package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class AgentConversationTest {
    @Test fun mergesReadOnlyHistoryAndLiveEventsForOpenedAgentOnly() {
        val state = AgentConversation()
        state.open(AgentTimeline("child", "Reviewer", "running", emptyList()))
        state.begin(1, older = false)
        fun delta(thread: String, text: String) = JSONObject(
            """{"threadId":"$thread","turnId":"turn","itemId":"answer","delta":"$text"}"""
        )
        state.event("item/agentMessage/delta", delta("other-child", "foreign"))
        state.event("item/agentMessage/delta", delta("parent", "parent"))
        state.event("item/agentMessage/delta", delta("child", "world"))
        assertEquals(listOf("world"), state.messages.map { it.text })

        state.result(1, JSONObject("""{"thread":{"id":"child"},"messages":[{"id":"answer","role":"assistant","turn_id":"turn","text":"Hello "}],"next_cursor":"before","active_turn":true,"active_turn_id":"turn"}"""))
        assertEquals(listOf("Hello world"), state.messages.map { it.text })
        assertEquals("before", state.cursor)
        state.begin(2, older = true)
        state.result(2, JSONObject("""{"thread":{"id":"child"},"messages":[{"id":"task","role":"user","turn_id":"earlier","text":"Read the code"}]}"""))
        assertEquals(listOf("Read the code", "Hello world"), state.messages.map { it.text })
    }

    @Test fun lateResponseCannotReplaceAnotherAgentOrDismissedView() {
        val state = AgentConversation()
        state.open(AgentTimeline("first", "First", "running", emptyList()))
        state.begin(3, older = false)
        state.open(AgentTimeline("second", "Second", "completed", emptyList()))
        assertFalse(state.result(3, JSONObject("""{"thread":{"id":"first"},"messages":[]}""")))
        state.begin(4, older = false)
        state.close()
        assertFalse(state.result(4, null))
        assertNull(state.agent)
        assertTrue(state.messages.isEmpty())
    }
}
