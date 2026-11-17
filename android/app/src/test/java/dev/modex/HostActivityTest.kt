package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class HostActivityTest {
    @Test fun oneRowPerConversationWithWaitingFirst() {
        val decisions = listOf(JSONObject().put("thread_id", "review"), JSONObject().put("thread_id", "review"))
        val rows = hostActivity("laptop", setOf("working", "review"), decisions, mapOf("review" to "Fix login"))
        assertEquals(listOf("review", "working"), rows.map { it.threadId })
        assertEquals(2, rows.first().waiting)
        assertTrue(rows.first().running)
        assertEquals("Fix login", rows.first().title)
    }
    @Test fun idleHostHasNoActivityRows() {
        assertTrue(hostActivity("host", emptySet(), emptyList(), emptyMap()).isEmpty())
    }
    @Test fun sameThreadIdOnDifferentRemotesIsStillDistinct() {
        val a=hostActivity("a", setOf("thread"), emptyList(), emptyMap()).single()
        val b=hostActivity("b", setOf("thread"), emptyList(), emptyMap()).single()
        assertNotEquals(a.key, b.key)
        assertEquals("a", a.hostId)
        assertEquals("thread", a.threadId)
    }
    @Test fun unscopedQuestionIsNotAssignedToAnArbitraryConversation() {
        val rows=hostActivity("host", setOf("running"), listOf(JSONObject().put("decision_id", "question")), emptyMap())
        assertEquals("",rows.first().threadId)
        assertEquals(1,rows.first().waiting)
        assertEquals("running",rows.last().threadId)
    }
}
