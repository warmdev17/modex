package dev.modex

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ChatInteractionTest {
    @Test fun reasoningSummarySectionsSurviveCompletionWithoutRawContent() {
        fun delta(index: Int, text: String) = JSONObject().put("itemId", "r").put("turnId", "t").put("summaryIndex", index).put("delta", text)
        var entries = updateChat(emptyList(), "item/reasoning/summaryTextDelta", delta(0, "Checking"))
        entries = updateChat(entries, "item/reasoning/summaryTextDelta", delta(0, " files"))
        entries = updateChat(entries, "item/reasoning/summaryTextDelta", delta(1, "Planning changes"))
        assertEquals("Checking files\n\nPlanning changes", entries.single().text)
        assertEquals("reasoning", entries.single().role)
        entries = updateChat(entries, "item/completed", JSONObject().put("turnId", "t").put("item", JSONObject().put("id", "r").put("type", "reasoning").put("summary", JSONArray().put("Final summary")).put("content", JSONArray().put("PRIVATE_RAW"))))
        assertEquals("Final summary", entries.single().text)
        assertEquals(entries, updateChat(entries, "item/reasoning/textDelta", delta(0, "PRIVATE_RAW")))
        assertEquals(entries, updateChat(entries, "item/reasoning/summaryTextDelta", delta(Int.MAX_VALUE, "unsafe")))
    }

    @Test fun stopTargetsExactTurnAndAcknowledgementDoesNotFinishWork() {
        val state = SessionState()
        state.open("a")
        state.sent(ChatEntry(role = "You", text = "work"))
        assertNull(state.requestStop())
        state.event("turn/started", turn("a", "new"))
        val stop = state.requestStop()!!
        assertEquals("new", stop.getString("turn_id"))
        assertEquals("a", stop.getString("thread_id"))
        assertTrue(state.stopping)
        assertNull(state.requestStop())
        state.event("turn/completed", turn("a", "old"))
        assertTrue(state.busy)
        assertTrue(state.stopping)
        state.event("turn/completed", turn("a", "new"))
        assertFalse(state.busy)
        assertFalse(state.stopping)
    }

    @Test fun timersAndStopStateStayWithTheirOwnThread() {
        var now = 1000L
        val state = SessionState { now }
        state.open("a"); state.sent(ChatEntry(role = "You", text = "first"))
        state.event("turn/started", turn("a", "t-a"))
        assertEquals("t-a", state.messages.single().turnId)
        now = 4500
        state.event("turn/completed", turn("a", "t-a").apply { getJSONObject("turn").put("status", "interrupted") })
        assertEquals(3500L, state.messages.single().durationMs)
        assertEquals("interrupted", state.messages.single().status)
        val restored = readChat(JSONArray().put(state.messages.single().toJson())).single()
        assertEquals(state.messages.single(), restored)
        state.open("b"); state.sent(ChatEntry(role = "You", text = "second"))
        state.event("turn/started", turn("b", "t-b"))
        state.requestStop()
        state.event("turn/completed", turn("a", "t-a"))
        assertTrue(state.busy)
        assertTrue(state.stopping)
    }

    @Test fun lateStartAcknowledgementCannotReactivateCompletedTurn() {
        val state = SessionState()
        state.open("a"); state.sent(ChatEntry(role = "You", text = "go"))
        state.event("turn/started", turn("a", "t"))
        state.event("turn/completed", turn("a", "t"))
        state.acknowledgeTurn("a", JSONObject().put("id", "t"))
        assertFalse(state.busy)
    }

    @Test fun historyProvidesStopTargetAndTimingWithoutInventingMissingTimes() {
        val state = SessionState()
        state.open("a")
        state.loadHistory("a", emptyList(), true, activeTurnId = "known")
        assertEquals("known", state.requestStop()!!.getString("turn_id"))
        val old = readChat(JSONArray().put(JSONObject().put("role", "user").put("text", "old"))).single()
        assertNull(old.startedAtMs)
        assertNull(old.durationMs)
        val completed = readChat(JSONArray().put(JSONObject().put("role", "user").put("text", "done").put("turn_id", "t").put("turn_status", "completed").put("duration_ms", 2100))).single()
        assertEquals(2100L, completed.durationMs)
    }

    @Test fun liveRevealNeverSplitsUnicodeSurrogatesAndAlwaysFinishes() {
        val text = "Hello 👋🏽 bradar 🇻🇳"
        var end = 0
        var steps = 0
        while (end < text.length) {
            val next = nextRevealEnd(text, end)
            assertTrue(next > end)
            assertFalse(next < text.length && Character.isLowSurrogate(text[next]))
            end = next; steps++
            assertTrue(steps < 100)
        }
        assertEquals(text.length, end)
        assertEquals("1m 1s", formatWorkDuration(61000))
    }

    @Test fun historyReconcilesUncertainStopWithoutReplaying() {
        val state = SessionState()
        state.open("a")
        state.loadHistory("a", emptyList(), true, activeTurnId = "old")
        assertNotNull(state.requestStop())
        state.loadHistory("a", emptyList(), true, activeTurnId = "new")
        assertFalse(state.stopping)
        assertEquals("new", state.requestStop()!!.getString("turn_id"))
        state.loadHistory("a", emptyList(), false)
        assertFalse(state.busy)
        assertFalse(state.stopping)
    }

    private fun turn(thread: String, turn: String) = JSONObject().put("threadId", thread).put("turn", JSONObject().put("id", turn))
}
