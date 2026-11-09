package dev.modex

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Test

class SessionLabelTest {
    @Test fun actualNameWinsOverFirstMessage() {
        assertEquals("Modex", sessionLabel(JSONObject("""{"name":"Modex","preview":"first question","id":"123456789"}""")))
    }
    @Test fun unnamedUsesPreviewThenShortId() {
        assertEquals("first question", sessionLabel(JSONObject("""{"preview":"first question"}""")))
        assertEquals("Untitled session · 12345678", sessionLabel(JSONObject("""{"id":"123456789"}""")))
    }
}
