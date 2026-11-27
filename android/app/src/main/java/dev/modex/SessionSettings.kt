package dev.modex

import org.json.JSONObject

internal const val CODEX_AGENT = "codex"
internal const val ANTIGRAVITY_AGENT = "antigravity"

internal data class ChatAgent(val id: String, val label: String, val ready: Boolean, val reason: String = "")

internal fun readChatAgents(result: JSONObject): List<ChatAgent> {
    val data = result.optJSONArray("data") ?: return emptyList()
    return (0 until data.length()).mapNotNull { index ->
        val item = data.optJSONObject(index) ?: return@mapNotNull null
        val id = item.optString("id")
        if (id !in listOf(CODEX_AGENT, ANTIGRAVITY_AGENT)) return@mapNotNull null
        ChatAgent(id, item.optString("label").ifBlank { id }, item.optBoolean("ready"), item.optString("reason"))
    }.distinctBy { it.id }
}

internal fun threadAgent(thread: JSONObject): String =
    thread.optString("agent").takeIf { it in listOf(CODEX_AGENT, ANTIGRAVITY_AGENT) }
        ?: if (thread.optString("id").startsWith("agy:")) ANTIGRAVITY_AGENT else CODEX_AGENT

internal data class SessionPermissionProfile(val id: String, val description: String, val allowed: Boolean) {
    val label get() = when (id) {
        ":read-only" -> "Read-only"
        ":workspace" -> "Workspace write"
        ":danger-full-access" -> "Full access"
        else -> id
    }
}

internal val permissionModes = linkedMapOf(
    "host-default" to "Host default",
    "read-only" to "Read-only",
    "workspace-write" to "Workspace write",
    "full-access" to "Full access"
)

internal fun newThreadParams(model: String, mode: String, workspace: String = "", projectId: String = "", standalone: Boolean = false, agent: String = CODEX_AGENT): JSONObject {
    require(mode in permissionModes)
    require(agent in listOf(CODEX_AGENT, ANTIGRAVITY_AGENT))
    require(!standalone || (workspace.isBlank() && projectId.isBlank()))
    return JSONObject().apply {
        if (agent == ANTIGRAVITY_AGENT) put("agent", agent)
        if (standalone) put("standalone", true)
        if (workspace.isNotBlank()) put("workspace", workspace)
        if (projectId.isNotBlank()) put("project_id", projectId)
        if (agent == CODEX_AGENT && model.isNotBlank()) put("model", model)
        if (agent == CODEX_AGENT && mode != "host-default") put("permission_mode", mode)
    }
}

internal fun turnStartParams(thread: String, text: String, model: String, effort: String = "", attachments: List<Attachment> = emptyList(), references: List<PromptReference> = emptyList(), agent: String = CODEX_AGENT): JSONObject =
    JSONObject().put("thread_id", thread).put("text", text).apply {
        require(agent in listOf(CODEX_AGENT, ANTIGRAVITY_AGENT))
        if (agent == ANTIGRAVITY_AGENT) return@apply
        if (references.isNotEmpty()) put("references", org.json.JSONArray().apply { references.forEach { put(it.selectionJson()) } })
        require(validModelId(model) && validEffort(effort))
        if (attachments.isNotEmpty()) put("attachments", org.json.JSONArray().apply { attachments.forEach { put(it.toJson()) } })
        if (model.isNotBlank()) put("model", model.trim())
        if (effort.isNotBlank()) put("effort", effort)
    }

internal fun BackendCapabilities.supportedNewThreadParams(model: String, mode: String, workspace: String = "", projectId: String = ""): JSONObject =
    newThreadParams(
        model,
        if (permissionProfiles) mode else "host-default",
        if (projects) workspace else "",
        if (projects) projectId else "",
        standalone = !projects || (workspace.isBlank() && projectId.isBlank()),
    )

internal fun BackendCapabilities.supportedTurnStartParams(
    thread: String,
    text: String,
    model: String,
    effort: String = "",
    files: List<Attachment> = emptyList(),
    references: List<PromptReference> = emptyList(),
): JSONObject = turnStartParams(thread, text, model, effort, files, if (this.references) references else emptyList())

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

// Empty membership marks a deliberate Desktop projectless assignment; absent membership can use roots.
internal fun readProjectMembership(data: org.json.JSONArray): Map<String, String> =
    (0 until data.length()).mapNotNull { i ->
        val thread = data.getJSONObject(i)
        val project = thread.optString("projectId").takeUnless { thread.isNull("projectId") || it.isBlank() }
        when {
            project != null -> thread.getString("id") to project
            thread.optBoolean("projectless") -> thread.getString("id") to ""
            else -> null
        }
    }.toMap()

internal fun projectSessions(projects: List<RemoteProject>, threads: List<String>, membership: Map<String,String>, workspaces: Map<String,String> = emptyMap()): Map<String,List<String>> {
    fun path(value: String): String {
        val normalized = value.replace('\\', '/').trimEnd('/')
        return if (normalized.length >= 2 && normalized[1] == ':') normalized.lowercase(java.util.Locale.ROOT) else normalized
    }
    val roots = projects.flatMap { p -> p.roots.filter { it.isNotBlank() }.map { path(it) to p.id } }
    val assignments = threads.associateWith { id ->
        if (membership.containsKey(id)) membership[id]
        else workspaces[id]?.takeIf { it.isNotBlank() }?.let { cwd ->
            val directory = path(cwd)
            val matches = roots.filter { (root, _) -> directory == root || directory.startsWith("$root/") }
            val longest = matches.maxOfOrNull { it.first.length }
            matches.filter { it.first.length == longest }.map { it.second }.distinct().singleOrNull()
        }
    }
    return projects.associate { project -> project.id to threads.filter { assignments[it] == project.id } }
}
