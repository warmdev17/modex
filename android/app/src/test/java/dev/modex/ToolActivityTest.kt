package dev.modex

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ToolActivityTest {
    @Test fun rendersSearchAndUpdatesFileDiffWithoutDuplicatingRows() {
        fun event(item: String) = JSONObject("""{"turnId":"t","item":$item}""")
        val web = updateChat(emptyList(), "item/started", event("""{"type":"webSearch","id":"web","query":""}"""))
        assertEquals("Browsing the web", web.single().command)
        val searched = updateChat(web, "item/completed", event("""{"type":"webSearch","id":"web","query":"compose"}"""))
        assertEquals("Searched the web for compose", searched.single().command)
        val file = updateChat(searched, "item/started", event("""{"type":"fileChange","id":"edit","status":"inProgress","changes":[]}"""))
        val patched = updateChat(file, "item/fileChange/patchUpdated", JSONObject("""{"turnId":"t","itemId":"edit","changes":[{"path":"src/Main.kt","diff":"@@ -1 +1 @@\n-old\n+new"}]}"""))
        assertEquals(2, patched.size)
        val patch = patched.last()
        assertEquals("src/Main.kt", patch.command)
        assertTrue(patch.text.contains("+new"))
        assertEquals(patched, readChat(JSONArray(patched.map { it.toJson() })))
    }

    @Test fun historyUsesStructuredActionsAndDiffs() {
        val entries = readChat(JSONArray("""[
            {"id":"c","role":"command","command":"rg test","activity_item":{"commandActions":[{"type":"search","query":"test","path":"src"}]}},
            {"id":"f","role":"file_change","activity_item":{"changes":[{"path":"a.kt","diff":"-old\n+new"}]}}
        ]"""))
        assertEquals("Search test in src", entries.first().activity)
        assertEquals("a.kt", entries.last().command)
        assertEquals("-old\n+new", entries.last().text)
        assertEquals(2, styledFileDiff(entries.last().text).spanStyles.size)
    }
}
