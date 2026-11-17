package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class SessionStateTest {
    private fun started(id: String, turn: String) = JSONObject().put("threadId", id).put("turn", JSONObject().put("id", turn))

    @Test fun switchingAwayClearsOldChatWithoutStoppingItsTurn() {
        val state = SessionState()
        state.open("a")
        state.loadHistory("a", listOf(ChatEntry(role = "You", text = "private old prompt")), false)
        state.event("turn/started", started("a", "turn-a"))
        assertTrue(state.busy)
        state.clearSelection()
        assertEquals("", state.selected)
        assertTrue(state.messages.isEmpty())
        assertFalse(state.busy)
        assertEquals(setOf("a"), state.runningThreads)
        state.event("item/agentMessage/delta", JSONObject().put("threadId", "a").put("itemId", "reply").put("delta", "background output"))
        assertTrue(state.messages.isEmpty())
        state.loadHistory("a", listOf(ChatEntry(role = "Codex", text = "late old history")), true)
        assertTrue(state.messages.isEmpty())
    }

    @Test fun backgroundCompletionDoesNotStopSelectedSession() {
        val state = SessionState()
        state.event("turn/started", started("a", "turn-a"))
        state.open("b")
        state.sent(ChatEntry(role = "You", text = "second prompt"))
        state.event("turn/started", started("b", "turn-b"))
        assertEquals(setOf("a", "b"), state.runningThreads)
        state.event("turn/completed", started("a", "turn-a"))
        assertTrue(state.busy)
        assertEquals(setOf("b"), state.runningThreads)
        assertEquals("second prompt", state.messages.single().text)
        state.event("turn/completed", started("b", "turn-b"))
        assertFalse(state.busy)
    }

    @Test fun staleCompletionAndBackgroundRejectionDoNotClearNewWork() {
        val state = SessionState()
        state.open("b")
        state.sent(ChatEntry(role = "You", text = "pending"))
        state.event("turn/started", started("b", "new-turn"))
        state.event("turn/completed", started("b", "old-turn"))
        state.rejectTurn("a")
        assertTrue(state.busy)
        state.rejectTurn("b")
        assertTrue(state.busy) // Observed turn/started beats a late request error.
        state.event("turn/completed", started("b", "new-turn"))
        state.sent(ChatEntry(role = "You", text = "rejected prompt"))
        state.rejectTurn("b")
        assertFalse(state.busy)
    }

    @Test fun activityAndSelectionRestoreWithoutMixingHistory() {
        val state = SessionState()
        state.event("turn/started", started("a", "turn-a"))
        state.event("turn/started", started("b", "turn-b"))
        val restored = SessionState()
        restored.restoreActivity(JSONObject(state.activityJson().toString()))
        restored.open("b")
        restored.loadHistory("a", listOf(ChatEntry(role = "Codex", text = "wrong session")), false)
        assertTrue(restored.messages.isEmpty())
        restored.loadHistory("b", listOf(ChatEntry(role = "Codex", text = "correct session")), true)
        assertEquals(setOf("a", "b"), restored.runningThreads)
        assertTrue(restored.busy)
        assertEquals("correct session", restored.messages.single().text)
        restored.reset()
        assertTrue(restored.runningThreads.isEmpty())
        assertEquals("", restored.selected)
        assertTrue(restored.messages.isEmpty())
    }
    @Test fun lateHistoryCannotUndoNewerTurnActivity() {
        val state = SessionState()
        state.open("a")
        val beforeStart = state.activityVersion("a")
        state.event("turn/started", started("a", "turn-a"))
        state.loadHistory("a", emptyList(), false, beforeStart)
        assertTrue(state.busy)
        val beforeCompletion = state.activityVersion("a")
        state.event("turn/completed", started("a", "turn-a"))
        state.loadHistory("a", emptyList(), true, beforeCompletion)
        assertFalse(state.busy)
        assertTrue(state.runningThreads.isEmpty())
    }

    @Test fun retryNoticePersistsWithItsTurnAndClearsOnProgress() {
        val state = SessionState()
        state.open("a")
        state.sent(ChatEntry(role = "You", text = "prompt"))
        state.event("turn/started", started("a", "t"))
        state.event("error", JSONObject("""{"threadId":"a","turnId":"t","notice":{"category":"stream_disconnected","message":"Connection lost. Retrying…","retrying":true}}"""))
        val encoded = state.messages.single().toJson()
        assertEquals("stream_disconnected", encoded.optJSONObject("turn_notice")?.optString("category"))
        assertTrue(state.busy)
        assertEquals(encoded.toString(), readChat(org.json.JSONArray().put(encoded)).single().toJson().toString())
        state.event("item/agentMessage/delta", JSONObject("""{"threadId":"a","turnId":"t","itemId":"reply","delta":"Partial"}"""))
        assertTrue(state.messages.first().toJson().isNull("turn_notice"))
        state.event("turn/completed", JSONObject("""{"threadId":"a","turn":{"id":"t","status":"interrupted"}}"""))
        assertFalse(state.busy)
        assertEquals("interrupted", state.messages.first().status)
        assertEquals("Partial", state.messages.last().text)
    }
    @Test fun oldCompletionCannotStopUnacknowledgedNewPrompt() {
        val state = SessionState()
        state.open("a")
        state.sent(ChatEntry(role = "You", text = "new prompt"))
        state.event("turn/completed", started("a", "old"))
        assertTrue(state.busy)
    }

    @Test fun staleNoticeAndIdleStatusDoNotOverwriteCurrentTurn() {
        val state = SessionState()
        state.open("a"); state.sent(ChatEntry(role = "You", text = "new"))
        state.event("turn/started", started("a", "new"))
        state.event("error", JSONObject("""{"threadId":"a","turnId":"old","notice":{"category":"turn_failed","message":"Old error","retrying":false}}"""))
        state.event("thread/status/changed", JSONObject("""{"threadId":"a","status":{"type":"idle"}}"""))
        assertTrue(state.busy)
        assertNull(state.messages.single().notice)
        assertEquals("inProgress", state.messages.single().status)
        val version = state.activityVersion("a")
        state.event("turn/started", started("a", "newer"))
        state.reconcileActivity("a", false, "", version)
        assertTrue(state.busy)
        state.reconcileActivity("a", false, "", state.activityVersion("a"))
        assertFalse(state.busy)
        // Observation may clear activity, but cannot invent a completed/interrupted prompt.
        assertEquals("inProgress", state.messages.single().status)
    }
    @Test fun nonselectedReconcileDoesNotChangeVisibleHistory() {
        val state = SessionState()
        state.open("a"); state.sent(ChatEntry(role = "You", text = "keep"))
        state.event("turn/started", started("b", "background"))
        state.reconcileActivity("b", false, "", state.activityVersion("b"))
        assertEquals(setOf("a"), state.runningThreads)
        assertEquals("keep", state.messages.single().text)
    }

    @Test fun oldHostKnownFailuresAreSafeAndCanonicalNoticeWins() {
        val old = JSONObject("""{"error":{"message":"stream closed before completion https://secret:key@provider.invalid","codexErrorInfo":"other"},"willRetry":true}""")
        val notice = eventTurnNotice(old)!!
        assertEquals("stream_disconnected", notice.category)
        assertTrue(notice.retrying)
        assertFalse(notice.message.contains("secret"))
        old.put("notice", JSONObject("""{"category":"rate_limit_exceeded","message":"The provider is limiting requests.","retrying":false,"http_status":429}"""))
        assertEquals(429, eventTurnNotice(old)!!.httpStatus)
        assertFalse(eventTurnNotice(old)!!.retrying)
    }

    @Test fun interruptedToolKeepsPartialOutputWithoutRunningOrSuccessIndicator() {
        val state = SessionState()
        state.open("a"); state.event("turn/started", started("a", "t"))
        state.event("item/started", JSONObject("""{"threadId":"a","turnId":"t","item":{"id":"tool","type":"commandExecution","command":"test","status":"inProgress"}}"""))
        state.event("item/commandExecution/outputDelta", JSONObject("""{"threadId":"a","turnId":"t","itemId":"tool","delta":"partial output"}"""))
        state.event("turn/completed", JSONObject("""{"threadId":"a","turn":{"id":"t","status":"interrupted"}}"""))
        assertEquals("partial output", state.messages.single().text)
        assertEquals("interrupted", state.messages.single().status)
        assertFalse(state.busy)
    }

    @Test fun lateOldItemCannotReviveInterruptedToolOrAffectNewTurn() {
        val state = SessionState()
        state.open("a"); state.event("turn/started", started("a", "old"))
        val item = JSONObject("""{"threadId":"a","turnId":"old","item":{"id":"tool","type":"commandExecution","command":"test","status":"inProgress"}}""")
        state.event("item/started", item)
        state.event("turn/completed", JSONObject("""{"threadId":"a","turn":{"id":"old","status":"interrupted"}}"""))
        val terminal = state.messages
        state.event("item/started", item)
        assertEquals(terminal, state.messages)
        state.sent(ChatEntry(role = "You", text = "new prompt"))
        val awaiting = state.messages
        state.event("item/started", item)
        assertEquals(awaiting, state.messages)
        state.event("turn/started", started("a", "new"))
        val current = state.messages
        state.event("item/started", item)
        assertEquals(current, state.messages)
        assertTrue(state.busy)
    }

}
