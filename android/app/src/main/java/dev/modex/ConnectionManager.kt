package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import java.io.File
import java.io.IOException
import android.util.Base64
import org.json.JSONObject
import java.util.UUID

/** Retains independent chat controllers while the user moves between hosts and Home. */
internal class ConnectionManager(private val app: ModexApplication) {
    private val store = SavedHost(app, "connections")
    var profiles by mutableStateOf(emptyList<HostProfile>()); private set
    var reconnectOnOpen by mutableStateOf(true); private set
    var error by mutableStateOf(""); private set
    private var catalogLocked = false
    init {
        try {
            val data = store.load()
            profiles = connectionProfiles(data, File(app.noBackupFilesDir, "host.enc").exists())
            reconnectOnOpen = data?.optBoolean("reconnect_on_open", true) ?: true
            if (data == null && profiles.isNotEmpty()) save(profiles)
        } catch (_: Exception) {
            catalogLocked = true
            error = "Saved hosts could not be unlocked. Existing credentials have not been changed."
        }
    }
    private fun save(next: List<HostProfile>, reconnect: Boolean = reconnectOnOpen): Boolean {
        return try {
            if (catalogLocked) return false
            store.save(profilesJson(next).put("reconnect_on_open", reconnect))
            profiles = next; reconnectOnOpen = reconnect; error = ""
            true
        } catch (_: Exception) { error = "Could not save hosts securely. Existing credentials have not been removed."; false }
    }
    fun controller(id: String): ModexViewModel {
        require(profiles.any { it.id == id })
        return ViewModelProvider(app, object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T = ModexViewModel(app, id) as T
        })["remote:$id", ModexViewModel::class.java]
    }
    fun add(name: String, connection: Connection, key: ByteArray): Result<String> = runCatching {
        require(connection.isValid() && connection.host.length <= 253 &&
            connection.host.none { it.isWhitespace() || it.isISOControl() }) { INVALID_CONNECTION }
        val cleaned = name.trim()
        require(cleaned.length <= 80 && cleaned.none { it.isISOControl() }) { "Enter a valid display name." }
        require(isUnencryptedSshKey(key)) { "Import an unencrypted SSH private key." }
        check(!catalogLocked) { error }
        val id = UUID.randomUUID().toString()
        val credentials = SavedHost(app, id)
        try {
            credentials.save(JSONObject().put("connection", connection.toJson())
                .put("key", Base64.encodeToString(key, Base64.NO_WRAP)))
            check(save(profiles + HostProfile(id, cleaned.ifBlank { connection.host })))
        } catch (failure: Exception) {
            runCatching { credentials.clear() }
            throw IOException("Could not save this host securely. Try again.", failure)
        }
        id
    }
    fun rename(id: String, name: String) {
        val cleaned = name.trim()
        if (cleaned.isNotBlank() && cleaned.length <= 80 && cleaned.none { it.isISOControl() })
            save(profiles.map { if (it.id == id) it.copy(name = cleaned) else it })
    }
    fun remove(id: String) {
        // Persist the index first: a failed catalog write must not destroy credentials.
        if (save(profiles.filterNot { it.id == id })) controllerForRemoval(id).forget()
    }
    private fun controllerForRemoval(id: String) = app.sessions[id] ?: ModexViewModel(app, id)
    fun reconnectOnOpen(value: Boolean) { save(profiles, value) }
    fun foreground() { profiles.forEach { controller(it.id).let { vm -> if (vm.connected || reconnectOnOpen) vm.foreground() } } }
}
