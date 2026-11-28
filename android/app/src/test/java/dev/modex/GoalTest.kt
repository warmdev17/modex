package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class GoalTest {
    @Test fun parsesAcknowledgedAndNativeGoalsWithoutInventingABudget() {
        val acknowledged = readGoalResult(JSONObject("""{"goal":{"thread_id":"thread-1","objective":"Ship the modal","status":"paused","token_budget":1200,"tokens_used":345,"time_used_seconds":67,"created_at":10,"updated_at":20}}"""), "thread-1")
        assertEquals(Goal("thread-1", "Ship the modal", "paused", 1200, 345, 67, 10, 20), acknowledged)

        val native = readNativeGoal(JSONObject("""{"threadId":"thread-1","goal":{"threadId":"thread-1","objective":"Ship the modal","status":"usageLimited","tokenBudget":null,"tokensUsed":345,"timeUsedSeconds":67,"createdAt":10,"updatedAt":21}}"""))
        assertEquals("usageLimited", native.status)
        assertNull(native.tokenBudget)
        assertNull(readGoalResult(JSONObject("""{"goal":null}"""), "thread-1"))
    }

    @Test fun rejectsMalformedOrCrossThreadGoalState() {
        for (json in listOf(
            """{"goal":{"thread_id":"other","objective":"x","status":"active","token_budget":null,"tokens_used":0,"time_used_seconds":0,"created_at":1,"updated_at":1}}""",
            """{"goal":{"thread_id":"thread-1","objective":"x","status":"mystery","token_budget":null,"tokens_used":0,"time_used_seconds":0,"created_at":1,"updated_at":1}}""",
            """{"goal":{"thread_id":"thread-1","objective":"x","status":"active","token_budget":0,"tokens_used":0,"time_used_seconds":0,"created_at":1,"updated_at":1}}""",
        )) assertThrows(IllegalArgumentException::class.java) { readGoalResult(JSONObject(json), "thread-1") }

        assertThrows(IllegalArgumentException::class.java) {
            readNativeGoal(JSONObject("""{"threadId":"thread-1","goal":{"threadId":"other","objective":"x","status":"active","tokenBudget":null,"tokensUsed":0,"timeUsedSeconds":0,"createdAt":1,"updatedAt":1}}"""))
        }
    }

    @Test fun validatesGoalInputAndOnlySendsAnExplicitPositiveBudget() {
        assertTrue(validGoalObjective(" Ship it "))
        assertFalse(validGoalObjective("   "))
        assertTrue(validGoalBudget(""))
        assertTrue(validGoalBudget("42"))
        for (value in listOf("0", "-1", "1.5", "nope", "9223372036854775808")) assertFalse(value, validGoalBudget(value))

        val withoutBudget = goalSetParams("thread-1", "/work", objective = " Ship it ", budget = "")
        assertEquals("Ship it", withoutBudget.getString("objective"))
        assertFalse(withoutBudget.has("token_budget"))
        assertEquals("/work", withoutBudget.getString("workspace"))

        val withBudget = goalSetParams("thread-1", "/work", objective = "Ship it", budget = "42")
        assertEquals(42L, withBudget.getLong("token_budget"))
        assertEquals("paused", goalSetParams("thread-1", "/work", status = "paused").getString("status"))
        assertThrows(IllegalArgumentException::class.java) { goalSetParams("thread-1", "/work", status = "complete") }
    }

    @Test fun budgetLimitedRecoveryRequiresANewBudgetAboveUsageAndTheOldLimit() {
        val limited = Goal("thread-1", "Ship it", "budgetLimited", 1200, 1200, 10, 1, 2)
        for (value in listOf("", "1199", "1200")) assertFalse(value, validGoalUpdateBudget(limited, value))
        assertTrue(validGoalUpdateBudget(limited, "1201"))
        assertTrue(validGoalUpdateBudget(limited.copy(tokenBudget = null, tokensUsed = 2000), "2001"))
        assertTrue(validGoalUpdateBudget(limited.copy(status = "active"), ""))
    }
}
