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

internal fun explainHostRejection(method: String, detail: String): String {
    return when {
        detail.contains("response_too_large") -> "This session exceeds the history transfer limit. Open it on your computer; the host is still available."
        detail.contains("takeover_unmanaged") -> "This CLI was not started with the Modex launcher. Close it on your computer, or use make cli THREAD=<session-id> next time."
        detail.contains("takeover_mismatch") -> "The registered CLI has changed sessions or loaded multiple sessions. Nothing was taken over. Close it on your computer."
        detail.contains("takeover_unavailable") -> "The registered CLI is no longer available. Refresh the session before retrying."
        detail.contains("takeover_uncertain") -> "Takeover result is uncertain. Do not retry takeover; refresh and inspect the session first."
        method == "turn/start" && (detail.contains("attachment", true) || detail.contains("select PNG")) -> "An attachment could not be read. Check the file still exists, is at most 20 MiB, and images are PNG, JPEG, GIF or WebP."
        detail.contains("active_writer", ignoreCase = true) -> "This session is active in Codex Desktop or another client. Close it there, then reconnect and retry."
        detail.contains("thread_not_found", ignoreCase = true) -> "This session is no longer available in the target Codex history. Refresh sessions."
        detail.contains("model_provider_mismatch", ignoreCase = true) -> "This session belongs to a different Codex provider or model configuration."
        else -> "Host rejected $method (request_rejected). Review session state before retrying."
    }
}

// State survives rotations, but not process death. No automatic command retries.
class ModexViewModel(application: Application) : AndroidViewModel(application) {
    private val saved = SavedHost(application)
    private val sessions = SessionState()
    private val historyCache = HistoryCache()
    private data class HistoryRequest(val thread: String, val cursor: String, val activity: Long, val before: List<ChatEntry>)
    private val historyRequests = mutableMapOf<Long, HistoryRequest>()
    private val selectionRequests = mutableSetOf<Long>()
    private var lastThread = ""
    private var ownedThread = ""
    private var visibleHistory: CachedHistory? = null
    var olderHistoryLoading by mutableStateOf(false); private set
    var hasOlderHistory by mutableStateOf(false); private set
    var savedConnection by mutableStateOf<Connection?>(null); private set
    private var reconnectAllowed = true
    internal var projects by mutableStateOf(emptyList<RemoteProject>()); private set
    var projectMembership by mutableStateOf(mapOf<String,String>()); private set
    var projectStatus by mutableStateOf(""); private set
    private val projectCursors = mutableSetOf<String>()
    var threadWorkspaces by mutableStateOf(mapOf<String,String>()); private set
    var selectedWorkspace by mutableStateOf(""); private set
    private val threadCursors = mutableSetOf<String>()
    var threadListLoading by mutableStateOf(false); private set
    var titles by mutableStateOf(mapOf<String, String>()); private set
    var historyLoading by mutableStateOf(false); private set
    var takeoverThread by mutableStateOf(""); private set
    private var saveJob: kotlinx.coroutines.Job? = null
    var connected by mutableStateOf(false); private set
    var connecting by mutableStateOf(false); private set
    var error by mutableStateOf(""); private set
    var threads by mutableStateOf(listOf<String>()); private set
    val selected: String get() = sessions.selected
    internal val messages: List<ChatEntry> get() = sessions.messages
    val runningThreads: Set<String> get() = sessions.runningThreads
    var decisions by mutableStateOf(listOf<JSONObject>()); private set
    var answeringDecisionIds by mutableStateOf(setOf<String>()); private set
    val stopping: Boolean get() = sessions.stopping
    val busy: Boolean get() = sessions.busy
    var nextCursor by mutableStateOf<String?>(null); private set
    var sessionSettings by mutableStateOf(""); private set
    internal var hostChoice by mutableStateOf(ModelChoice()); private set
    internal var modelChoices by mutableStateOf(emptyMap<String,ModelChoice>()); private set
    private fun choiceFor(id: String = selected) = resolveModelChoice(modelChoices[id], null, hostChoice, models)
    val sessionModel: String get() = choiceFor().model
    val sessionEffort: String get() = choiceFor().effort
    fun chooseModel(model: String, effort: String) {
        if (selected.isBlank() || model.isBlank() || !validModelId(model) || !validEffort(effort)) return
        modelChoices = modelChoices + (selected to resolveModelChoice(ModelChoice(model.trim(), effort), null, hostChoice, models))
        persist()
    }
    private fun rememberChoice(id: String, reported: ModelChoice? = null) {
        val choice = resolveModelChoice(modelChoices[id], reported, hostChoice, models)
        if (choice.model.isNotBlank()) modelChoices = modelChoices + (id to choice)
    }
    internal var models by mutableStateOf(emptyList<ModelOption>()); private set
    var modelCatalogStatus by mutableStateOf(""); private set
    private val modelCursors = mutableSetOf<String>()
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

    init {
        (application as ModexApplication).session = this
        try {
            saved.load()?.let { data ->
                val c = data.getJSONObject("connection")
                config = Connection(c.getString("host"), c.getInt("port"), c.getString("user"), c.getString("pin"), c.getString("binary"), c.getString("state"))
                savedConnection = config
                key = Base64.decode(data.getString("key"), Base64.NO_WRAP); keyLoaded = true
                journal = JournalState(data.optString("instance").ifEmpty { null }, data.optLong("cursor"))
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
                    if (r.getString("method") == "decision/respond") answeringDecisionIds += r.getString("tag")
                }
            }
        } catch (_: Exception) { error = "Saved host could not be unlocked. Forget it and import the SSH key again." }
    }
    private fun persist() {
        val c = config ?: return
        val privateKey = key ?: return
        val revision = saved.revision()
        saveJob?.cancel()
        saveJob = viewModelScope.launch {
            kotlinx.coroutines.delay(150)
            val snapshot = JSONObject().put("connection", JSONObject().put("host", c.host).put("port", c.port).put("user", c.user).put("pin", c.pin).put("binary", c.binary).put("state", c.state))
                .put("key", Base64.encodeToString(privateKey, Base64.NO_WRAP)).put("instance", journal.instance.orEmpty()).put("cursor", journal.cursor).put("selected", selected)
                .put("model_choices", modelChoicesJson(modelChoices)).put("host_choice", hostChoice.toJson()).put("selected_workspace", selectedWorkspace).put("attachment_directories", attachments.directoriesJson())
                .put("active_turns", sessions.activityJson()).put("last_thread", lastThread).put("history_cache", historyCache.toJson())
                .put("decisions", JSONArray(decisions)).put("pending", JSONArray().apply { pending.forEach { (id, op) -> put(JSONObject().put("id", id).put("method", op.first).put("tag", op.second)) } })
            withContext(Dispatchers.IO) {
            try { saved.save(snapshot, revision) } catch (_: Exception) { withContext(Dispatchers.Main) { error = "Could not save connection securely. Keep the app open or import again next time." } }
        } }
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
        if (reconnectAllowed && !connected && !connecting) savedConnection?.let { connect(it) }
    }
    fun forget() {
        attachments.clear(); sentAttachmentPaths.clear(); modelChoices = emptyMap(); hostChoice = ModelChoice()
        sessionSettings = ""
        disconnect(); transport = null; connected = false; connecting = false; reconnectAllowed = false; saveJob?.cancel()
        historyCache.clear(); lastThread = ""; ownedThread = ""
        saved.clear(); key?.fill(0); key = null; keyLoaded = false; savedConnection = null
        threadWorkspaces = emptyMap(); selectedWorkspace = ""; titles = emptyMap(); projects = emptyList(); projectMembership = emptyMap(); projectStatus = ""
        journal = JournalState(); config = null; savedConnection = null; pending.clear(); sessions.reset(); decisions = emptyList(); answeringDecisionIds = emptySet(); threads = emptyList()
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
    private fun leaveSession() {
        if (selected.isNotBlank()) lastThread = selected
        ownedThread = ""
        sessions.clearSelection()
        sessionSettings = ""; takeoverThread = ""
        historyLoading = false; olderHistoryLoading = false; hasOlderHistory = false
        visibleHistory = null; selectionRequests.clear(); historyRequests.clear()
    }
    fun importKey(bytes: ByteArray) {
        require(!connected && !connecting && bytes.size in 1..(256 * 1024))
        key?.fill(0); key = bytes; keyLoaded = true
    }
    fun reset() {
        if (connected || connecting) return
        sessionSettings = ""; threadListLoading = false; titles = emptyMap()
        leaveSession()
        journal = JournalState(); pending.clear(); sentAttachmentPaths.clear(); threads = emptyList()
        sessions.reset(); historyLoading = false; selectedWorkspace = ""; threadWorkspaces = emptyMap(); decisions = emptyList(); answeringDecisionIds = emptySet(); error = ""
        persist()
    }
    fun connect(value: Connection) {
        if (connected || connecting) return
        if (key == null) { error = "Import an unencrypted SSH private key first."; return }
        if (config != null && config != value) { error = "Forget saved host before changing the target."; return }
        config = value; savedConnection = value; reconnectAllowed = true; error = ""; connecting = true
        try { getApplication<Application>().startForegroundService(Intent(getApplication(), ConnectionService::class.java)) }
        catch (_: Exception) { connecting = false; error = "Android could not start the connection service. Open Modex and connect again."; return }
        persist()
        val current = SshTransport(); transport = current
        val copy = key!!.copyOf()
        viewModelScope.launch(Dispatchers.IO) {
            try {
                current.open(value, copy, journal, onHello = {
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        check(transport === current)
                        connecting = false; connected = true; threadListLoading = false
                        models = emptyList(); modelCursors.clear(); modelCatalogStatus = "Loading models…"
                        request("status", JSONObject())
                        request("models/list", JSONObject().put("limit", 100))
                        refreshThreads()
                        if (lastThread.isNotBlank()) select(lastThread)
                    } }
                }, onFrame = { frame ->
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        check(transport === current)
                        journal.frame(frame) { apply(it) }; persist()
                    } }
                })
            } catch (failure: Exception) {
                withContext(Dispatchers.Main) { if (transport === current && reconnectAllowed) error = (failure as? ConnectionFailure)?.message ?: "Connection stopped while applying session data. Reconnect to observe; do not resend uncertain prompts." }
            } finally {
                copy.fill(0)
                withContext(Dispatchers.Main) { if (transport === current) {
                    connected = false; connecting = false; transport = null; threadListLoading = false
                    leaveSession(); persist()
                    getApplication<Application>().stopService(Intent(getApplication(), ConnectionService::class.java))
                } }
            }
        }
    }
    fun disconnect() {
        reconnectAllowed = false
        val current = transport; transport = null; connected = false; connecting = false
        leaveSession(); current?.close(); persist()
        getApplication<Application>().stopService(Intent(getApplication(), ConnectionService::class.java))
    }
    fun create(model: String, mode: String, effort: String = "") {
        if (!connected || historyLoading || switching()) return
        val workspace = selectedWorkspace
        clearSelection()
        request("threads/start", withWorkspace(newThreadParams(model, mode), workspace), resolveModelChoice(ModelChoice(model.ifBlank { hostChoice.model }, effort), null, hostChoice, models).toJson().toString())?.let { selectionRequests += it }
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
        if (!connected || target.isBlank() || busy || historyLoading || switching()) return
        takeoverThread = ""
        error = ""
        historyLoading = true
        request("threads/takeover", withWorkspace(JSONObject().put("thread_id", target).put("confirm", true), threadWorkspaces[target].orEmpty()), target)?.let { selectionRequests += it }
    }
    fun refreshThreads() {
        if (!connected || threadListLoading) return
        projects = emptyList(); projectCursors.clear(); projectStatus = "Loading projects…"
        request("projects/list", JSONObject().put("limit", 100))
        threadCursors.clear(); threadListLoading = true
        request("threads/list", JSONObject().put("limit", 100).put("all_workspaces", true))
    }
    fun more() {
        if (!connected || threadListLoading) return
        nextCursor?.let { threadListLoading = true; request("threads/list", JSONObject().put("limit", 100).put("all_workspaces", true).put("cursor", it), "more") }
    }
    fun send(text: String, model: String, effort: String = ""): Boolean {
        val files = attachments.drafts[selected].orEmpty()
        val choice = resolveModelChoice(ModelChoice(model.ifBlank { sessionModel }, effort.ifBlank { if (model.isBlank() || model == sessionModel) sessionEffort else "" }), null, hostChoice, models)
        if (!connected || busy || attachments.working || historyLoading || switching() || selected.isEmpty() || ownedThread != selected || (text.isBlank() && files.isEmpty()) || !validModelId(model) || !validEffort(effort)) return false
        sessions.sent(ChatEntry(role = "You", text = text, attachments = files))
        val id = request("turn/start", withWorkspace(turnStartParams(selected, text, choice.model, choice.effort, files), selectedWorkspace), selected)
        if (id != null) sentAttachmentPaths[id] = selected to files.map { it.path }.toSet()
        return true
    }
    fun stop() {
        if (!connected || historyLoading || switching()) return
        val target = sessions.requestStop() ?: return
        request("turn/interrupt", withWorkspace(target, selectedWorkspace), target.toString())
    }
    fun answer(decision: JSONObject, result: JSONObject) {
        if (!connected) return
        val id = decision.getString("decision_id")
        if (pending.values.any { it.first == "decision/respond" && it.second == id }) return
        answeringDecisionIds += id
        request("decision/respond", JSONObject().put("decision_id", id).put("result", result), id)
    }
    private fun switching() = selectionRequests.isNotEmpty()
    private fun request(method: String, params: JSONObject, tag: String = ""): Long? {
        if (!connected) return null
        val id = sequence++; pending[id] = method to tag
        persist()
        val command = JSONObject().put("id", id).put("method", method).put("params", params)
        val destination = transport
        viewModelScope.launch(Dispatchers.IO) {
            try { destination?.send(command) ?: error("Disconnected") }
            catch (_: Exception) { withContext(Dispatchers.Main) { if (transport === destination) error = "Delivery uncertain. Reconnect and observe before retrying." } }
        }
        return id
    }
    private fun apply(frame: JSONObject) {
        if (frame.has("id")) {
            val id = frame.getLong("id")
            val sentAttachments = sentAttachmentPaths.remove(id)
            val operation = pending.remove(id) ?: return
            val historyRequest = historyRequests.remove(id)
            if (operation.first in listOf("threads/start", "threads/resume", "threads/takeover") && (!selectionRequests.remove(id) || !connected)) return
            if (operation.first == "threads/read" && (historyRequest == null || !connected || historyRequest.thread != selected || ownedThread != selected)) return
            if (frame.has("error")) {
                if (operation.first == "projects/list") { projectStatus = "Project list unavailable. Update the host/Codex; chats remain in Recents."; return }
                if (operation.first == "models/list") { modelCatalogStatus = "Model list unavailable. You can enter a custom model ID."; return }
                if (operation.first == "turn/interrupt") {
                    val target = JSONObject(operation.second)
                    sessions.interruptRejected(target.getString("thread_id"), target.getString("turn_id"))
                    if (target.getString("thread_id") != selected) return
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
                if (operation.first == "threads/resume" || operation.first == "threads/takeover")
                    takeoverThread = takeoverTarget(operation.first, detail, operation.second)
                error = explainHostRejection(operation.first, detail); return
            }
            val result = frame.optJSONObject("result") ?: return
            when (operation.first) {
                "status" -> {
                    hostChoice = readModelChoice(result.optJSONObject("config")) ?: ModelChoice()
                }
                "projects/list" -> {
                    projects = (projects + readProjects(result)).distinctBy { it.id }
                    val cursor = if (result.isNull("next_cursor")) "" else result.optString("next_cursor")
                    if (cursor.isNotEmpty() && projectCursors.add(cursor)) request("projects/list", JSONObject().put("limit",100).put("cursor",cursor))
                    else projectStatus = if (cursor.isNotEmpty()) "Project listing stopped: repeated cursor." else ""
                }
                "models/list" -> {
                    models = (models + readModels(result)).distinctBy { it.model }
                    val cursor = if (result.isNull("next_cursor")) "" else result.optString("next_cursor")
                    if (cursor.isNotEmpty() && modelCursors.add(cursor) && modelCursors.size <= 100) {
                        request("models/list", JSONObject().put("limit", 100).put("cursor", cursor))
                    } else modelCatalogStatus = if (cursor.isNotEmpty()) "Model list truncated. Custom IDs remain available." else if (models.isEmpty()) "No models advertised. Enter a custom model ID." else ""
                }
                "threads/list" -> {
                    if (error.startsWith("Host rejected threads/list")) error = ""
                    val list = result.optJSONArray("data") ?: JSONArray()
                    titles = titles + (0 until list.length()).associate { i -> val t = list.getJSONObject(i); t.getString("id") to sessionLabel(t) }
                    val paths = (0 until list.length()).associate { i -> val t = list.getJSONObject(i); t.getString("id") to t.optString("cwd") }
                    val membership = readProjectMembership(list)
                    projectMembership = if (operation.second == "more") projectMembership + membership else membership
                    threadWorkspaces = if (operation.second == "more") threadWorkspaces + paths else paths
                    val ids = (0 until list.length()).map { list.getJSONObject(it).getString("id") }
                    threads = if (operation.second == "more") (threads + ids).distinct() else ids
                    nextCursor = if (result.isNull("next_cursor")) null else result.optString("next_cursor").ifEmpty { null }
                    threadListLoading = false
                    nextCursor?.let { cursor ->
                        if (threadCursors.add(cursor)) more() else { nextCursor = null; error = "Session listing stopped because the host repeated a page cursor. Refresh to retry." }
                    }
                }
                "threads/start", "threads/resume", "threads/takeover" -> {
                    takeoverThread = ""
                    error = ""
                    sessionSettings = sessionSettingsLabel(result)
                    sessions.open(result.getJSONObject("thread").getString("id"))
                    ownedThread = selected; lastThread = selected
                    visibleHistory = historyCache.get(selected)
                    visibleHistory?.let { sessions.loadHistory(selected, it.entries, selected in runningThreads); hasOlderHistory = it.cursor.isNotBlank() }
                    val reported = ModelChoice(result.optString("model"), result.optString("effort"))
                    if (operation.first == "threads/start") {
                        val chosen = runCatching { readModelChoice(JSONObject(operation.second)) }.getOrNull()
                        if (chosen != null) modelChoices = modelChoices + (selected to chosen)
                    }
                    rememberChoice(selected, reported)
                    selectedWorkspace = result.getJSONObject("thread").optString("cwd")
                    threadWorkspaces = threadWorkspaces + (selected to selectedWorkspace)
                    threads = (listOf(selected) + threads).distinct()
                    if (operation.first == "threads/start") {
                        // Codex cannot read persisted turns until a new thread has its first turn.
                        historyLoading = false
                    } else loadHistory()
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
                "decision/respond" -> {
                    answeringDecisionIds -= operation.second
                    decisions = decisions.filterNot { it.optString("decision_id") == operation.second }
                }
                "turn/start" -> { result.optJSONObject("turn")?.let { sessions.acknowledgeTurn(operation.second, it) }; sentAttachments?.let { attachments.sent(it.first, it.second) } }
            }
            return
        }
        val params = frame.optJSONObject("params") ?: return
        sessions.event(frame.optString("event"), params)
        when (frame.optString("event")) {
            "decision/required" -> if (decisions.none { it.optString("decision_id") == params.optString("decision_id") }) decisions = decisions + params
            "error" -> if (params.optString("threadId") == selected) {
                turnFailure(params)?.let { error = it }
            }
            "turn/completed" -> {
                if (params.optString("threadId") == selected) {
                    turnFailure(params)?.let { error = it }
                }
                decisions = decisions.filterNot { it.optString("thread_id") == params.optString("threadId") && it.optString("turn_id") == params.optJSONObject("turn")?.optString("id") }
            }
        }
    }
    override fun onCleared() { transport?.close(); key?.fill(0) }
}
