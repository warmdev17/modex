package dev.modex

import org.json.JSONObject

internal const val CODEX_AGENT = "codex"
internal const val OPENCODE_AGENT = "opencode"
internal const val ANTIGRAVITY_AGENT = "antigravity"
internal const val ALL_AGENTS = "all"
internal val CHAT_AGENT_IDS = listOf(CODEX_AGENT, OPENCODE_AGENT, ANTIGRAVITY_AGENT)

internal data class ChatAgent(
    val id: String,
    val label: String,
    val ready: Boolean,
    val reason: String = "",
    val capabilities: Set<String>? = null,
)

internal fun readChatAgents(result: JSONObject): List<ChatAgent> {
    val data = result.optJSONArray("data") ?: return emptyList()
    return (0 until data.length()).mapNotNull { index ->
        val item = data.optJSONObject(index) ?: return@mapNotNull null
        val id = item.optString("id")
        if (id !in CHAT_AGENT_IDS) return@mapNotNull null
        val raw = item.optJSONArray("capabilities")
        val capabilities = raw?.takeIf { array -> (0 until array.length()).all { array.opt(it) is String } }
            ?.let { array -> (0 until array.length()).map(array::getString).toSet() }
        ChatAgent(id, item.optString("label").ifBlank { id }, item.optBoolean("ready"), item.optString("reason"), capabilities)
    }.distinctBy { it.id }
}

internal fun threadAgent(thread: JSONObject): String =
    thread.optString("agent").takeIf { it in CHAT_AGENT_IDS }
        ?: when {
            thread.optString("id").startsWith("oc:") -> OPENCODE_AGENT
            thread.optString("id").startsWith("agy:") -> ANTIGRAVITY_AGENT
            else -> CODEX_AGENT
        }

internal fun connectionThreadAgent(capabilities: BackendCapabilities, thread: JSONObject): String =
    thread.optString("agent").takeIf { it in CHAT_AGENT_IDS }
        ?: if (capabilities.backend == OPENCODE_AGENT && !capabilities.runtimeRouting) OPENCODE_AGENT else threadAgent(thread)

internal fun defaultChatAgent(capabilities: BackendCapabilities): String =
    if (capabilities.backend == OPENCODE_AGENT) OPENCODE_AGENT else CODEX_AGENT

internal fun safeChatAgent(preferred: String, fallback: String, agents: List<ChatAgent>): String =
    agents.firstOrNull { it.id == preferred }?.id
        ?: agents.firstOrNull { it.id == fallback && it.ready }?.id
        ?: fallback

internal fun checkingChatAgents(agents: List<ChatAgent>, default: String): List<ChatAgent> =
    agents.map { if (it.id == default) it else it.copy(ready = false, reason = "Checking availability for this workspace…") }

internal fun runtimeCapabilities(host: BackendCapabilities, agents: List<ChatAgent>, agent: String): BackendCapabilities {
    if (!host.runtimeRouting || agent == CODEX_AGENT) return host
    val keys = agents.firstOrNull { it.id == agent }?.capabilities.orEmpty()
    return BackendCapabilities(
        backend = agent,
        sessionStatus = "session_status" in keys,
        planMode = "plan_mode" in keys,
        permissionProfiles = "permission_profiles" in keys,
        sessionLifecycle = "session_lifecycle" in keys,
        projects = host.projects,
        projectCreation = host.projectCreation,
        references = "references" in keys,
        takeover = "takeover" in keys,
        agentDiscovery = host.agentDiscovery,
        runtimeRouting = true,
    )
}

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

internal fun newThreadParams(
    model: String,
    mode: String,
    workspace: String = "",
    projectId: String = "",
    standalone: Boolean = false,
    agent: String = CODEX_AGENT,
    explicitAgent: Boolean = false,
): JSONObject {
    require(mode in permissionModes)
    require(agent in CHAT_AGENT_IDS)
    require(!standalone || (workspace.isBlank() && projectId.isBlank()))
    return JSONObject().apply {
        if (explicitAgent || agent == ANTIGRAVITY_AGENT) put("agent", agent)
        if (standalone) put("standalone", true)
        if (workspace.isNotBlank()) put("workspace", workspace)
        if (projectId.isNotBlank()) put("project_id", projectId)
        if (agent != ANTIGRAVITY_AGENT && model.isNotBlank()) put("model", model)
        if (agent == CODEX_AGENT && mode != "host-default") put("permission_mode", mode)
    }
}

internal fun turnStartParams(thread: String, text: String, model: String, effort: String = "", attachments: List<Attachment> = emptyList(), references: List<PromptReference> = emptyList(), agent: String = CODEX_AGENT): JSONObject =
    JSONObject().put("thread_id", thread).put("text", text).apply {
        require(agent in CHAT_AGENT_IDS)
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

internal fun agentsListParams(workspace: String): JSONObject = JSONObject().apply {
    if (workspace.isNotBlank()) put("workspace", workspace)
}

internal fun modelsListParams(agent: String, workspace: String, cursor: String = "", routed: Boolean): JSONObject =
    JSONObject().put("limit", 100).apply {
        require(agent in CHAT_AGENT_IDS)
        if (routed) put("agent", agent)
        if (workspace.isNotBlank()) put("workspace", workspace)
        if (cursor.isNotBlank()) put("cursor", cursor)
    }

internal fun threadsListParams(agent: String, archived: Boolean, cursor: String = "", routed: Boolean): JSONObject =
    JSONObject().put("limit", 100).put("all_workspaces", true).put("archived", archived).apply {
        require(agent in CHAT_AGENT_IDS)
        if (routed) put("agent", agent)
        if (cursor.isNotBlank()) put("cursor", cursor)
    }

internal data class RuntimeScope(val agent: String, val workspace: String) {
    init { require(agent in CHAT_AGENT_IDS) }
}

internal class ScopedRuntimeRequests {
    internal data class Request(val key: String, val scope: RuntimeScope, val cursor: String, val generation: Long)
    private val scopes = mutableMapOf<String, RuntimeScope>()
    private val generations = mutableMapOf<String, Long>()
    private val pending = mutableMapOf<Long, Request>()

    fun select(key: String, scope: RuntimeScope): Boolean {
        if (scopes[key] == scope) return false
        scopes[key] = scope
        generations[key] = generations.getOrDefault(key, 0) + 1
        pending.entries.removeAll { it.value.key == key }
        return true
    }

    fun scope(key: String): RuntimeScope? = scopes[key]

    fun begin(id: Long, key: String, cursor: String = "") {
        val scope = scopes.getValue(key)
        pending[id] = Request(key, scope, cursor, generations.getOrDefault(key, 0))
    }

    fun take(id: Long): Request? = pending.remove(id)?.takeIf {
        scopes[it.key] == it.scope && generations[it.key] == it.generation
    }

    fun invalidate() {
        scopes.keys.forEach { generations[it] = generations.getOrDefault(it, 0) + 1 }
        pending.clear()
    }
}

internal class RuntimeThreadPages(private val agents: List<String>) {
    private data class Request(val agent: String, val cursor: String, val generation: Long)
    private var generation = 0L
    private val pages = mutableMapOf<String, List<String>>()
    private val cursors = mutableMapOf<String, String>()
    private val seen = mutableMapOf<String, MutableSet<String>>()
    private val pending = mutableMapOf<Long, Request>()

    init { require(agents.isNotEmpty() && agents.distinct().size == agents.size && agents.all { it in CHAT_AGENT_IDS }) }

    val ids: List<String> get() = agents.flatMap { pages[it].orEmpty() }.distinct()
    val loading: Boolean get() = pending.values.any { it.generation == generation }
    val hasMore: Boolean get() = cursors.values.any(String::isNotBlank)

    fun reset() {
        generation++
        pages.clear(); cursors.clear(); seen.clear(); pending.clear()
    }

    fun begin(id: Long, agent: String, cursor: String = "") {
        require(agent in agents)
        pending[id] = Request(agent, cursor, generation)
    }

    fun accept(id: Long, ids: List<String>, nextCursor: String): Boolean {
        val request = pending.remove(id)?.takeIf { it.generation == generation } ?: return false
        pages[request.agent] = if (request.cursor.isBlank()) ids.distinct() else (pages[request.agent].orEmpty() + ids).distinct()
        val repeated = nextCursor.isNotBlank() && (nextCursor == request.cursor || !seen.getOrPut(request.agent, ::mutableSetOf).add(nextCursor))
        cursors[request.agent] = nextCursor.takeUnless { repeated }.orEmpty()
        return true
    }

    fun fail(id: Long): Boolean = pending.remove(id)?.generation == generation

    fun initialTargets(archived: Boolean = false): List<Pair<String, String>> =
        agents.filter { !archived || it == CODEX_AGENT }.map { it to "" }
    fun moreTargets(): List<Pair<String, String>> = agents.mapNotNull { agent -> cursors[agent]?.takeIf(String::isNotBlank)?.let { agent to it } }
}

internal fun visibleThreads(
    threads: List<String>,
    agent: String,
    search: String,
    agents: Map<String, String>,
    titles: Map<String, String>,
    workspaces: Map<String, String>,
): List<String> = threads.distinct().filter { id ->
    (agent == ALL_AGENTS || agents[id] == agent) &&
        (search.isBlank() || (titles[id] ?: id).contains(search, true) || id.contains(search, true) || workspaces[id].orEmpty().contains(search, true))
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
