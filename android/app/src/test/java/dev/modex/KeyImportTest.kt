package dev.modex

import java.io.ByteArrayInputStream
import java.io.IOException
import java.io.InputStream
import org.junit.Assert.*
import org.junit.Test

class KeyImportTest {
    @Test fun readsBytesWithoutChangingThem() {
        val bytes = "private key fixture\n".toByteArray()
        assertArrayEquals(bytes, readImportedKey(ByteArrayInputStream(bytes)))
    }

    @Test fun acceptsSizeLimit() {
        assertEquals(256 * 1024, readImportedKey(ByteArrayInputStream(ByteArray(256 * 1024))).size)
    }

    @Test fun distinguishesEmptyAndOversizedFiles() {
        for ((size, message) in listOf(0 to "The selected file is empty.",
            256 * 1024 + 1 to "The selected file exceeds 256 KiB.")) {
            try {
                readImportedKey(ByteArrayInputStream(ByteArray(size)))
                fail("Expected rejection")
            } catch (error: IllegalArgumentException) {
                assertEquals(message, error.message)
            }
        }
    }

    @Test fun preservesReadFailureForCaller() {
        val failure = IOException("fixture")
        try {
            readImportedKey(object : InputStream() { override fun read(): Int = throw failure })
            fail("Expected read failure")
        } catch (error: IOException) {
            assertSame(failure, error)
        }
    }
}
