package dev.modex

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ChatEntryTest {
    private fun event(raw: String) = JSONObject(raw)
    @Test fun separatesAgentMessagesAndCompletesWithoutDuplicating() {
        var items = emptyList<ChatEntry>()
        items = updateChat(items, "item/agentMessage/delta", event("""{"itemId":"a","delta":"First"}"""))
        items = updateChat(items, "item/agentMessage/delta", event("""{"itemId":"b","delta":"Second"}"""))
        items = updateChat(items, "item/completed", event("""{"item":{"id":"a","type":"agentMessage","text":"First final"}}"""))
        assertEquals(listOf("First final", "Second"), items.map { it.text })
    }
    @Test fun commandLifecycleKeepsOrderAndExitCode() {
        var items = updateChat(emptyList(), "item/started", event("""{"item":{"id":"c","type":"commandExecution","command":"go test ./...","cwd":"/work","status":"inProgress"}}"""))
        assertEquals("inProgress", items.single().status)
        items = updateChat(items, "item/commandExecution/outputDelta", event("""{"itemId":"c","delta":"PASS"}"""))
        assertEquals("PASS", items.single().text)
        items = updateChat(items, "item/completed", event("""{"item":{"id":"c","type":"commandExecution","command":"go test ./...","status":"completed","aggregatedOutput":"PASS\n","exitCode":0}}"""))
        assertEquals(1, items.size)
        assertEquals(0, items.single().exitCode)
        assertEquals("completed", items.single().status)
        assertEquals("PASS\n", items.single().text)
    }
    @Test fun legacyHistoryAndNewEntriesRoundTrip() {
        val old = readChat(JSONArray("""[{"role":"user","text":"hello"},{"role":"assistant","text":"hi"}]"""))
        assertEquals(listOf("You", "Codex"), old.map { it.role })
        val command = ChatEntry("c", "command", "output", "ls", "completed", 1, "/work")
        assertEquals(command, readChat(JSONArray().put(command.toJson())).single())
    }
    @Test fun outputIsBoundedAndMarked() {
        val command = ChatEntry("c", "command", "", "cat huge")
        val items = updateChat(listOf(command), "item/commandExecution/outputDelta", JSONObject().put("itemId", "c").put("delta", "界".repeat(100000)))
        assertTrue(items.single().text.toByteArray().size < 66000)
        assertTrue(items.single().text.contains("truncated"))
    }
    @Test fun unknownItemsDoNotBecomeChatMessages() {
        assertTrue(updateChat(emptyList(), "item/started", event("""{"item":{"id":"x","type":"unknownFutureItem"}}""")).isEmpty())
    }
}
