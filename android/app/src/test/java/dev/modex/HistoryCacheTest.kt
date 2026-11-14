package dev.modex

import org.junit.Assert.*
import org.junit.Test

class HistoryCacheTest {
    private fun entry(id: String) = ChatEntry(id, "Codex", id, turnId = id)
    @Test fun unchangedNewestPageKeepsOlderCacheButChangedHistoryInvalidatesIt() {
        val cache = HistoryCache()
        cache.latest("a", listOf(entry("new")), "older", "r1")
        cache.remember("a", cache.get("a")!!.copy(entries = listOf(entry("old"), entry("new")), cursor = ""))
        assertEquals(listOf("old", "new"), cache.latest("a", listOf(entry("new")), "older", "r1").entries.map { it.id })
        assertEquals(listOf("replacement"), cache.latest("a", listOf(entry("replacement")), "next", "r2").entries.map { it.id })
        val restored = HistoryCache().apply { restore(cache.toJson()) }
        assertEquals("replacement", restored.get("a")!!.entries.single().id)
        assertNull(restored.get("b"))
    }
    @Test fun cacheBoundsEvictOldSessionsAndOversizedSnapshots() {
        val cache = HistoryCache(maxSessions = 2, maxBytes = 10000)
        cache.latest("a", listOf(entry("a")), "next", "r")
        cache.latest("b", listOf(entry("b")), "", "r")
        cache.latest("c", listOf(entry("c")), "", "r")
        assertNull(cache.get("a"))
        cache.latest("huge", listOf(entry("huge").copy(text = "x".repeat(20000))), "", "r")
        assertNotNull(cache.get("b"))
        assertNull(cache.get("huge"))
        val small = HistoryCache(maxBytes = 10)
        small.latest("a", listOf(entry("a")), "", "r")
        assertNull(small.get("a"))
    }
    @Test fun overlappingPagesPreferNewerStreamedContent() {
        val old = entry("a").copy(text = "partial")
        val new = old.copy(text = "complete")
        assertEquals(listOf(entry("before"), new), mergeHistoryEntries(listOf(entry("before"), old), listOf(new)))
        val state = SessionState()
        state.open("a")
        val version = state.activityVersion("a")
        state.event("item/agentMessage/delta", org.json.JSONObject().put("threadId", "a").put("itemId", "answer").put("delta", "new"))
        assertTrue(state.activityVersion("a") > version)
    }
    @Test fun disconnectHidesMessagesButKeepsRunningTurnForOtherClients() {
        val state = SessionState()
        state.open("a")
        state.loadHistory("a", listOf(entry("message")), true, activeTurnId = "running")
        state.clearSelection()
        assertEquals("", state.selected)
        assertTrue(state.messages.isEmpty())
        assertTrue("a" in state.runningThreads)
        state.loadHistory("a", listOf(entry("late")), false)
        assertTrue(state.messages.isEmpty())
    }
}
