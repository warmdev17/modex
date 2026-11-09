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

class JournalState(instance: String? = null, cursor: Long = 0) {
    init { require(cursor >= 0 && (cursor == 0L || !instance.isNullOrBlank())) }
    var instance: String? = instance
        private set
    var cursor = cursor
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

// Never display raw upstream errors: they can include credential-bearing URLs.
fun turnFailure(params: JSONObject): String? {
    if (params.optBoolean("willRetry")) return null
    val turn = params.optJSONObject("turn")
    val failure = turn?.optJSONObject("error") ?: params.optJSONObject("error")
    if (failure == null && turn?.optString("status") != "failed") return null
    val details = failure?.optJSONObject("codexErrorInfo") ?: failure?.optJSONObject("codex_error_info")
    val connection = details?.optJSONObject("httpConnectionFailed") ?: details?.optJSONObject("http_connection_failed")
    val status = connection?.optInt("httpStatusCode", 0)?.takeIf { it != 0 } ?: connection?.optInt("http_status_code", 0)
    if (status == 403 || failure?.optString("message")?.contains("403") == true) {
        return "Provider rejected this turn (HTTP 403). Check model access and API key permissions; choose an allowed model for a new chat."
    }
    return "Codex turn failed. Check the model/provider on the host before retrying."
}

fun approvalDetails(context: JSONObject): List<Pair<String, String>> {
    fun readable(value: Any): String = when (value) {
        is JSONObject -> value.keys().asSequence().map { key -> "$key: ${readable(value.get(key))}" }.joinToString("\n")
        is org.json.JSONArray -> (0 until value.length()).joinToString("\n") { "• ${readable(value.get(it))}" }
        else -> value.toString()
    }
    val labels = linkedMapOf("command" to "Command", "cwd" to "Working directory", "reason" to "Why approval is needed",
        "kind" to "Action type", "additionalPermissions" to "Additional permissions", "networkApprovalContext" to "Network access", "grantRoot" to "Requested directory access")
    val keys = (labels.keys + context.keys().asSequence().toList()).distinct()
    return keys.filter { it != "availableDecisions" && context.has(it) && !context.isNull(it) }
        .map { (labels[it] ?: it) to readable(context.get(it)) }
}
