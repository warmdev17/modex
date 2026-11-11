package dev.modex

import org.junit.Assert.*
import org.junit.Test

class ConnectionFailureTest {
    @Test fun distinguishesAuthFromNetworkWithoutLeakingDetails() {
        assertTrue(connectionFailure("ssh", Exception("Auth fail for private-user")).message!!.contains("authentication"))
        val message = connectionFailure("ssh", Exception("connect failed secret-host")).message!!
        assertTrue(message.contains("IP"))
        assertFalse(message.contains("secret-host"))
    }
    @Test fun identifiesRemoteFailuresWithoutShowingStderr() {
        assertTrue(connectionFailure("attach", Exception(), "modex-host: another client is attached\nsecret").message!!.contains("another client"))
        assertTrue(connectionFailure("attach", Exception(), "modex-host: supervisor instance changed; do not reuse cursor").message!!.contains("Reset session cursor"))
        assertTrue(connectionFailure("attach", Exception(), "sh: /tmp/secret: not found").message!!.contains("binary"))
        assertFalse(connectionFailure("attach", Exception(), "unrecognized secret").message!!.contains("secret"))
    }
    @Test fun identifiesJournalGapAndFingerprint() {
        assertTrue(connectionFailure("stream", Exception("Journal gap. Do not resend the previous prompt.")).message!!.contains("Reset session cursor"))
        assertTrue(connectionFailure("fingerprint", Exception("private detail")).message!!.contains("fingerprint"))
    }
    @Test fun doesNotMisclassifyCallbackErrorsAsNetworkFailure() {
        assertTrue(connectionFailure("stream", Exception("Auth fail callback")).message!!.contains("invalid frame"))
        assertTrue(connectionFailure("attach", Exception(), "modex-host: supervisor is unavailable").message!!.contains("supervisor is unavailable"))
    }
    @Test fun detectsExpiredReplayFromRemoteHost() {
        for (detail in listOf("replay gap: cursor older than retained events", "cursor ahead of supervisor journal")) {
            assertTrue(connectionFailure("attach", Exception(), detail).message!!.contains("Reset session cursor"))
        }
    }
    @Test fun missingBinaryCommandWithCursorFlagIsNotReplayGap() {
        val stderr = "fish: Unknown command: /tmp/missing\nexec /tmp/missing --attach --cursor 0 --instance private-value"
        val message = connectionFailure("attach", Exception(), stderr).message!!
        assertTrue(message.contains("binary") || message.contains("attach failed"))
        assertFalse(message.contains("Reset session cursor"))
        assertFalse(message.contains("private-value"))
    }
}
