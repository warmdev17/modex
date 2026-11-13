package dev.modex

import org.junit.Assert.*
import org.junit.Test

class ThinkingTimelineTest {
    @Test fun oneThinkingRowPerTurnKeepsCommandsAndLatestActualSummary() {
        val entries = listOf(
            ChatEntry("u", "You", "Find it", turnId = "t"),
            ChatEntry("r1", "reasoning", "**Searching files**\n\nLooking through source", turnId = "t"),
            ChatEntry("c", "command", "found", command = "rg target", turnId = "t"),
            ChatEntry("r2", "reasoning", "**Checking results**\n\nReviewing matches", turnId = "t"),
            ChatEntry("r3", "reasoning", "", turnId = "t"),
            ChatEntry("a", "Codex", "Done", turnId = "t")
        )
        val timeline = thinkingTimeline(entries)
        assertEquals(listOf("You", "reasoning", "command", "Codex"), timeline.map { it.role })
        val thinking = timeline.single { it.role == "reasoning" }
        assertEquals("r1", thinking.id)
        assertTrue(thinking.text.contains("Searching files"))
        assertTrue(thinking.text.contains("Checking results"))
        assertEquals("Checking results", reasoningLabel(thinking.summaryParts.last()))
        assertEquals(6, entries.size)
    }

    @Test fun separatesPromptsIncludingOlderSnapshotsWithoutTurnIds() {
        val entries = listOf(
            ChatEntry(role = "You", text = "one"),
            ChatEntry(role = "reasoning", text = "Search"),
            ChatEntry(role = "reasoning", text = "Read"),
            ChatEntry(role = "You", text = "two"),
            ChatEntry(role = "reasoning", text = "Inspect"),
            ChatEntry(role = "reasoning", text = "Verify")
        )
        assertEquals(listOf("Read", "Verify"), thinkingTimeline(entries).filter { it.role == "reasoning" }.map { reasoningLabel(it.summaryParts.last()) })
    }

    @Test fun mixedLegacyAndLiveTurnIdsStillProduceOneRowForOnePrompt() {
        val entries = listOf(ChatEntry(role = "You", text = "one"), ChatEntry("r1", "reasoning", "Search"), ChatEntry("r2", "reasoning", "Read", turnId = "known"))
        assertEquals(1, thinkingTimeline(entries).count { it.role == "reasoning" })
    }

    @Test fun emptyReasoningDoesNotProducePlaceholderRows() {
        assertTrue(thinkingTimeline(listOf(ChatEntry("r", "reasoning", " \n"))).isEmpty())
        assertEquals("Đang tìm kiếm thông tin", reasoningLabel("**Đang tìm kiếm thông tin**\n\nChecking sources"))
        assertEquals("Checking results", reasoningLabel("Checking results\nMore detail"))
    }
}
