package dev.modex

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.AtomicFile
import android.system.Os
import java.io.File
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import org.json.JSONObject

/** A named device-local encrypted store; excluded from cloud backup and device transfer. */
class SavedHost(context: Context, name: String = "host") {
    private val file = AtomicFile(File(context.noBackupFilesDir, "$name.enc"))
    private val pending = File(context.noBackupFilesDir, "$name.enc.pending")
    private val legacyBackup = File(context.noBackupFilesDir, "$name.enc.bak")
    private var generation = 0L
    @Synchronized fun revision() = generation
    private val alias = "modex.saved-$name.v1"
    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(alias, null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    @Synchronized fun save(value: JSONObject, revision: Long = generation) {
        if (revision != generation) return
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
        val plain = value.toString().toByteArray(Charsets.UTF_8)
        try {
            val bytes = cipher.iv + cipher.doFinal(plain)
            if (legacyBackup.exists()) {
                file.openRead().close()
                check(!legacyBackup.exists()) { "Could not recover the encrypted store" }
            }
            pending.outputStream().use {
                it.write(bytes)
                it.fd.sync()
            }
            // AtomicFile.finishWrite logs commit failures instead of reporting them to the caller.
            Os.rename(pending.path, file.baseFile.path)
        } finally {
            plain.fill(0)
            pending.delete()
        }
    }
    @Synchronized fun load(): JSONObject? {
        if (!file.baseFile.exists() && !legacyBackup.exists()) return null
        val bytes = file.readFully()
        require(bytes.size > 28)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, bytes.copyOfRange(0, 12))) }
        val plain = cipher.doFinal(bytes.copyOfRange(12, bytes.size))
        return try { JSONObject(String(plain, Charsets.UTF_8)) } finally { plain.fill(0) }
    }
    @Synchronized fun clear() {
        generation++
        file.delete()
        pending.delete()
        KeyStore.getInstance("AndroidKeyStore").apply { load(null); deleteEntry(alias) }
    }
}

internal fun Connection.toJson(): JSONObject = JSONObject().put("host", host).put("port", port)
    .put("user", user).put("pin", pin).put("binary", binary).put("state", state)
