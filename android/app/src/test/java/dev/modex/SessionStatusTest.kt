package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class SessionStatusTest {
    @Test fun exhaustedUsageIsReportedUntilItsReset() {
        val limit = JSONObject("""{"limitId":"codex","primary":{"usedPercent":100,"windowDurationMins":300,"resetsAt":1700000030}}""")
        assertTrue(usageLimitWarning(limit, 1700000000).startsWith("Codex 5-hour usage limit reached. Resets"))
        assertEquals("", usageLimitWarning(limit, 1700000030))
        limit.getJSONObject("primary").put("usedPercent", 99.9)
        assertEquals("", usageLimitWarning(limit, 1700000000))
        limit.put("rateLimitReachedType", "workspace_member_usage_limit_reached")
        assertEquals("Codex workspace usage limit reached.", usageLimitWarning(limit, 1700000000))
    }
    @Test fun contextRemainingUsesCurrentTokensAndKeepsMissingDataUnavailable() {
        assertEquals(88, contextRemaining(SessionStatus(contextTokens = 1200, contextWindow = 10000)))
        assertEquals(0, contextRemaining(SessionStatus(contextTokens = 12000, contextWindow = 10000)))
        assertNull(contextRemaining(SessionStatus(contextTokens = -1, contextWindow = 10000)))
        assertNull(contextRemaining(SessionStatus(contextTokens = 0, contextWindow = null)))
    }

    @Test fun usageWindowsUseReportedDurationAndRemainingCapacity() {
        val weekly = RateLimitWindow(26.0, 10080, 1770000000)
        val fiveHours = RateLimitWindow(68.0, 300, 1770000100)
        val status = SessionStatus(primaryLimit = weekly, secondaryLimit = fiveHours)
        assertEquals(fiveHours, status.usageWindow(300))
        assertEquals(weekly, status.usageWindow(10080))
        assertEquals(32, fiveHours.remainingPercent())
        assertEquals(74, weekly.remainingPercent())
        assertNull(status.copy(primaryLimit = RateLimitWindow(0.0, 60, null)).usageWindow(10080))
        assertNull(fiveHours.copy(usedPercent = Double.NaN).remainingPercent())
        assertNull(fiveHours.copy(usedPercent = null).remainingPercent())
    }

    @Test fun parsesReportedSessionContextAndLimitsWithoutInventingZeros() {
        val result = JSONObject("""{
          "session":{"collaboration_mode":"plan","model":"gpt-6","effort":"high","approval_policy":"on-request","approvals_reviewer":"user","sandbox":"workspaceWrite","thread":{"id":"t1","cwd":"/work/modex","modelProvider":"openai"}},
          "token_usage":{"last":{"totalTokens":1200},"total":{"totalTokens":5400},"modelContextWindow":10000},
          "rate_limits":{"limitId":"codex","limitName":"Codex","planType":"pro","primary":{"usedPercent":25,"windowDurationMins":300,"resetsAt":1770000000}}
        }""")

        val status = readSessionStatus(result)

        assertEquals("plan", status.mode)
        assertEquals("/work/modex", status.workspace)
        assertEquals(1200L, status.contextTokens)
        assertEquals(5400L, status.totalTokens)
        assertEquals(10000L, status.contextWindow)
        assertEquals(25.0, status.primaryLimit?.usedPercent ?: -1.0, 0.01)
        assertNull(status.secondaryLimit)
    }

    @Test fun absentProviderMetricsStayUnavailable() {
        val status = readSessionStatus(JSONObject("""{"session":{"model":"fixture","thread":{"id":"t1","cwd":"/tmp"}},"token_usage":null,"rate_limits":null}"""))

        assertEquals("", status.mode)
        assertNull(status.contextTokens)
        assertNull(status.contextWindow)
        assertNull(status.totalTokens)
        assertNull(status.primaryLimit)
        assertNull(status.secondaryLimit)
    }

    @Test fun modeRequestUsesSelectedSessionWithoutStartingATurn() {
        val params = threadModeParams("thread-1", "/work/modex", "plan", "gpt-6", "high")

        assertEquals("thread-1", params.getString("thread_id"))
        assertEquals("/work/modex", params.getString("workspace"))
        assertEquals("plan", params.getString("mode"))
        assertEquals("gpt-6", params.getString("model"))
        assertEquals("high", params.getString("effort"))
        assertFalse(params.has("text"))
    }

    @Test fun contextSummaryUsesCurrentTurnAndClampsPercentage() {
        assertEquals("Context 12% · 1,200 / 10,000", contextSummary(SessionStatus(contextTokens = 1200, contextWindow = 10000)))
        assertEquals("Context 100% · 12,000 / 10,000", contextSummary(SessionStatus(contextTokens = 12000, contextWindow = 10000)))
        assertEquals("Context unavailable", contextSummary(SessionStatus(contextTokens = 0, contextWindow = null)))
    }

    @Test fun modeControlSeparatesFreshUnknownFromUncertainFailure() {
        assertEquals(ModeControl("Enable Plan", "plan", refresh = false, accented = false, enabled = true), modeControl("", uncertain = false, changing = false))
        assertEquals(ModeControl("Mode unknown · Refresh", "", refresh = true, accented = false, enabled = true), modeControl("plan", uncertain = true, changing = false))
        assertEquals(ModeControl("Plan", "default", refresh = false, accented = true, enabled = true), modeControl("plan", uncertain = false, changing = false))
        assertEquals(ModeControl("Changing mode…", "", refresh = false, accented = false, enabled = false), modeControl("default", uncertain = false, changing = true))
    }
}
