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

internal fun turnStartParams(thread: String, text: String, model: String, effort: String = "", attachments: List<Attachment> = emptyList()): JSONObject =
    JSONObject().put("thread_id", thread).put("text", text).apply {
        require(validModelId(model) && validEffort(effort))
        if (attachments.isNotEmpty()) put("attachments", org.json.JSONArray().apply { attachments.forEach { put(it.toJson()) } })
        if (model.isNotBlank()) put("model", model.trim())
        if (effort.isNotBlank()) put("effort", effort)
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

internal fun validEffort(value: String): Boolean = value.isEmpty() ||
    (value.isNotBlank() && value.toByteArray(Charsets.UTF_8).size <= 64 && value.none { it == '\r' || it == '\n' || it == '\u0000' })

internal data class ModelOption(val model: String, val name: String, val defaultEffort: String, val efforts: List<Pair<String, String>>)

internal fun readModels(result: JSONObject): List<ModelOption> {
    val data = result.optJSONArray("data") ?: return emptyList()
    return (0 until data.length()).mapNotNull { i ->
        val entry = data.optJSONObject(i) ?: return@mapNotNull null
        val model = entry.optString("model")
        if (model.isBlank() || !validModelId(model)) return@mapNotNull null
        val raw = entry.optJSONArray("supportedReasoningEfforts")
        val efforts = (0 until (raw?.length() ?: 0)).mapNotNull effort@ { j ->
            val item = raw?.optJSONObject(j) ?: return@effort null
            val effort = item.optString("reasoningEffort")
            if (effort.isBlank() || !validEffort(effort)) null else effort to item.optString("description")
        }.distinctBy { it.first }
        ModelOption(model, entry.optString("displayName").ifBlank { model }, entry.optString("defaultReasoningEffort"), efforts)
    }
}

internal fun withWorkspace(params: JSONObject, workspace: String): JSONObject = params.apply {
    if (workspace.isNotEmpty()) put("workspace", workspace)
}

internal fun projectName(path: String): String = path.trimEnd('/', '\\').substringAfterLast('/').substringAfterLast('\\').ifBlank { path.ifBlank { "Other sessions" } }

internal data class RemoteProject(val id: String, val name: String, val roots: List<String>)

internal fun readProjects(result: JSONObject): List<RemoteProject> {
    val data = result.optJSONArray("data") ?: return emptyList()
    return (0 until data.length()).mapNotNull { i ->
        val p = data.optJSONObject(i) ?: return@mapNotNull null
        val id = p.optString("id")
        if (id.isBlank()) return@mapNotNull null
        val roots = p.optJSONArray("roots")
        RemoteProject(id, p.optString("name"), (0 until (roots?.length() ?: 0)).mapNotNull { j -> roots?.optJSONObject(j)?.optString("path")?.takeIf { it.isNotBlank() } })
    }
}

internal fun projectSessions(projects: List<RemoteProject>, threads: List<String>, membership: Map<String,String>): Map<String,List<String>> =
    projects.associate { project -> project.id to threads.filter { membership[it] == project.id } }
