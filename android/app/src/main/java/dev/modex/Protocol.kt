package dev.modex

import java.security.MessageDigest
import java.util.Base64
import org.json.JSONObject

internal data class BackendCapabilities(
    val backend: String,
    val sessionStatus: Boolean,
    val planMode: Boolean,
    val permissionProfiles: Boolean,
    val sessionLifecycle: Boolean,
    val projects: Boolean,
    val references: Boolean,
    val takeover: Boolean,
) {
    val label: String get() = when (backend) { "opencode" -> "OpenCode"; "codex" -> "Codex"; else -> "Agent" }
    fun supports(method: String): Boolean = when (method) {
        "threads/status" -> sessionStatus
        "threads/mode", "plans/respond" -> planMode
        "threads/permissions" -> permissionProfiles
        "threads/archive", "threads/unarchive", "threads/delete", "threads/fork" -> sessionLifecycle
        "projects/list", "projects/create" -> projects
        "references/list" -> references
        "threads/takeover" -> takeover
        else -> true
    }

    companion object {
        val LegacyCodex = BackendCapabilities("codex", true, true, true, true, true, true, true)

        fun fromReady(frame: JSONObject): BackendCapabilities {
            if (!frame.has("backend") && !frame.has("capabilities")) return LegacyCodex
            val backend = frame.optString("backend")
            val raw = frame.optJSONArray("capabilities")
            val valid = backend in setOf("codex", "opencode") && raw != null &&
                (0 until raw.length()).all { raw.opt(it) is String }
            val keys = if (valid) (0 until raw!!.length()).map { raw.getString(it) }.toSet() else emptySet()
            return BackendCapabilities(
                backend,
                "session_status" in keys,
                "plan_mode" in keys,
                "permission_profiles" in keys,
                "session_lifecycle" in keys,
                "projects" in keys,
                "references" in keys,
                "takeover" in keys,
            )
        }
    }
}

fun fingerprint(key: ByteArray): String = "SHA256:" + Base64.getEncoder().withoutPadding().encodeToString(MessageDigest.getInstance("SHA-256").digest(key))

fun remoteCommand(binary: String, state: String, cursor: Long, instance: String?): String {
    require((binary.isEmpty() || binary.startsWith('/')) && (state.isEmpty() || state.startsWith('/')))
    require(listOf(binary, state, instance.orEmpty()).none { value -> value.any { it == '\n' || it == '\r' || it == '\u0000' } })
    require(cursor >= 0 && (cursor == 0L || !instance.isNullOrBlank()))
    fun quote(value: String) = "'" + value.replace("'", "'\"'\"'") + "'"
    val executable = if (binary.isEmpty()) "\"\$HOME\"/.local/bin/modex-host" else quote(binary)
    val stateDirectory = if (state.isEmpty()) "\"\$HOME\"/.local/state/modex-phone" else quote(state)
    return "exec $executable --attach --heartbeat --state-dir $stateDirectory --cursor $cursor" + (instance?.let { " --instance ${quote(it)}" } ?: "")
}

class JournalState(instance: String? = null, cursor: Long = 0) {
    init { require(cursor >= 0 && (cursor == 0L || !instance.isNullOrBlank())) }
    var instance: String? = instance
        private set
    var cursor = cursor
        private set
    internal var backendCapabilities = BackendCapabilities.LegacyCodex
        private set
    fun hello(frame: JSONObject) {
        require(frame.optBoolean("ready") && frame.optString("instance").isNotBlank()) { "Supervisor did not acknowledge attach" }
        val incoming = frame.getString("instance")
        require(instance == null || instance == incoming) { "Supervisor identity changed in the attach reply." }
        val checkpoint = if (frame.optBoolean("resynced")) {
            require(instance == null && cursor == 0L) { "Checkpoint requires a fresh journal." }
            val value = frame.get("cursor")
            require(value is Number && value.toString().toLongOrNull()?.let { it > 0 } == true) { "Invalid journal checkpoint" }
            value.toLong()
        } else cursor
        instance = incoming
        cursor = checkpoint
        backendCapabilities = BackendCapabilities.fromReady(frame)
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

internal fun updatePendingDecisions(current: List<JSONObject>, event: String, params: JSONObject): List<JSONObject> = when (event) {
    "decision/reset" -> emptyList()
    "decision/resolved" -> current.filterNot { it.optString("decision_id") == params.optString("decision_id") }
    "decision/required" -> if (current.none { it.optString("decision_id") == params.optString("decision_id") }) current + params else current
    else -> current
}

// Never display raw upstream errors: they can include credential-bearing URLs.
fun turnFailure(params: JSONObject): String? {
    readTurnNotice(params.optJSONObject("notice") ?: params.optJSONObject("turn")?.optJSONObject("notice"))?.let { return it.message }
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
