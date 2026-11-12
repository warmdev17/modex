package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ProtocolTest {
    @Test fun takeoverOnlyTargetsRejectedResumes() {
        assertEquals("target", takeoverTarget("threads/resume", "Codex request failed: active_writer", "target"))
        assertEquals("", takeoverTarget("turn/start", "active_writer", "target"))
        assertEquals("", takeoverTarget("threads/resume", "request_rejected", "target"))
        assertEquals("", takeoverTarget("threads/resume", "active_writer", ""))
        assertTrue(explainHostRejection("threads/takeover", "takeover_unmanaged").contains("launcher"))
        assertTrue(explainHostRejection("threads/takeover", "takeover_uncertain").contains("Do not retry"))
    }

    @Test
    fun activeWriterRejectionExplainsDesktopOwnership() {
        assertEquals(
            "This session is active in Codex Desktop or another client. Close it there, then reconnect and retry.",
            explainHostRejection("threads/resume", "Codex request failed: active_writer")
        )
    }

    @Test
    fun unknownRejectionKeepsSafeCategoryForDiagnostics() {
        assertEquals(
            "Host rejected threads/list (request_rejected). Review session state before retrying.",
            explainHostRejection("threads/list", "request_rejected")
        )
    }

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
    @Test fun reportsFailedTurnsWithoutLeakingRawProviderDetails() {
        val failed = JSONObject("""{"turn":{"status":"failed","error":{"message":"403 Forbidden: secret URL and key","codexErrorInfo":{"httpConnectionFailed":{"httpStatusCode":403}}}}}""")
        assertEquals("Provider rejected this turn (HTTP 403). Check model access and API key permissions; choose an allowed model for a new chat.", turnFailure(failed))
        assertEquals("Codex turn failed. Check the model/provider on the host before retrying.", turnFailure(JSONObject("""{"turn":{"status":"failed"}}""")))
        assertNull(turnFailure(JSONObject("""{"turn":{"status":"completed"}}""")))
        assertNull(turnFailure(JSONObject("""{"error":{"message":"temporary"},"willRetry":true}""")))
    }
    @Test fun approvalDetailsPreservePermissionContextWithoutJSONEnvelopes() {
        val context = JSONObject("""{"command":"printf MODEX_APPROVAL_OK","cwd":"/work/space","reason":"Harmless test","kind":"exec","additionalPermissions":{"fileSystem":{"write":["/tmp/example"]}},"networkApprovalContext":{"host":"example.com","port":443},"optional":null}""")
        val fields = approvalDetails(context).toMap()
        assertEquals("printf MODEX_APPROVAL_OK", fields["Command"])
        assertEquals("/work/space", fields["Working directory"])
        assertEquals("Harmless test", fields["Why approval is needed"])
        assertTrue(fields.getValue("Additional permissions").contains("/tmp/example"))
        assertTrue(fields.getValue("Network access").contains("example.com"))
        assertFalse(fields.containsKey("optional"))
        assertFalse(fields.getValue("Additional permissions").contains("{"))
        assertEquals(emptyList<Pair<String, String>>(), approvalDetails(JSONObject()))
    }
    @Test fun restoresJournalWithoutReplayingHandledFrames() {
        val journal = JournalState("saved-instance", 8)
        journal.hello(JSONObject("""{"ready":true,"instance":"saved-instance"}"""))
        assertNull(journal.frame(JSONObject("""{"seq":8,"message":{}}""")))
        journal.frame(JSONObject("""{"seq":9,"message":{}}"""))
        assertEquals(9L, journal.cursor)
        assertThrows(IllegalArgumentException::class.java) { JournalState(null, 8) }
    }
    @Test fun computesPinnedSHA256Fingerprint() {
        assertEquals("SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU", fingerprint(byteArrayOf()))
    }
}
