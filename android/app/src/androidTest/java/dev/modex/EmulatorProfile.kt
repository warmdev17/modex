package dev.modex

import android.app.Instrumentation
import android.content.pm.ApplicationInfo
import android.os.Build
import android.os.Bundle
import android.util.Base64
import com.jcraft.jsch.JSch
import java.io.File
import org.json.JSONObject

/** Explicit development setup through the test APK only; no production import endpoint. */
fun Instrumentation.importEmulatorProfile() {
    val result = Bundle()
    val file = File(targetContext.filesDir, "emulator-profile.json")
    var key: ByteArray? = null
    var status = 0
    val identities = JSch()
    try {
        check(targetContext.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0)
        check(Build.HARDWARE in setOf("ranchu", "goldfish"))
        val saved = SavedHost(targetContext)
        if (saved.load() != null) {
            result.putString("result", "PASS: emulator profile already saved; existing history and settings kept")
        } else {
            check(file.length() in 1..(384L * 1024))
            val data = JSONObject(file.readText())
            val connection = data.getJSONObject("connection")
            check(connection.getString("host").isNotBlank() && connection.getString("user").isNotBlank())
            check(connection.getInt("port") in 1..65535)
            check(connection.getString("pin").matches(Regex("SHA256:[A-Za-z0-9+/]{43}")))
            remoteCommand(connection.getString("binary"), connection.getString("state"), 0, null)
            val bytes = Base64.decode(data.getString("key"), Base64.NO_WRAP)
            key = bytes
            check(bytes.size in 1..(256 * 1024))
            identities.addIdentity("emulator-bootstrap", bytes, null, null)
            check(identities.identityRepository.identities.isNotEmpty())
            check(identities.identityRepository.identities.none { it.isEncrypted })
            saved.save(data)
            check(saved.load()?.getJSONObject("connection")?.getString("host") == connection.getString("host"))
            result.putString("result", "PASS: emulator profile imported into encrypted device storage")
        }
        status = -1
    } catch (_: Throwable) {
        result.putString("result", "FAIL: emulator profile import; use an unencrypted private key and valid connection")
    } finally {
        try { identities.removeAllIdentity() } finally {
            key?.fill(0)
            file.delete()
        }
    }
    finish(status, result)
}
