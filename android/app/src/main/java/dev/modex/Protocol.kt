package dev.modex

import java.security.MessageDigest
import java.util.Base64
import org.json.JSONObject

fun fingerprint(key: ByteArray): String = "SHA256:" + Base64.getEncoder().withoutPadding().encodeToString(MessageDigest.getInstance("SHA-256").digest(key))

fun remoteCommand(binary: String, state: String, cursor: Long, instance: String?): String {
    require(binary.startsWith('/') && state.startsWith('/'))
    require(listOf(binary, state, instance.orEmpty()).none { value -> value.any { it == '\n' || it == '\r' || it == '\u0000' } })
    require(cursor >= 0 && (cursor == 0L || !instance.isNullOrBlank()))
    fun quote(value: String) = "'" + value.replace("'", "'\"'\"'") + "'"
    return "exec ${quote(binary)} --attach --state-dir ${quote(state)} --cursor $cursor" + (instance?.let { " --instance ${quote(it)}" } ?: "")
}

class JournalState {
    var instance: String? = null
        private set
    var cursor = 0L
        private set
    fun hello(frame: JSONObject) {
        require(frame.optBoolean("ready") && frame.optString("instance").isNotBlank()) { "Supervisor did not acknowledge attach" }
        val incoming = frame.getString("instance")
        require(instance == null || instance == incoming) { "Supervisor restarted. Reset connection to load a new journal." }
        instance = incoming
    }
    fun frame(frame: JSONObject, apply: (JSONObject) -> Unit = {}): JSONObject? {
        require(!frame.has("error")) { "Replay unavailable. Reset connection after reviewing session state." }
        val next = frame.getLong("seq")
        if (next <= cursor) return null
        require(next == cursor + 1) { "Journal gap. Do not resend the previous prompt." }
        val message = frame.getJSONObject("message")
        apply(message)
        cursor = next
        return message
    }
}
