package dev.modex

import org.json.JSONObject

internal val permissionModes = linkedMapOf(
    "host-default" to "Host default",
    "read-only" to "Read-only",
    "workspace-write" to "Workspace write",
    "full-access" to "Full access"
)

internal fun newThreadParams(model: String, mode: String): JSONObject {
    require(mode in permissionModes)
    return JSONObject().apply {
        if (model.isNotBlank()) put("model", model)
        if (mode != "host-default") put("permission_mode", mode)
    }
}

internal fun turnStartParams(thread: String, text: String, model: String): JSONObject =
    JSONObject().put("thread_id", thread).put("text", text).apply {
        require(validModelId(model))
        if (model.isNotBlank()) put("model", model.trim())
    }

internal fun validModelId(value: String): Boolean = value.isEmpty() ||
    (value.isNotBlank() && value.toByteArray(Charsets.UTF_8).size <= 256 && value.none { it == '\r' || it == '\n' || it == '\u0000' })

internal fun sessionSettingsLabel(result: JSONObject): String {
    fun value(key: String) = result.optString(key).ifBlank { "Unknown" }
    val sandbox = when (result.optString("sandbox")) {
        "readOnly" -> "Read-only"
        "workspaceWrite" -> "Workspace write"
        "dangerFullAccess" -> "Full access"
        "externalSandbox" -> "External sandbox"
        else -> "Unknown"
    }
    val provider = result.optJSONObject("thread")?.optString("modelProvider").orEmpty().ifBlank { "Unknown" }
    return "Provider: $provider\nModel: ${value("model")}\nSandbox: $sandbox\nApprovals: ${value("approval_policy")}\nReviewer: ${value("approvals_reviewer")}"
}
