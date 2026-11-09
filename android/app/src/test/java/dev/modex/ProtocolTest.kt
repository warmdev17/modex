package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ProtocolTest {
    @Test fun quotesRemotePaths() {
        assertEquals("exec '/opt/host'\"'\"'s app' --attach --state-dir '/state $(id)' --cursor 0", remoteCommand("/opt/host's app", "/state $(id)", 0, null))
    }
    @Test fun rejectsInvalidPathAndCursor() {
        for (path in listOf("relative", "/host\ncmd")) {
            assertThrows(IllegalArgumentException::class.java) { remoteCommand(path, "/state", 0, null) }
        }
        assertThrows(IllegalArgumentException::class.java) { remoteCommand("/host", "/state", 1, null) }
    }
    @Test fun journalChecksGapsDuplicatesAndInstances() {
        val journal = JournalState()
        journal.hello(JSONObject("""{"ready":true,"instance":"one"}"""))
        assertNotNull(journal.frame(JSONObject("""{"seq":1,"message":{"id":1,"result":{}}}""")))
        assertNull(journal.frame(JSONObject("""{"seq":1,"message":{}}""")))
        assertThrows(IllegalArgumentException::class.java) { journal.frame(JSONObject("""{"seq":3,"message":{}}""")) }
        assertEquals(1L, journal.cursor)
        assertThrows(IllegalArgumentException::class.java) { journal.hello(JSONObject("""{"ready":true,"instance":"two"}""")) }
    }
    @Test fun cursorAdvancesOnlyAfterSuccessfulApply() {
        val journal = JournalState()
        val frame = JSONObject("""{"seq":1,"message":{}}""")
        assertThrows(IllegalStateException::class.java) { journal.frame(frame) { error("Reducer failed") } }
        assertEquals(0L, journal.cursor)
        journal.frame(frame) { assertEquals(0L, journal.cursor) }
        assertEquals(1L, journal.cursor)
    }
    @Test fun computesPinnedSHA256Fingerprint() {
        assertEquals("SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU", fingerprint(byteArrayOf()))
    }
}
