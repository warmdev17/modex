package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ProtocolTest {
    @Test fun readyMetadataDefaultsLegacyFramesAndParsesExplicitCapabilities() {
        val legacy = BackendCapabilities.fromReady(JSONObject("""{"ready":true,"instance":"legacy"}"""))
        assertEquals("Codex", legacy.label)
        assertTrue(legacy.sessionStatus && legacy.planMode && legacy.permissionProfiles)
        assertTrue(legacy.sessionLifecycle && legacy.projects && legacy.references && legacy.takeover)

        val openCode = BackendCapabilities.fromReady(JSONObject("""{"ready":true,"instance":"open","backend":"opencode","capabilities":[]}"""))
        assertEquals("OpenCode", openCode.label)
        assertFalse(openCode.sessionStatus || openCode.planMode || openCode.permissionProfiles)
        assertFalse(openCode.sessionLifecycle || openCode.projects || openCode.references || openCode.takeover)

        val codex = BackendCapabilities.fromReady(JSONObject("""{
          "ready":true,"instance":"codex","backend":"codex",
          "capabilities":["session_status","plan_mode","permission_profiles","session_lifecycle","projects","references","takeover"]
        }"""))
        assertEquals(legacy, codex)
    }

    @Test fun explicitBackendDisablesMalformedOrMissingOptionalCapabilitiesAndReplacementDoesNotLeak() {
        val journal = JournalState()
        journal.hello(JSONObject("""{"ready":true,"instance":"one","backend":"codex","capabilities":["references","takeover"]}"""))
        var current = journal.backendCapabilities
        assertTrue(current.references && current.takeover)
        assertFalse(current.sessionStatus || current.projects)

        journal.hello(JSONObject("""{"ready":true,"instance":"one","backend":"opencode","capabilities":["references",7]}"""))
        current = journal.backendCapabilities
        assertEquals("OpenCode", current.label)
        assertFalse(current.references || current.takeover || current.projects)

        journal.hello(JSONObject("""{"ready":true,"instance":"one","backend":"opencode"}"""))
        current = journal.backendCapabilities
        assertFalse(current.sessionStatus || current.planMode || current.permissionProfiles || current.sessionLifecycle || current.projects || current.references || current.takeover)
    }

    @Test fun openCodeCapabilitiesKeepCoreChatAndBlockEveryOptionalRequest() {
        val capabilities = BackendCapabilities.fromReady(JSONObject("""{"ready":true,"instance":"open","backend":"opencode","capabilities":[]}"""))
        for (method in listOf("threads/list", "threads/read", "threads/start", "turn/start", "turn/steer", "turn/interrupt", "decision/respond", "models/list", "status")) {
            assertTrue(method, capabilities.supports(method))
        }
        for (method in listOf("threads/status", "threads/mode", "threads/permissions", "threads/archive", "threads/unarchive", "threads/delete", "threads/fork", "threads/takeover", "projects/list", "projects/create", "references/list", "plans/respond")) {
            assertFalse(method, capabilities.supports(method))
        }
    }

    @Test fun decisionResetAndReplayReconcilePendingWithoutReanswering() {
        val stale = JSONObject("""{"decision_id":"stale"}""")
        var decisions = updatePendingDecisions(listOf(stale), "decision/reset", JSONObject())
        assertTrue(decisions.isEmpty())
        val approval = JSONObject("""{"decision_id":"approval","context":{"command":"echo fixture","availableDecisions":["accept","decline"]}}""")
        val question = JSONObject("""{"decision_id":"question","context":{"questions":[{"id":"choice","question":"Choose","options":[{"label":"Yes"}]}]}}""")
        decisions = updatePendingDecisions(decisions, "decision/required", approval)
        decisions = updatePendingDecisions(decisions, "decision/required", question)
        decisions = updatePendingDecisions(decisions, "decision/required", approval)
        assertEquals(listOf(approval, question), decisions)
        decisions = updatePendingDecisions(decisions, "decision/resolved", JSONObject("""{"decision_id":"approval"}"""))
        assertEquals(listOf(question), decisions)
        assertSame(decisions, updatePendingDecisions(decisions, "unrelated/event", JSONObject()))
        assertTrue(updatePendingDecisions(decisions, "decision/reset", JSONObject()).isEmpty())
        assertEquals("echo fixture", approval.getJSONObject("context").getString("command"))
    }
    @Test fun freshJournalAcceptsAnExplicitCheckpointAndThenRequiresContiguousFrames() {
        val journal = JournalState()
        journal.hello(JSONObject("""{"ready":true,"instance":"same","resynced":true,"cursor":85}"""))
        assertEquals(85L, journal.cursor)
        assertEquals("same", journal.instance)
        assertNotNull(journal.frame(JSONObject("""{"seq":86,"message":{"event":"decision/snapshot","params":{"data":[]}}}""")))
        assertThrows(IllegalArgumentException::class.java) { journal.frame(JSONObject("""{"seq":88,"message":{}}""")) }
    }

    @Test fun checkpointsCannotSkipSavedEventsOrAcceptInvalidCursors() {
        val saved = JournalState("same", 8)
        assertThrows(IllegalArgumentException::class.java) {
            saved.hello(JSONObject("""{"ready":true,"instance":"same","resynced":true,"cursor":85}"""))
        }
        assertEquals(8L, saved.cursor)
        for (value in listOf("-1", "0", "8.5", "\"85\"", "null", "9223372036854775808")) {
            val fresh = JournalState()
            assertThrows(IllegalArgumentException::class.java) {
                fresh.hello(JSONObject("""{"ready":true,"instance":"same","resynced":true,"cursor":$value}"""))
            }
            assertNull(fresh.instance)
            assertEquals(0L, fresh.cursor)
        }
    }
    @Test fun readsLargeHistoryAndFollowingFrameWithoutLosingBoundaries() {
        val body = "x".repeat(5 * 1024 * 1024)
        val input = ("{\"text\":\"" + body + "\"}\n{\"ready\":true}\n").byteInputStream().buffered()
        assertEquals(body, readFrame(input)!!.getString("text"))
        assertTrue(readFrame(input)!!.getBoolean("ready"))
        assertNull(readFrame(input))
    }

    @Test fun rejectsOversizedAndTruncatedFrames() {
        val huge = ("x".repeat(32 * 1024 * 1024 + 1)).byteInputStream().buffered()
        assertThrows(IllegalArgumentException::class.java) { readFrame(huge) }
        assertThrows(IllegalArgumentException::class.java) { readFrame("{\"text\":\"partial".byteInputStream()) }
        assertTrue(explainHostRejection("threads/read", "response_too_large").contains("host is still available"))
    }

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
        assertEquals("exec '/opt/host'\"'\"'s app' --attach --heartbeat --state-dir '/state $(id)' --cursor 0", remoteCommand("/opt/host's app", "/state $(id)", 0, null))
        assertEquals("exec \"\$HOME\"/.local/bin/modex-host --attach --heartbeat --state-dir \"\$HOME\"/.local/state/modex-phone --cursor 0", remoteCommand("", "", 0, null))
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
