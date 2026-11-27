package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import android.app.Application
import android.content.Intent
import android.util.Base64
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.first

internal fun takeoverTarget(method: String, detail: String, target: String): String =
    if (method == "threads/resume" && detail.contains("active_writer", true)) target else ""

private const val CHOOSE_OPENCODE_PROVIDER = "Multiple OpenCode providers offer this model. Choose the provider in the model picker."

internal fun explainHostRejection(method: String, detail: String): String {
    return when {
        method == "projects/create" -> "Project creation was not confirmed. Check the folder and update modex-host/Codex if needed. Refresh projects before retrying."
        detail.contains("thread_has_dependents") -> "This conversation is needed by a fork. Delete dependent conversations first; nothing was deleted."
        method in listOf("threads/archive", "threads/unarchive", "threads/delete", "threads/fork") -> "Session action was not confirmed. Refresh sessions and inspect before trying again. Update modex-host if needed."
        detail.contains("response_too_large") -> "This session exceeds the history transfer limit. Open it on your computer; the host is still available."
        detail.contains("takeover_unmanaged") -> "This CLI was not started with the Modex launcher. Close it on your computer, or use make cli THREAD=<session-id> next time."
        detail.contains("takeover_mismatch") -> "The registered CLI has changed sessions or loaded multiple sessions. Nothing was taken over. Close it on your computer."
        detail.contains("takeover_unavailable") -> "The registered CLI is no longer available. Refresh the session before retrying."
        detail.contains("takeover_uncertain") -> "Takeover result is uncertain. Do not retry takeover; refresh and inspect the session first."
        detail == "model is offered by multiple OpenCode providers; select a provider-qualified model" -> CHOOSE_OPENCODE_PROVIDER
        method == "turn/start" && detail.contains("reference", true) -> "A selected skill or plugin is unavailable. Refresh references and review the prompt before sending a new turn."
        method == "turn/start" && detail.contains("usage_limit_exceeded") -> "Codex usage limit reached. Open Session Status to check reset times."
        method == "turn/start" && (detail.contains("attachment", true) || detail.contains("select PNG")) -> "An attachment could not be read. Check the file still exists, is at most 20 MiB, and images are PNG, JPEG, GIF or WebP."
        detail.contains("active_writer", ignoreCase = true) -> "This session is active in Codex Desktop or another client. Close it there, then reconnect and retry."
        detail.contains("thread_not_found", ignoreCase = true) -> "This session is no longer available in the target Codex history. Refresh sessions."
        detail.contains("model_provider_mismatch", ignoreCase = true) -> "This session belongs to a different Codex provider or model configuration."
        else -> "Host rejected $method (request_rejected). Review session state before retrying."
    }
}

// State survives rotations, but not process death. No automatic command retries.
class ModexViewModel(application: Application, storageName: String = "host") : AndroidViewModel(application) {
    private val saved = SavedHost(application, storageName)
    private val sessions = SessionState()
    private val historyCache = HistoryCache()
    private val draftCache = lazy { PromptDrafts(SavedHost(application, "$storageName-prompts"), viewModelScope) }
    internal val promptDrafts by draftCache
    internal fun flushDrafts() { if (draftCache.isInitialized()) promptDrafts.flush() }
    private data class HistoryRequest(val thread: String, val cursor: String, val activity: Long, val before: List<ChatEntry>)
    private val historyRequests = mutableMapOf<Long, HistoryRequest>()
    private data class LifecycleRequest(val target: String, val generation: Long, val selection: Long, val activity: Long)
    private data class SessionRequest(val thread: String, val generation: Long, val settingsRevision: Long, val quotaRevision: Long = 0)
    private var listGeneration = 0L
    private var selectionGeneration = 0L
    private val listRequests = mutableMapOf<Long, Long>()
    private val sharedThreadPages = RuntimeThreadPages(CHAT_AGENT_IDS)
    private val lifecycleRequests = mutableMapOf<Long, LifecycleRequest>()
    private val sessionRequests = mutableMapOf<Long, SessionRequest>()
    private var settingsRevision = 0L
    private var quotaRevision = 0L
    private var quotaSnapshot by mutableStateOf<JSONObject?>(null)
    private var quotaFailure by mutableStateOf("")
    val usageWarning: String get() = usageLimitWarning(quotaSnapshot).ifBlank { quotaFailure }
    private val observationRequests = mutableMapOf<Long, Pair<String, Long>>()
    private val observationAgain = mutableSetOf<String>()
    var archived by mutableStateOf(false); private set
    internal var backendCapabilities by mutableStateOf(BackendCapabilities.LegacyCodex); private set
    var actionThreads by mutableStateOf(emptySet<String>()); private set
    private val selectionRequests = mutableSetOf<Long>()
    private var lastThread = ""
    private var ownedThread = ""
    private var visibleHistory: CachedHistory? = null
    var olderHistoryLoading by mutableStateOf(false); private set
    var hasOlderHistory by mutableStateOf(false); private set
    var savedConnection by mutableStateOf<Connection?>(null); private set
    private var reconnectAllowed = true
    internal var projects by mutableStateOf(emptyList<RemoteProject>()); private set
    var projectCreating by mutableStateOf(false); private set
    var projectMembership by mutableStateOf(mapOf<String,String>()); private set
    var projectStatus by mutableStateOf(""); private set
    private val projectCursors = mutableSetOf<String>()
    var threadWorkspaces by mutableStateOf(mapOf<String,String>()); private set
    var selectedWorkspace by mutableStateOf(""); private set
    private val threadCursors = mutableSetOf<String>()
    var threadListLoading by mutableStateOf(false); private set
    var threadListStatus by mutableStateOf(""); private set
    var titles by mutableStateOf(mapOf<String, String>()); private set
    var historyLoading by mutableStateOf(false); private set
    var takeoverThread by mutableStateOf(""); private set
    private var saveJob: kotlinx.coroutines.Job? = null
    var connected by mutableStateOf(false); private set
    var connecting by mutableStateOf(false); private set
    internal var connectFailure by mutableStateOf(""); private set
    var error by mutableStateOf(""); private set
    var threads by mutableStateOf(listOf<String>()); private set
    val selected: String get() = sessions.selected
    internal val messages: List<ChatEntry> get() = sessions.messages
    val runningThreads: Set<String> get() = sessions.runningThreads
    var decisions by mutableStateOf(listOf<JSONObject>()); private set
    var handledPlans by mutableStateOf(setOf<String>()); private set
    val planResponding: Boolean get() = planSending
    private var planSending by mutableStateOf(false)
    var answeringDecisionIds by mutableStateOf(setOf<String>()); private set
    private val queuedAnswerRequests = mutableMapOf<Long, String>()
    val stopping: Boolean get() = sessions.stopping
    val busy: Boolean get() = sessions.busy
    val activeTurnId: String get() = sessions.activeTurnId
    val steerSending: Boolean get() = steeringDrafts.values.any { it.first == selected }
    private val steeringDrafts = androidx.compose.runtime.mutableStateMapOf<Long, Pair<String, PromptDraft?>>()
    var nextCursor by mutableStateOf<String?>(null); private set
    var sessionSettings by mutableStateOf(""); private set
    internal var sessionStatus by mutableStateOf<SessionStatus?>(null); private set
    var sessionStatusLoading by mutableStateOf(false); private set
    var sessionStatusError by mutableStateOf(""); private set
    var modeChanging by mutableStateOf(false); private set
    internal var permissionProfiles by mutableStateOf(emptyList<SessionPermissionProfile>()); private set
    var permissionsWorking by mutableStateOf(false); private set
    var permissionsMessage by mutableStateOf(""); private set
    var modeUncertain by mutableStateOf(false); private set
    internal var hostChoice by mutableStateOf(ModelChoice()); private set
    internal var modelChoices by mutableStateOf(emptyMap<String,ModelChoice>()); private set
    internal var chatAgents by mutableStateOf(listOf(ChatAgent(CODEX_AGENT, "Codex", true))); private set
    internal var agentsLoading by mutableStateOf(false); private set
    internal var threadAgents by mutableStateOf(emptyMap<String,String>()); private set
    val sessionAgent: String get() = threadAgents[selected] ?: when {
        selected.startsWith("oc:") -> OPENCODE_AGENT
        selected.startsWith("agy:") -> ANTIGRAVITY_AGENT
        else -> defaultChatAgent(backendCapabilities)
    }
    internal val sessionCapabilities: BackendCapabilities get() = runtimeCapabilities(backendCapabilities, chatAgents, sessionAgent)
    private fun choiceFor(id: String = selected) = resolveModelChoice(modelChoices[id], null, hostChoice, models)
    val sessionModel: String get() = choiceFor().model
    val sessionEffort: String get() = choiceFor().effort
    fun chooseModel(model: String, effort: String) {
        if (sessionAgent == ANTIGRAVITY_AGENT || selected.isBlank() || model.isBlank() || !validModelId(model) || !validEffort(effort)) return
        modelChoices = modelChoices + (selected to resolveModelChoice(ModelChoice(model.trim(), effort), null, hostChoice, models))
        persist()
    }
    private fun rememberChoice(id: String, reported: ModelChoice? = null) {
        val choice = resolveModelChoice(modelChoices[id], reported, hostChoice, models)
        if (choice.model.isNotBlank()) modelChoices = modelChoices + (id to choice)
    }
    internal val references = ReferenceCatalogState()
    fun refreshReferences(force: Boolean = true) {
        if (!sessionCapabilities.references || !connected || selected.isBlank() || selectedWorkspace.isBlank()) return
        if (references.loading && !force) return
        val workspace = selectedWorkspace
        request("references/list", JSONObject().put("workspace", workspace).put("force_reload", force), workspace)?.let { references.begin(it, workspace) }
    }
    internal var models by mutableStateOf(emptyList<ModelOption>()); private set
    var modelCatalogStatus by mutableStateOf(""); private set
    internal var newChatModels by mutableStateOf(emptyList<ModelOption>()); private set
    var newChatModelCatalogStatus by mutableStateOf(""); private set
    private val catalogueRequests = ScopedRuntimeRequests()
    private val catalogueCursors = mutableMapOf<String, MutableSet<String>>()
    var keyLoaded by mutableStateOf(false); private set
    private var key: ByteArray? = null
    private var config: Connection? = null
    private var journal = JournalState()
    private var transport: SshTransport? = null
    internal val attachments = AttachmentState(viewModelScope, { transport }, { config }, { persist() })
    // Avoid matching responses retained from an earlier app process.
    private var sequence = java.security.SecureRandom().nextLong().ushr(12) + 1
    private val pending = mutableMapOf<Long, Pair<String, String>>()
    private val sentAttachmentPaths = mutableMapOf<Long, Pair<String, Set<String>>>()

    private fun updateCatalogue(key: String, value: List<ModelOption>, status: String) {
        if (key == "session-models") { models = value; modelCatalogStatus = status }
        else { newChatModels = value; newChatModelCatalogStatus = status }
    }

    private fun catalogue(key: String): List<ModelOption> = if (key == "session-models") models else newChatModels

    private fun loadCatalogue(key: String, agent: String, workspace: String) {
        val scope = RuntimeScope(agent, workspace)
        val changed = catalogueRequests.select(key, scope)
        if (changed) {
            catalogueCursors.remove(key)
            updateCatalogue(key, emptyList(), if (agent == ANTIGRAVITY_AGENT) "" else "Loading models…")
        } else if (agent != ANTIGRAVITY_AGENT && catalogue(key).isEmpty()) {
            updateCatalogue(key, emptyList(), "Loading models…")
        }
        if (agent == ANTIGRAVITY_AGENT) return
        request("models/list", modelsListParams(agent, workspace, routed = backendCapabilities.runtimeRouting), key)?.let {
            catalogueRequests.begin(it, key)
        }
    }

    internal fun scopeNewChat(agent: String, workspace: String) {
        if (!connected) return
        val fallback = defaultChatAgent(backendCapabilities)
        val runtime = if (backendCapabilities.agentDiscovery) agent.takeIf { it in CHAT_AGENT_IDS } ?: fallback else fallback
        if (backendCapabilities.agentDiscovery) {
            val key = "new-agents"
            if (catalogueRequests.select(key, RuntimeScope(runtime, workspace))) {
                chatAgents = checkingChatAgents(chatAgents, fallback)
            }
            agentsLoading = true
            request("agents/list", agentsListParams(workspace), key)?.let { catalogueRequests.begin(it, key) }
                ?: run { agentsLoading = false }
        }
        loadCatalogue("new-models", runtime, workspace)
    }

    private fun loadSessionCatalogue(agent: String, workspace: String) {
        loadCatalogue("session-models", agent, workspace)
    }

    init {
        (application as ModexApplication).apply { sessions[storageName] = this@ModexViewModel; if (storageName == "host") session = this@ModexViewModel }
        try {
            saved.load()?.let { data ->
                val c = data.getJSONObject("connection")
                config = Connection(c.getString("host"), c.getInt("port"), c.getString("user"), c.getString("pin"), c.getString("binary"), c.getString("state"))
                savedConnection = config
                key = Base64.decode(data.getString("key"), Base64.NO_WRAP); keyLoaded = true
                journal = JournalState(data.optString("instance").ifEmpty { null }, data.optLong("cursor"))
                handledPlans = data.optJSONArray("handled_plans")?.let { a -> (0 until a.length()).map { a.getString(it) }.toSet() }.orEmpty()
                modelChoices = readModelChoices(data.optJSONObject("model_choices"))
                hostChoice = readModelChoice(data.optJSONObject("host_choice")) ?: ModelChoice()
                lastThread = data.optString("last_thread").ifBlank { data.optString("selected") }
                historyCache.restore(data.optJSONObject("history_cache"))
                // A stored selection is display data, not a remote writer lease.
                sessions.clearSelection()
                sessions.restoreActivity(data.optJSONObject("active_turns"))
                selectedWorkspace = data.optString("selected_workspace")
                attachments.restoreDirectories(data.optJSONObject("attachment_directories"))

                decisions = data.optJSONArray("decisions")?.let { list -> (0 until list.length()).map { list.getJSONObject(it) } } ?: emptyList()
                val requests = data.optJSONArray("pending") ?: JSONArray()
                for (i in 0 until requests.length()) {
                    val r = requests.getJSONObject(i); pending[r.getLong("id")] = r.getString("method") to r.getString("tag")
                    if (r.getString("method") == "turn/steer") {
                        val draft = r.optJSONObject("steering_draft")?.let { PromptDraft(it.getString("text"), readReferenceDraft(it.optString("references"))) }
                        steeringDrafts[r.getLong("id")] = r.getString("tag") to draft
                    }
                    if (r.getString("method") == "plans/respond") handledPlans += r.getString("tag")
                    if (r.getString("method") == "decision/respond") answeringDecisionIds += r.getString("tag")
                    if (r.getString("method") == "threads/mode") modeChanging = true
                }
            }
        } catch (_: Exception) { error = "Saved host could not be unlocked. Open Edit host to enter the details and import the SSH key again." }
    }
    private fun persist() {
        val c = config ?: return
        val privateKey = key ?: return
        val revision = saved.revision()
        saveJob?.cancel()
        saveJob = viewModelScope.launch {
            kotlinx.coroutines.delay(150)
            val cachedPages = historyCache.pages().toMutableList()
            if (selected.isNotBlank()) {
                cachedPages.removeAll { it.first == selected }
                cachedPages += selected to (visibleHistory ?: CachedHistory(emptyList(), "", "")).copy(entries = messages, revision = "")
            }
            val snapshot = JSONObject().put("connection", c.toJson())
                .put("key", Base64.encodeToString(privateKey, Base64.NO_WRAP)).put("instance", journal.instance.orEmpty()).put("cursor", journal.cursor).put("selected", selected)
                .put("model_choices", modelChoicesJson(modelChoices)).put("host_choice", hostChoice.toJson()).put("selected_workspace", selectedWorkspace).put("attachment_directories", attachments.directoriesJson())
                .put("active_turns", sessions.activityJson()).put("last_thread", lastThread)
                .put("handled_plans", JSONArray(handledPlans.toList())).put("decisions", JSONArray(decisions))
                .put("pending", JSONArray().apply {
                    pending.forEach { (id, op) ->
                        val entry = JSONObject().put("id", id).put("method", op.first).put("tag", op.second)
                        steeringDrafts[id]?.second?.let { draft ->
                            entry.put("steering_draft", JSONObject().put("text", draft.text).put("references", referenceDraftJson(draft.references)))
                        }
                        put(entry)
                    }
                })
            withContext(Dispatchers.IO) {
                try {
                    val bounded = HistoryCache()
                    for ((id, page) in cachedPages) {
                        currentCoroutineContext().ensureActive()
                        bounded.remember(id, page)
                    }
                    snapshot.put("history_cache", bounded.toJson())
                    currentCoroutineContext().ensureActive()
                    saved.save(snapshot, revision)
                } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled }
                catch (_: Exception) { withContext(Dispatchers.Main) { error = "Could not save connection securely. Keep the app open or import again next time." } }
            }
        }
    }
    private fun cacheVisibleHistory() {
        if (selected.isNotBlank()) {
            val page = visibleHistory ?: CachedHistory(emptyList(), "", "")
            historyCache.remember(selected, page.copy(entries = messages, revision = ""))
        }
    }

    internal suspend fun attachmentFile(file: Attachment): java.io.File = withContext(Dispatchers.IO) {
        if (connecting) kotlinx.coroutines.withTimeout(20000) {
            androidx.compose.runtime.snapshotFlow { connected || !connecting }.first { it }
        }
        check(connected) { "Reconnect to open this attachment." }
        val host = config ?: error("Reconnect to open this attachment.")
        val cache = java.io.File(getApplication<Application>().cacheDir, "attachment-preview")
        transport?.downloadAttachment(file, cache, host.toString(), currentCoroutineContext().let { job -> { job.ensureActive() } }) ?: error("Reconnect to open this attachment.")
    }
    fun foreground() {
        if (connected) { (runningThreads + selected).filter { it.isNotBlank() }.take(32).forEach(::observe); return }
        if (reconnectAllowed && !connected && !connecting) savedConnection?.let { connect(it) }
    }
    fun forget() {
        promptDrafts.clear()
        attachments.clear(); sentAttachmentPaths.clear(); steeringDrafts.clear(); modelChoices = emptyMap(); hostChoice = ModelChoice()
        sessionSettings = ""
        disconnect(); transport = null; connected = false; connecting = false; projectCreating = false; reconnectAllowed = false; saveJob?.cancel()
        historyCache.clear(); lastThread = ""; ownedThread = ""
        saved.clear(); key?.fill(0); key = null; keyLoaded = false; savedConnection = null
        threadWorkspaces = emptyMap(); selectedWorkspace = ""; titles = emptyMap(); projects = emptyList(); projectMembership = emptyMap(); projectStatus = ""
        journal = JournalState(); config = null; savedConnection = null; pending.clear(); handledPlans = emptySet(); planSending = false; sessions.reset(); decisions = emptyList(); answeringDecisionIds = emptySet(); threads = emptyList()
    }
    private fun loadHistory(older: Boolean = false) {
        if (!connected || selected.isBlank() || ownedThread != selected) return
        val cursor = if (older) visibleHistory?.cursor.orEmpty() else ""
        if (older && (cursor.isBlank() || olderHistoryLoading || historyLoading)) return
        if (older) { olderHistoryLoading = true; error = "" } else historyLoading = true
        val params = withWorkspace(JSONObject().put("thread_id", selected).put("include_turns", true).put("limit", 10), selectedWorkspace)
        if (cursor.isNotBlank()) params.put("cursor", cursor)
        request("threads/read", params, selected)?.let { historyRequests[it] = HistoryRequest(selected, cursor, sessions.activityVersion(selected), messages) }
    }
    fun loadOlderHistory() = loadHistory(true)
    internal val agentConversation = AgentConversation()

    internal fun openAgent(agent: AgentTimeline) {
        if (!connected || selected.isBlank() ||
            messages.none { entry -> entry.agentActivity?.targets?.any { it.id == agent.id } == true }) return
        agentConversation.open(agent)
        loadAgentHistory()
    }

    internal fun loadAgentHistory(older: Boolean = false) {
        val agent = agentConversation.agent ?: return
        if (agentConversation.loading || !connected || selected.isBlank()) return
        val params = withWorkspace(JSONObject().put("thread_id", agent.id).put("include_turns", true).put("limit", 10), selectedWorkspace)
        if (older) {
            if (agentConversation.cursor.isBlank()) return
            params.put("cursor", agentConversation.cursor)
        }
        request("threads/read", params, tag = "agent-history")?.let { agentConversation.begin(it, older) }
    }

    private fun leaveSession() {
        agentConversation.close()
        cacheVisibleHistory()
        selectionGeneration++
        settingsRevision++
        handledPlans += pending.values.filter { it.first == "plans/respond" }.map { it.second }
        planSending = false
        references.clear()
        if (selected.isNotBlank()) lastThread = selected
        ownedThread = ""
        sessions.clearSelection()
        sessionSettings = ""; sessionStatus = null; sessionStatusLoading = false; sessionStatusError = ""; modeChanging = false; modeUncertain = false; takeoverThread = ""
        sessionRequests.clear()
        permissionProfiles = emptyList()
        permissionsWorking = false
        permissionsMessage = ""
        quotaSnapshot = null
        quotaFailure = ""
        quotaRevision++
        historyLoading = false; olderHistoryLoading = false; hasOlderHistory = false
        visibleHistory = null; selectionRequests.clear(); historyRequests.clear()
    }
    fun importKey(bytes: ByteArray) {
        require(!connected && !connecting && bytes.size in 1..(256 * 1024))
        key?.fill(0); key = bytes; keyLoaded = true
    }
    private fun clearJournalState(replacement: JournalState = JournalState()) {
        leaveSession()
        journal = replacement
        pending.clear()
        queuedAnswerRequests.clear()
        sentAttachmentPaths.clear(); steeringDrafts.clear()
        threadListLoading = false
        threadListStatus = ""
        sharedThreadPages.reset()
        catalogueRequests.invalidate()
        titles = emptyMap()
        threads = emptyList()
        sessions.reset()
        selectedWorkspace = ""
        threadWorkspaces = emptyMap()
        decisions = emptyList()
        answeringDecisionIds = emptySet()
        error = ""
    }
    fun configure(value: Connection): Boolean {
        if (!value.isValid()) { error = INVALID_CONNECTION; return false }
        if (!keyLoaded) { error = "Import an unencrypted SSH private key first."; return false }
        val previous = config
        if (previous != value && (connected || connecting)) disconnect()
        if (previous != null && (previous.pin != value.pin || previous.user != value.user || previous.state != value.state)) {
            clearJournalState(); handledPlans = emptySet(); planSending = false; historyCache.clear(); lastThread = ""; modelChoices = emptyMap(); hostChoice = ModelChoice()
            promptDrafts.clear()
            attachments.clear(); projects = emptyList(); projectMembership = emptyMap(); threadAgents = emptyMap()
        }
        config = value; savedConnection = value; error = ""; persist()
        return true
    }
    fun connect(value: Connection) {
        if (connected || connecting) return
        connectFailure = ""
        if (key == null) {
            error = "Import an unencrypted SSH private key first."
            connectFailure = error
            return
        }
        if (!configure(value)) { connectFailure = error; return }
        invalidateSessionRequests()
        leaveSession()
        config = value; savedConnection = value; reconnectAllowed = true; error = ""; connecting = true
        backendCapabilities = BackendCapabilities.LegacyCodex
        try { getApplication<Application>().startForegroundService(Intent(getApplication(), ConnectionService::class.java)) }
        catch (_: Exception) {
            error = "Android could not start the connection service. Open Modex and connect again."
            connectFailure = error; connecting = false
            return
        }
        persist()
        val current = SshTransport(); transport = current
        val copy = key!!.copyOf()
        viewModelScope.launch(Dispatchers.IO) {
            try {
                fun attach(targetJournal: JournalState, recovering: Boolean) = current.open(value, copy, targetJournal, onHello = { capabilities ->
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        check(transport === current)
                        backendCapabilities = capabilities
                        if (!capabilities.sessionLifecycle) archived = false
                        if (recovering) {
                            val uncertain = pending.values.any { it.first in setOf("turn/start", "turn/steer", "turn/interrupt", "decision/respond", "plans/respond", "threads/permissions") }
                            invalidateSessionRequests()
                            clearJournalState(targetJournal)
                            if (uncertain) error = "Session reconnected. Review the conversation before repeating any action whose result was unclear."
                            persist()
                        }
                        connecting = false; connected = true; threadListLoading = false
                        models = emptyList(); modelCatalogStatus = ""
                        newChatModels = emptyList(); newChatModelCatalogStatus = ""
                        catalogueRequests.invalidate(); catalogueCursors.clear()
                        val defaultAgent = defaultChatAgent(capabilities)
                        chatAgents = listOf(ChatAgent(defaultAgent, capabilities.label, true))
                        agentsLoading = false
                        request("status", JSONObject())
                        scopeNewChat(defaultAgent, "")
                        refreshThreads()
                        runningThreads.take(32).forEach(::observe)
                    } }
                }, onFrame = { frame ->
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        check(transport === current)
                        journal.frame(frame) { apply(it) }; persist()
                    } }
                })
                val original = journal
                try {
                    attach(original, recovering = false)
                } catch (rejected: ConnectionFailure) {
                    if (!rejected.recoverJournal || original.instance == null) throw rejected
                    withContext(Dispatchers.Main) { check(transport === current && connecting) }
                    attach(JournalState(), recovering = true)
                }
            } catch (failure: Exception) {
                withContext(Dispatchers.Main) {
                    if (transport === current && reconnectAllowed) {
                        error = (failure as? ConnectionFailure)?.message ?: "Connection stopped while applying session data. Reconnect to observe; do not resend uncertain prompts."
                        if (connecting) connectFailure = error
                    }
                }
            } finally {
                copy.fill(0)
                withContext(Dispatchers.Main) { if (transport === current) {
                    connected = false; connecting = false; transport = null; threadListLoading = false; projectCreating = false
                    invalidateSessionRequests(); leaveSession(); persist()
                    getApplication<ModexApplication>().stopConnectionServiceIfIdle()
                } }
            }
        }
    }
    fun disconnect() {
        reconnectAllowed = false
        connectFailure = ""
        val current = transport; transport = null; connected = false; connecting = false; projectCreating = false; agentsLoading = false
        invalidateSessionRequests(); leaveSession(); current?.close(); persist()
        getApplication<ModexApplication>().stopConnectionServiceIfIdle()
    }
    fun create(model: String, mode: String, effort: String = "", workspace: String = "", projectId: String = "", agent: String = "") {
        if (!connected || historyLoading || switching()) return
        val runtime = agent.ifBlank { defaultChatAgent(backendCapabilities) }
        if (runtime !in CHAT_AGENT_IDS) return
        val advertised = chatAgents.firstOrNull { it.id == runtime }
        if ((backendCapabilities.runtimeRouting || runtime == ANTIGRAVITY_AGENT) && advertised?.ready != true) {
            error = advertised?.reason.orEmpty().ifBlank { "${advertised?.label ?: runtime} is unavailable on this host." }
            return
        }
        if (runtime == OPENCODE_AGENT && ambiguousOpenCodeModel(model, newChatModels)) {
            error = CHOOSE_OPENCODE_PROVIDER
            return
        }
        clearSelection()
        val choice = if (runtime != ANTIGRAVITY_AGENT) resolveModelChoice(ModelChoice(model.ifBlank { hostChoice.model }, effort), null, hostChoice, newChatModels).toJson().toString() else ""
        val params = if (backendCapabilities.runtimeRouting || runtime == ANTIGRAVITY_AGENT) {
            newThreadParams(model, mode, workspace, projectId, standalone = workspace.isBlank() && projectId.isBlank(), agent = runtime, explicitAgent = backendCapabilities.runtimeRouting)
        } else backendCapabilities.supportedNewThreadParams(model, mode, workspace, projectId)
        request("threads/start", params, choice)?.let { selectionRequests += it }
    }
    fun createProject(name: String, workspace: String) {
        if (!backendCapabilities.projectCreation || !connected || projectCreating || name.isBlank() || workspace.isBlank()) return
        projectCreating = true
        error = ""
        request("projects/create", JSONObject().put("name", name.trim()).put("workspace", workspace).put("idempotency_key", java.util.UUID.randomUUID().toString()))
    }
    fun select(id: String) {
        if (!connected || id.isBlank() || historyLoading || switching()) return
        clearSelection()
        lastThread = id
        request("threads/resume", withWorkspace(JSONObject().put("thread_id", id), threadWorkspaces[id].orEmpty()), id)?.let { selectionRequests += it }
    }
    private fun clearSelection() {
        leaveSession()
        selectedWorkspace = ""
        takeoverThread = ""
        sessionSettings = ""
        error = ""
        historyLoading = true
    }
    fun takeOver() {
        val target = takeoverThread
        if (!backendCapabilities.takeover || !connected || target.isBlank() || busy || historyLoading || switching()) return
        takeoverThread = ""
        error = ""
        historyLoading = true
        request("threads/takeover", withWorkspace(JSONObject().put("thread_id", target).put("confirm", true), threadWorkspaces[target].orEmpty()), target)?.let { selectionRequests += it }
    }
    private fun invalidateSessionRequests() {
        listGeneration++; listRequests.clear(); lifecycleRequests.clear(); actionThreads = emptySet()
        sharedThreadPages.reset(); catalogueRequests.invalidate()
        observationRequests.clear(); observationAgain.clear(); threadListLoading = false
    }
    fun showArchived(value: Boolean) {
        if (!backendCapabilities.sessionLifecycle) return
        if (archived == value) return
        archived = value; listGeneration++; threadListLoading = false
        threads = emptyList(); nextCursor = null; error = ""
        refreshThreads()
    }
    internal fun lifecycle(action: String, target: String, confirmed: Boolean = false) {
        if (!runtimeCapabilities(backendCapabilities, chatAgents, threadAgents[target] ?: connectionThreadAgent(backendCapabilities, JSONObject().put("id", target))).sessionLifecycle || !connected || target !in threads || target in actionThreads || action !in listOf("archive", "unarchive", "delete", "fork")) return
        if ((action == "unarchive") != archived && action != "delete") return
        if (action == "delete" && !confirmed) return
        if (action == "fork" && (target in runningThreads || decisions.any { it.optString("thread_id") == target } || historyLoading || switching())) return
        val params = withWorkspace(JSONObject().put("thread_id", target), threadWorkspaces[target].orEmpty())
        if (action == "delete") params.put("confirm", true)
        request("threads/$action", params, target)?.let {
            lifecycleRequests[it] = LifecycleRequest(target, listGeneration, selectionGeneration, sessions.activityVersion(target))
            actionThreads += target
        }
    }
    private fun observe(target: String) {
        if (!connected || target.isBlank()) return
        if (observationRequests.values.any { it.first == target }) { observationAgain += target; return }
        if (observationRequests.size >= 32) return
        val params = withWorkspace(JSONObject().put("thread_id", target).put("include_turns", true).put("limit", 10), threadWorkspaces[target].orEmpty())
        request("threads/read", params, target)?.let { observationRequests[it] = target to sessions.activityVersion(target) }
    }
    fun refreshThreads() {
        if (!connected || threadListLoading) return
        projects = emptyList(); projectCursors.clear(); projectStatus = ""
        if (backendCapabilities.projects) {
            projectStatus = "Loading projects…"
            request("projects/list", JSONObject().put("limit", 100))
        }
        threadCursors.clear(); threadListStatus = ""; threadListLoading = true
        if (backendCapabilities.runtimeRouting) {
            sharedThreadPages.reset()
            threads = emptyList(); nextCursor = null
            sharedThreadPages.initialTargets(archived).forEach { (agent, cursor) ->
                request("threads/list", threadsListParams(agent, archived, cursor, routed = true), agent)?.let { sharedThreadPages.begin(it, agent, cursor) }
            }
            threadListLoading = sharedThreadPages.loading
        } else {
            request("threads/list", threadsListParams(defaultChatAgent(backendCapabilities), archived, routed = false))?.let { listRequests[it] = listGeneration }
        }
    }
    fun more() {
        if (!connected || threadListLoading) return
        if (backendCapabilities.runtimeRouting) {
            sharedThreadPages.moreTargets().forEach { (agent, cursor) ->
                request("threads/list", threadsListParams(agent, archived, cursor, routed = true), agent)?.let { sharedThreadPages.begin(it, agent, cursor) }
            }
            threadListLoading = sharedThreadPages.loading
        } else nextCursor?.let { threadListLoading = true; request("threads/list", threadsListParams(defaultChatAgent(backendCapabilities), archived, it, routed = false), "more")?.let { request -> listRequests[request] = listGeneration } }
    }
    internal fun send(text: String, model: String, effort: String = "", references: List<PromptReference> = emptyList()): Boolean {
        val agent = sessionAgent
        val capabilities = sessionCapabilities
        val selectedReferences = if (capabilities.references) referencesInText(text, references) else emptyList()
        if (selectedReferences.size > 8) return false
        val files = attachments.drafts[selected].orEmpty()
        val choice = resolveModelChoice(ModelChoice(model.ifBlank { sessionModel }, effort.ifBlank { if (model.isBlank() || model == sessionModel) sessionEffort else "" }), null, hostChoice, models)
        if (!connected || steerSending || (busy && (agent == ANTIGRAVITY_AGENT || activeTurnId.isBlank() || stopping)) || modeChanging || permissionsWorking || planResponding || attachments.working || historyLoading || switching() || selected.isEmpty() || ownedThread != selected || (text.isBlank() && files.isEmpty()) || (agent == ANTIGRAVITY_AGENT && (text.isBlank() || files.isNotEmpty())) || !validModelId(model) || !validEffort(effort)) return false
        val target = activeTurnId
        if (target.isEmpty() && agent == OPENCODE_AGENT && ambiguousOpenCodeModel(choice.model, models)) {
            error = CHOOSE_OPENCODE_PROVIDER
            return false
        }
        val params = withWorkspace(if (agent == ANTIGRAVITY_AGENT) {
            turnStartParams(selected, text, choice.model, choice.effort, files, selectedReferences, agent)
        } else capabilities.supportedTurnStartParams(selected, text, choice.model, choice.effort, files, selectedReferences), selectedWorkspace)
        if (target.isNotEmpty()) {
            val id = request("turn/steer", params.put("turn_id", target), selected) ?: return false
            steeringDrafts[id] = selected to promptDrafts[selected]
            sentAttachmentPaths[id] = selected to files.map { it.path }.toSet()
            return false // Clear the draft only after Codex confirms receipt.
        }
        val id = request("turn/start", params, selected) ?: return false
        sessions.sent(ChatEntry(role = "You", text = text, attachments = files))
        sentAttachmentPaths[id] = selected to files.map { it.path }.toSet()
        return true
    }
    fun stop() {
        if (!connected || historyLoading || switching()) return
        val target = sessions.requestStop() ?: return
        request("turn/interrupt", withWorkspace(target, selectedWorkspace), target.toString())
    }
    fun refreshSessionStatus() {
        if (!sessionCapabilities.sessionStatus || !connected || selected.isBlank() || ownedThread != selected || sessionStatusLoading || modeChanging) return
        sessionStatusLoading = true; sessionStatusError = ""
        request("threads/status", withWorkspace(JSONObject().put("thread_id", selected), selectedWorkspace), selected)?.let {
            sessionRequests[it] = SessionRequest(selected, selectionGeneration, settingsRevision, quotaRevision)
        } ?: run { sessionStatusLoading = false }
    }
    fun sessionPermissions(profile: String = "") {
        if (!sessionCapabilities.permissionProfiles || !connected || selected.isBlank() || ownedThread != selected || permissionsWorking || historyLoading || switching()) return
        if (profile.isNotEmpty() && (busy || modeChanging || planResponding ||
                decisions.any { it.optString("thread_id") == selected } ||
                permissionProfiles.none { it.id == profile && it.allowed })) return
        val params = withWorkspace(JSONObject().put("thread_id", selected), selectedWorkspace)
        if (profile.isNotEmpty()) params.put("profile", profile)
        permissionsMessage = ""
        permissionsWorking = true
        request("threads/permissions", params, profile)?.let { sessionRequests[it] = SessionRequest(selected, selectionGeneration, settingsRevision) }
            ?: run { permissionsWorking = false }
    }
    fun changeSessionMode(mode: String) {
        if (!sessionCapabilities.planMode || !connected || selected.isBlank() || ownedThread != selected || busy || modeChanging || modeUncertain || sessionStatusLoading || historyLoading || switching() ||
            decisions.any { it.optString("thread_id") == selected } || mode !in listOf("default", "plan")) return
        val params = runCatching { threadModeParams(selected, selectedWorkspace, mode, sessionModel, sessionEffort) }.getOrNull() ?: return
        modeChanging = true; sessionStatusError = ""
        request("threads/mode", params, selected)?.let { sessionRequests[it] = SessionRequest(selected, selectionGeneration, settingsRevision) }
            ?: run { modeChanging = false }
    }
    internal fun respondPlan(plan: ChatEntry, action: String) {
        if (!sessionCapabilities.planMode || !connected || busy || historyLoading || switching() || ownedThread != selected || planResponding ||
            action !in listOf("implement", "fresh", "stay") || pendingPlan(messages, busy, handledPlans) != plan) return
        val params = JSONObject().put("thread_id", selected).put("turn_id", plan.turnId).put("item_id", plan.id).put("action", action)
        if (sessionModel.isNotBlank()) params.put("model", sessionModel)
        if (sessionEffort.isNotBlank()) params.put("effort", sessionEffort)
        planSending = true
        if (request("plans/respond", withWorkspace(params, selectedWorkspace), planKey(plan)) == null) planSending = false
    }
    fun answer(decision: JSONObject, result: JSONObject) {
        if (!connected) return
        val id = decision.getString("decision_id")
        if (id.startsWith("queued:")) {
            if (sessionAgent != CODEX_AGENT || backendCapabilities.backend != "codex" ||
                selected.isBlank() || ownedThread != selected || historyLoading || switching() ||
                id in answeringDecisionIds || stopping || steerSending || busy && activeTurnId.isBlank()
            ) return
            val current = queuedQuestionCards(messages).firstOrNull { it.optString("decision_id") == id } ?: return
            val text = queuedQuestionReply(current, result) ?: return
            val params = JSONObject().put("thread_id", selected).put("text", text)
            val method = if (activeTurnId.isBlank()) "turn/start" else "turn/steer"
            if (method == "turn/steer") params.put("turn_id", activeTurnId)
            request(method, withWorkspace(params, selectedWorkspace), selected)?.let { requestId ->
                queuedAnswerRequests[requestId] = id
                answeringDecisionIds += id
            }
            return
        }
        if (pending.values.any { it.first == "decision/respond" && it.second == id }) return
        answeringDecisionIds += id
        request("decision/respond", JSONObject().put("decision_id", id).put("result", result), id)
    }
    private fun switching() = selectionRequests.isNotEmpty()
    private fun request(method: String, params: JSONObject, tag: String = ""): Long? {
        if (!connected || !backendCapabilities.supports(method)) return null
        val id = sequence++; pending[id] = method to tag
        persist()
        val command = JSONObject().put("id", id).put("method", method).put("params", params)
        val destination = transport
        viewModelScope.launch(Dispatchers.IO) {
            try { destination?.send(command) ?: error("Disconnected") }
            catch (_: Exception) {
                withContext(Dispatchers.Main) {
                    if (transport === destination) {
                        if (method == "threads/read" && tag == "agent-history") {
                            pending.remove(id)
                            agentConversation.result(id, null)
                        } else error = "Delivery uncertain. Reconnect and observe before retrying."
                    }
                }
            }
        }
        return id
    }
    private fun apply(frame: JSONObject) {
        if (frame.has("id")) {
            val id = frame.getLong("id")
            queuedAnswerRequests.remove(id)?.let { answeringDecisionIds -= it }
            val sentAttachments = sentAttachmentPaths.remove(id)
            val steeringDraft = steeringDrafts.remove(id)
            val operation = pending.remove(id) ?: return
            val catalogueRequest = if (operation.first in listOf("agents/list", "models/list")) catalogueRequests.take(id) else null
            if (operation.first in listOf("agents/list", "models/list") && catalogueRequest == null) return
            if (operation.first == "threads/read" && operation.second == "agent-history") {
                agentConversation.result(id, frame.optJSONObject("result"))
                return
            }
            if (operation.first == "threads/list" && !backendCapabilities.runtimeRouting && (listRequests.remove(id) != listGeneration || !connected)) return
            val lifecycle = lifecycleRequests.remove(id)
            if (operation.first in listOf("threads/archive", "threads/unarchive", "threads/delete", "threads/fork")) {
                if (lifecycle == null) return
                actionThreads -= lifecycle.target
                if (!connected) return
            }
            val observation = observationRequests.remove(id)
            if (observation != null) {
                val (target, version) = observation
                if (!connected) return
                val result = frame.optJSONObject("result")
                if (result != null && result.optJSONObject("thread")?.optString("id") == target && result.has("history_revision") && version == sessions.activityVersion(target)) {
                    val entries = readChat(result.optJSONArray("messages") ?: JSONArray())
                    sessions.reconcileActivity(target, result.optBoolean("active_turn"), result.optString("active_turn_id"), version)
                    val page = historyCache.latest(target, entries, result.optString("next_cursor").takeUnless { result.isNull("next_cursor") }.orEmpty(), result.getString("history_revision"))
                    if (target == selected && !historyLoading && !olderHistoryLoading) {
                        visibleHistory = page; hasOlderHistory = page.cursor.isNotBlank()
                        sessions.loadHistory(target, page.entries, result.optBoolean("active_turn"), version, result.optString("active_turn_id"))
                    }
                }
                if (observationAgain.remove(target)) observe(target)
                return
            }
            if (operation.first == "projects/create") projectCreating = false
            if (operation.first == "references/list") {
                val result = frame.optJSONObject("result")
                if (result == null) references.fail(id, operation.second, selectedWorkspace)
                else references.accept(id, operation.second, selectedWorkspace, result)
                return
            }
            val historyRequest = historyRequests.remove(id)
            val sessionRequest = sessionRequests.remove(id)
            if (operation.first == "threads/permissions") {
                if (sessionRequest == null || !connected || sessionRequest.thread != selected ||
                    sessionRequest.generation != selectionGeneration || ownedThread != selected) return
                permissionsWorking = false
                val result = frame.optJSONObject("result")
                if (result == null) {
                    permissionProfiles = emptyList()
                    permissionsMessage = if (operation.second.isEmpty()) "Permissions unavailable. Check host support and reload."
                    else "Change not confirmed. Reload permissions to check the active profile."
                } else if (operation.second.isEmpty()) {
                    val entries = result.optJSONArray("profiles") ?: JSONArray()
                    permissionProfiles = (0 until entries.length()).mapNotNull { index ->
                        val item = entries.optJSONObject(index) ?: return@mapNotNull null
                        val profile = item.optString("id")
                        if (profile.isBlank()) null else SessionPermissionProfile(profile, item.optString("description"), item.optBoolean("allowed"))
                    }
                    if (permissionProfiles.isEmpty()) permissionsMessage = "No profiles reported by Codex."
                }
                refreshSessionStatus()
                return
            }
            if (operation.first in listOf("threads/mode", "threads/status") &&
                (sessionRequest == null || !connected || sessionRequest.thread != selected || sessionRequest.generation != selectionGeneration || ownedThread != selected)) {
                modeChanging = sessionRequests.any { (request, value) -> pending[request]?.first == "threads/mode" && value.thread == selected && value.generation == selectionGeneration }
                sessionStatusLoading = sessionRequests.any { (request, value) -> pending[request]?.first == "threads/status" && value.thread == selected && value.generation == selectionGeneration }
                return
            }
            if (operation.first in listOf("threads/start", "threads/resume", "threads/takeover") && (!selectionRequests.remove(id) || !connected)) return
            if (operation.first == "threads/read" && (historyRequest == null || !connected || historyRequest.thread != selected || ownedThread != selected)) return
            if (operation.first == "plans/respond") planSending = pending.values.any { it.first == "plans/respond" && it.second !in handledPlans }
            if (frame.has("error")) {
                if (operation.first == "threads/list" && backendCapabilities.runtimeRouting) {
                    if (!sharedThreadPages.fail(id)) return
                    threadListLoading = sharedThreadPages.loading
                    threads = sharedThreadPages.ids
                    nextCursor = "shared".takeIf { sharedThreadPages.hasMore }
                    val label = chatAgents.firstOrNull { it.id == operation.second }?.label ?: operation.second
                    threadListStatus = listOf(threadListStatus, "$label conversations unavailable.").filter(String::isNotBlank).joinToString(" ")
                    return
                }
                if (operation.first == "threads/mode") {
                    modeChanging = false
                    modeUncertain = true
                    sessionStatus = sessionStatus?.copy(mode = "")
                    sessionStatusError = "Mode change was not confirmed. Reload status before trying again."
                    return
                }
                if (operation.first == "threads/status") {
                    sessionStatusLoading = false
                    sessionStatusError = "Session status is unavailable. Check the connection and update modex-host if needed."
                    return
                }
                if (operation.first == "plans/respond") {
                    handledPlans += operation.second
                    error = "Plan action was not confirmed. Refresh the session and inspect it before continuing; do not resend an uncertain action. Update modex-host if needed."
                    return
                }
                if (operation.first == "projects/list") { projectStatus = "Project list unavailable. Update the host/Codex; chats remain in Recents."; return }
                if (operation.first == "models/list") {
                    updateCatalogue(catalogueRequest!!.key, catalogue(catalogueRequest.key), "Model list unavailable. You can enter a custom model ID.")
                    return
                }
                if (operation.first == "agents/list") { agentsLoading = false; return }
                if (operation.first == "turn/interrupt") {
                    val target = JSONObject(operation.second)
                    sessions.interruptRejected(target.getString("thread_id"), target.getString("turn_id"))
                    if (target.getString("thread_id") != selected) return
                }
                if (operation.first == "turn/steer") {
                    if (operation.second == selected) error = "Follow-up was not confirmed. Check the conversation before sending it again. Update modex-host if needed."
                    return
                }
                if (operation.first == "turn/start") {
                    sessions.rejectTurn(operation.second)
                    if (operation.second != selected) return
                }
                if (operation.first == "threads/read") olderHistoryLoading = false
                if (operation.first in listOf("threads/read", "threads/resume", "threads/start", "threads/takeover")) historyLoading = false
                if (operation.first == "threads/list") threadListLoading = false
                if (operation.first == "decision/respond") answeringDecisionIds -= operation.second
                val detail = frame.optJSONObject("error")?.optString("message").orEmpty()
                if (operation.first == "turn/start" && detail.contains("usage_limit_exceeded")) {
                    quotaFailure = explainHostRejection(operation.first, detail)
                    refreshSessionStatus()
                    return
                }
                if (operation.first == "threads/resume" || operation.first == "threads/takeover")
                    takeoverThread = takeoverTarget(operation.first, detail, operation.second)
                error = explainHostRejection(operation.first, detail); return
            }
            val result = frame.optJSONObject("result") ?: return
            when (operation.first) {
                "threads/archive", "threads/unarchive", "threads/delete" -> {
                    val target = lifecycle?.target ?: return
                    if (lifecycle.generation == listGeneration) threads = threads - target
                    if (operation.first != "threads/unarchive" && target == selected) leaveSession()
                    if (operation.first == "threads/delete") {
                        historyCache.remove(target); sessions.remove(target)
                        promptDrafts.update(target, PromptDraft())
                        decisions = decisions.filterNot { it.optString("thread_id") == target }
                        modelChoices -= target; threadWorkspaces -= target; threadAgents -= target; titles -= target; projectMembership -= target
                    }
                    threadListLoading = false; listGeneration++
                    refreshThreads()
                }
                "status" -> {
                    hostChoice = readModelChoice(result.optJSONObject("config")) ?: ModelChoice()
                }
                "agents/list" -> {
                    agentsLoading = false
                    val reported = readChatAgents(result)
                    val default = defaultChatAgent(backendCapabilities)
                    chatAgents = (listOf(ChatAgent(default, backendCapabilities.label, true)) + reported).distinctBy { it.id }
                }
                "threads/status" -> {
                    sessionStatusLoading = false; sessionStatusError = ""; modeUncertain = false
                    if (sessionRequest?.quotaRevision == quotaRevision) {
                        result.optJSONObject("rate_limits")?.let { quotaSnapshot = it }
                    }
                    val reported = readSessionStatus(result)
                    val current = sessionStatus
                    sessionStatus = if (sessionRequest?.settingsRevision == settingsRevision || current == null) reported
                    else reported.copy(mode = current.mode, model = current.model, effort = current.effort,
                        permissionProfile = current.permissionProfile, sandbox = current.sandbox,
                        approvalPolicy = current.approvalPolicy, approvalsReviewer = current.approvalsReviewer)
                }
                "threads/mode" -> {
                    modeChanging = false; modeUncertain = false; sessionStatusError = ""
                    val reported = readSessionStatus(result, sessionStatus)
                    val current = sessionStatus
                    sessionStatus = if (sessionRequest?.settingsRevision == settingsRevision || current == null) reported
                    else reported.copy(mode = current.mode, model = current.model, effort = current.effort)
                    if (sessionRequest?.settingsRevision == settingsRevision) sessionSettings = sessionSettingsLabel(result)
                }
                "projects/create" -> {
                    val project = result.optJSONObject("project")
                    if (project != null) projects = (projects + readProjects(JSONObject().put("data", JSONArray().put(project)))).distinctBy { it.id }
                }
                "projects/list" -> {
                    projects = (projects + readProjects(result)).distinctBy { it.id }
                    val cursor = if (result.isNull("next_cursor")) "" else result.optString("next_cursor")
                    if (cursor.isNotEmpty() && projectCursors.add(cursor)) request("projects/list", JSONObject().put("limit",100).put("cursor",cursor))
                    else projectStatus = if (cursor.isNotEmpty()) "Project listing stopped: repeated cursor." else ""
                }
                "models/list" -> {
                    val scoped = catalogueRequest ?: return
                    val entries = (catalogue(scoped.key) + readModels(result)).distinctBy { it.model }
                    val cursor = if (result.isNull("next_cursor")) "" else result.optString("next_cursor")
                    val seen = catalogueCursors.getOrPut(scoped.key, ::mutableSetOf)
                    if (cursor.isNotEmpty() && seen.add(cursor) && seen.size <= 100) {
                        updateCatalogue(scoped.key, entries, "Loading models…")
                        request("models/list", modelsListParams(scoped.scope.agent, scoped.scope.workspace, cursor, backendCapabilities.runtimeRouting), scoped.key)?.let {
                            catalogueRequests.begin(it, scoped.key, cursor)
                        }
                    } else updateCatalogue(scoped.key, entries, if (cursor.isNotEmpty()) "Model list truncated. Custom IDs remain available." else if (entries.isEmpty()) "No models advertised. Enter a custom model ID." else "")
                }
                "threads/list" -> {
                    if (error.startsWith("Host rejected threads/list")) error = ""
                    val list = result.optJSONArray("data") ?: JSONArray()
                    titles = titles + (0 until list.length()).associate { i -> val t = list.getJSONObject(i); t.getString("id") to sessionLabel(t) }
                    val paths = (0 until list.length()).associate { i -> val t = list.getJSONObject(i); t.getString("id") to t.optString("cwd") }
                    val runtimes = (0 until list.length()).associate { i -> val t = list.getJSONObject(i); t.getString("id") to connectionThreadAgent(backendCapabilities, t) }
                    val membership = readProjectMembership(list)
                    projectMembership = projectMembership + membership
                    threadWorkspaces = threadWorkspaces + paths
                    threadAgents = threadAgents + runtimes
                    val ids = (0 until list.length()).map { list.getJSONObject(it).getString("id") }
                    val cursor = if (result.isNull("next_cursor")) "" else result.optString("next_cursor")
                    if (backendCapabilities.runtimeRouting) {
                        if (!sharedThreadPages.accept(id, ids, cursor)) return
                        threads = sharedThreadPages.ids
                        threadListLoading = sharedThreadPages.loading
                        nextCursor = "shared".takeIf { sharedThreadPages.hasMore }
                    } else {
                        threads = if (operation.second == "more") (threads + ids).distinct() else ids
                        nextCursor = cursor.ifEmpty { null }
                        threadListLoading = false
                        nextCursor?.let { next ->
                            if (!threadCursors.add(next)) { nextCursor = null; error = "Session listing stopped because the host repeated a page cursor. Refresh to retry." }
                        }
                    }
                }
                "threads/start", "threads/resume", "threads/takeover", "threads/fork" -> {
                    if (operation.first == "threads/fork") {
                        if (lifecycle == null || lifecycle.generation != listGeneration || selectionGeneration != lifecycle.selection || sessions.activityVersion(lifecycle.target) != lifecycle.activity || historyLoading || switching()) return
                        val thread = result.optJSONObject("thread")
                        if (thread?.optString("id").isNullOrBlank() || thread?.optString("id") == lifecycle.target || thread?.optString("cwd") != threadWorkspaces[lifecycle.target]) {
                            error = "Fork was not confirmed. Refresh sessions and inspect before trying again."; return
                        }
                        leaveSession()
                    }
                    takeoverThread = ""
                    error = ""
                    sessionSettings = sessionSettingsLabel(result)
                    sessionStatus = readSessionStatus(result)
                    modeUncertain = false
                    val openedThread = result.getJSONObject("thread")
                    sessions.open(openedThread.getString("id"))
                    ownedThread = selected; lastThread = selected
                    threadAgents = threadAgents + (selected to connectionThreadAgent(backendCapabilities, openedThread))
                    visibleHistory = historyCache.get(selected)
                    visibleHistory?.let { sessions.loadHistory(selected, it.entries, selected in runningThreads); hasOlderHistory = it.cursor.isNotBlank() }
                    val reported = ModelChoice(result.optString("model"), result.optString("effort"))
                    if (operation.first == "threads/start") {
                        val chosen = runCatching { readModelChoice(JSONObject(operation.second)) }.getOrNull()
                        if (chosen != null) modelChoices = modelChoices + (selected to chosen)
                    }
                    rememberChoice(selected, reported)
                    selectedWorkspace = openedThread.optString("cwd")
                    loadSessionCatalogue(sessionAgent, selectedWorkspace)
                    refreshReferences(false)
                    projectMembership = projectMembership + readProjectMembership(JSONArray().put(openedThread))
                    threadWorkspaces = threadWorkspaces + (selected to selectedWorkspace)
                    threads = (listOf(selected) + threads).distinct()
                    if (operation.first == "threads/start") {
                        // Codex cannot read persisted turns until a new thread has its first turn.
                        historyLoading = false
                    } else loadHistory()
                    refreshSessionStatus()
                }
                "threads/read" -> {
                    val request = historyRequest ?: return
                    historyLoading = false; olderHistoryLoading = false
                    if (!result.has("history_revision")) {
                        error = "Update modex-host and Codex to load paged history."
                        return
                    }
                    val thread = result.optJSONObject("thread")
                    rememberChoice(selected, thread?.let { ModelChoice(it.optString("model"), it.optString("reasoningEffort")) })
                    // Do not replace newer streamed items with an older history snapshot.
                    val changed = request.activity != sessions.activityVersion(selected)
                    val entries = readChat(result.getJSONArray("messages"))
                    val cursor = result.optString("next_cursor").takeUnless { result.isNull("next_cursor") }.orEmpty()
                    val page = if (request.cursor.isBlank()) {
                        if (changed) CachedHistory(mergeHistoryEntries(entries, messages.filterNot(request.before.toSet()::contains)), cursor, "").also { historyCache.remember(selected, it) }
                        else historyCache.latest(selected, entries, cursor, result.getString("history_revision"))
                    }
                    else {
                        val previous = visibleHistory ?: return
                        if (previous.cursor != request.cursor || cursor == request.cursor) { error = "History cursor did not advance. Reopen this session."; return }
                        previous.copy(entries = mergeHistoryEntries(entries, messages), cursor = cursor).also { historyCache.remember(selected, it) }
                    }
                    visibleHistory = page; hasOlderHistory = page.cursor.isNotBlank()
                    val active = if (request.cursor.isBlank()) result.optBoolean("active_turn") else busy
                    sessions.loadHistory(selected, page.entries, active, if (request.cursor.isBlank()) request.activity else -1, result.optString("active_turn_id"))
                }
                "plans/respond" -> {
                    handledPlans += operation.second
                    val target = result.optString("thread_id")
                    val session = result.optJSONObject("session")
                    if (session != null) {
                        val source = result.optString("source_thread_id")
                        modelChoices = modelChoices + (target to resolveModelChoice(readModelChoice(session) ?: choiceFor(source), null, hostChoice, models))
                        val workspace = session.optJSONObject("thread")?.optString("cwd").orEmpty()
                        if (workspace.isNotBlank()) threadWorkspaces = threadWorkspaces + (target to workspace)
                        threads = (listOf(target) + threads).distinct()
                    }
                    if (connected && selected == result.optString("source_thread_id") && result.optString("action") != "stay" && target.isNotBlank()) {
                        if (target == selected) { sessionSettings = session?.let(::sessionSettingsLabel).orEmpty(); loadHistory() }
                        else select(target)
                    }
                }
                "decision/respond" -> {
                    answeringDecisionIds -= operation.second
                    decisions = decisions.filterNot { it.optString("decision_id") == operation.second }
                }
                "turn/steer" -> {
                    if (result.optString("turnId").isBlank() && result.optJSONObject("turn")?.optString("id").isNullOrBlank()) {
                        if (operation.second == selected) error = "Follow-up was not confirmed. Check the conversation before sending it again."
                        return
                    }
                    steeringDraft?.let { (thread, submitted) ->
                        if (submitted != null && promptDrafts[thread] == submitted) {
                            promptDrafts.update(thread, PromptDraft())
                            promptDrafts.flush()
                        }
                    }
                    sentAttachments?.let { attachments.sent(it.first, it.second) }
                }
                "turn/start" -> { result.optJSONObject("turn")?.let { sessions.acknowledgeTurn(operation.second, it) }; sentAttachments?.let { attachments.sent(it.first, it.second) } }
            }
            return
        }
        if (frame.optString("event") == "skills/changed") { refreshReferences(); return }
        val params = frame.optJSONObject("params") ?: return
        if (frame.optString("event") == "plan/resolved") {
            params.optString("plan_key").takeIf { it.isNotBlank() }?.let { handledPlans += it }
        }
        val event = frame.optString("event")
        if (params.optString("threadId") == agentConversation.agent?.id) {
            agentConversation.event(event, params)
            if ((event == "turn/started" || event == "turn/completed") &&
                (agentConversation.error || agentConversation.messages.isEmpty())) loadAgentHistory()
        }
        if (backendCapabilities.sessionStatus && event == "account/rateLimits/updated") {
            params.optJSONObject("rateLimits")?.let { limits ->
                if (limits.optString("limitId").let { it.isBlank() || it == "codex" } || usageLimitWarning(limits).isNotBlank()) {
                    quotaRevision++
                    quotaSnapshot = limits
                }
            }
        }
        if ((event == "error" || event == "turn/completed") &&
            (params.optString("threadId") == selected || (event == "error" && params.optString("threadId").isBlank()))) {
            val notice = eventTurnNotice(params)
            if (notice != null && !notice.retrying && notice.category in listOf("usage_limit_exceeded", "rate_limit_exceeded", "session_budget_exceeded")) {
                quotaFailure = notice.message
            } else if (event == "turn/completed" && params.optJSONObject("turn")?.optString("status") == "completed") {
                quotaFailure = ""
            }
        }
        if ((backendCapabilities.sessionStatus || backendCapabilities.planMode || backendCapabilities.permissionProfiles) &&
            event == "thread/settings/updated" && params.optString("threadId") == selected && ownedThread == selected) {
            val settings = params.optJSONObject("threadSettings") ?: return
            val previous = sessionStatus ?: SessionStatus(workspace = selectedWorkspace)
            val reportedMode = settings.optJSONObject("collaborationMode")?.optString("mode").orEmpty()
            sessionStatus = previous.copy(
                mode = reportedMode.takeIf { it == "default" || it == "plan" } ?: previous.mode,
                model = if (settings.has("model") && !settings.isNull("model")) settings.optString("model") else previous.model,
                effort = if (settings.has("effort")) if (settings.isNull("effort")) "" else settings.optString("effort") else previous.effort,
                permissionProfile = settings.optString("permission_profile", previous.permissionProfile),
                sandbox = settings.optString("sandbox", previous.sandbox),
                approvalPolicy = settings.optString("approval_policy", previous.approvalPolicy),
                approvalsReviewer = settings.optString("approvals_reviewer", previous.approvalsReviewer),
            )
            settingsRevision++
            if (reportedMode == "default" || reportedMode == "plan") modeUncertain = false
            sessionStatusError = ""
        }
        val before = messages
        sessions.event(event, params)
        // Streaming prefixes stay in SessionState; only snapshot/terminal/navigation boundaries encode history.
        if (messages !== before) {
            if (event.endsWith("Delta") || event.endsWith("/delta")) historyCache.invalidateRevision(selected)
            else cacheVisibleHistory()
        }
        if (event in listOf("thread/status/changed", "thread/closed")) {
            observe(params.optString("threadId"))
        }
        decisions = updatePendingDecisions(decisions, frame.optString("event"), params)
        if (event == "turn/completed") {
            if (params.optString("threadId") != selected) observe(params.optString("threadId")) else refreshSessionStatus()
            decisions = decisions.filterNot { it.optString("thread_id") == params.optString("threadId") && it.optString("turn_id") == params.optJSONObject("turn")?.optString("id") }
        }
    }

    override fun onCleared() { transport?.close(); key?.fill(0) }
}
