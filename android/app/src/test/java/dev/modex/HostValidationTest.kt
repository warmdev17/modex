package dev.modex

import com.jcraft.jsch.JSch
import com.jcraft.jsch.KeyPair
import java.io.ByteArrayOutputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HostValidationTest {
    private val pin = "SHA256:" + "A".repeat(43)

    @Test fun `valid host form preserves hostname and accepts IPv6`() {
        assertTrue(validateHostForm("server.example", "22", "dev", pin, "", "", true).isEmpty())
        assertTrue(validateHostForm("2001:db8::7", "65535", "dev", pin, "/opt/modex-host", "/var/lib/modex", true).isEmpty())
        assertTrue(validateHostForm(" server.example ", " 1 ", " dev ", " $pin ", " /opt/modex-host ", " /var/lib/modex ", true).isEmpty())
    }

    @Test fun `host form reports each invalid control`() {
        val errors = validateHostForm("bad host", "22oops", "", "SHA256:not-a-fingerprint", "relative/tool", "relative/state", false)

        assertEquals("Enter a hostname or IP address without spaces.", errors.host)
        assertEquals("Enter a port from 1 to 65535.", errors.port)
        assertEquals("Enter the SSH username.", errors.user)
        assertEquals("Paste the verified SHA256 fingerprint.", errors.pin)
        assertEquals("Use an absolute path or leave blank.", errors.binary)
        assertEquals("Use an absolute path or leave blank.", errors.state)
        assertEquals("Import an unencrypted SSH private key.", errors.key)
    }

    @Test fun `host form rejects port boundaries and unsafe host text`() {
        assertEquals("Enter a port from 1 to 65535.", validateHostForm("server.example", "0", "dev", pin, "", "", true).port)
        assertEquals("Enter a port from 1 to 65535.", validateHostForm("server.example", "65536", "dev", pin, "", "", true).port)
        assertEquals("Enter a hostname or IP address without spaces.", validateHostForm("bad\nhost", "22", "dev", pin, "", "", true).host)
    }

    @Test fun `rejected private key is zeroed before replacement`() {
        val rejected = "not a private key".toByteArray()

        assertEquals("Import a supported unencrypted SSH private key.", validateImportedKey(rejected))
        assertTrue(rejected.all { it == 0.toByte() })
    }

    @Test fun `valid unencrypted private key remains available for import`() {
        val pair = KeyPair.genKeyPair(JSch(), KeyPair.RSA, 1024)
        val bytes = ByteArrayOutputStream().also(pair::writePrivateKey).toByteArray()
        pair.dispose()

        assertNull(validateImportedKey(bytes))
        assertFalse(bytes.all { it == 0.toByte() })
        bytes.fill(0)
    }
}
