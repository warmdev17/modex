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

}
