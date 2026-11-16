package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class PlanReviewTest {
    @Test fun streamsPlanAndFinalItemIsAuthoritative() {
        var entries = updateChat(emptyList(), "item/plan/delta", JSONObject("""{"itemId":"plan","turnId":"turn","delta":"Draft"}"""))
        assertEquals("plan", entries.single().role)
        entries = updateChat(entries, "item/completed", JSONObject("""{"turnId":"turn","item":{"id":"plan","type":"plan","text":"Final plan"}}"""))
        assertEquals("Final plan",entries.single().text)
        assertEquals("plan",readChat(org.json.JSONArray().put(entries.single().toJson())).single().role)
    }
    @Test fun onlyLatestCompletedTurnOffersPlanActions() {
        val plan=ChatEntry("plan","plan","Implement feature",status="completed",turnId="turn")
        assertEquals("plan",pendingPlan(listOf(plan),false,emptySet())?.id)
        assertNull(pendingPlan(listOf(plan),true,emptySet()))
        assertNull(pendingPlan(listOf(plan,ChatEntry("user","You","Change this",turnId="next")),false,emptySet()))
        assertNull(pendingPlan(listOf(plan),false,setOf(planKey(plan))))
        assertNull(pendingPlan(listOf(plan.copy(status="interrupted")),false,emptySet()))
    }
    @Test fun nativePlanDeltaReachesSelectedSessionAndDoesNotCreateFakeBusyTurn() {
        val state=SessionState(); state.open("source")
        state.event("item/plan/delta",JSONObject("""{"threadId":"source","turnId":"turn","itemId":"plan","delta":"Plan"}"""))
        assertEquals("Plan",state.messages.single().text)
        assertFalse(state.busy)
        state.event("item/plan/delta",JSONObject("""{"threadId":"other","turnId":"turn","itemId":"plan","delta":"Hidden"}"""))
        assertEquals(1,state.messages.size)
    }
    @Test fun handledPlanSurvivesJsonAndFailedTurnDoesNotOfferImplementation() {
        val plan = ChatEntry("plan", "plan", "Plan", status = "completed", turnId = "turn", planHandled = true)
        val restored = readChat(org.json.JSONArray().put(plan.toJson()))
        assertTrue(restored.single().planHandled)
        assertNull(pendingPlan(restored, false, emptySet()))
        val state = SessionState(); state.open("source")
        state.event("turn/started", JSONObject("""{"threadId":"source","turn":{"id":"turn"}}"""))
        state.event("item/completed", JSONObject("""{"threadId":"source","turnId":"turn","item":{"id":"plan","type":"plan","text":"Plan"}}"""))
        state.event("turn/completed", JSONObject("""{"threadId":"source","turn":{"id":"turn","status":"interrupted"}}"""))
        assertFalse(state.busy)
        assertNull(pendingPlan(state.messages, false, emptySet()))
    }

}
