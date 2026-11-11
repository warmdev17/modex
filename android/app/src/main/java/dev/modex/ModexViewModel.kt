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

internal fun explainHostRejection(method: String, detail: String): String {
    return when {
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
    private var saveJob: kotlinx.coroutines.Job? = null
    var connected by mutableStateOf(false); private set
    var connecting by mutableStateOf(false); private set
    var error by mutableStateOf(""); private set
    var threads by mutableStateOf(listOf<String>()); private set
    var selected by mutableStateOf(""); private set
    internal var messages by mutableStateOf(listOf<ChatEntry>()); private set
    var decisions by mutableStateOf(listOf<JSONObject>()); private set
    var answeringDecisionIds by mutableStateOf(setOf<String>()); private set
    var busy by mutableStateOf(false); private set
    var nextCursor by mutableStateOf<String?>(null); private set
    var sessionSettings by mutableStateOf(""); private set
    var sessionModel by mutableStateOf(""); private set
    internal var models by mutableStateOf(emptyList<ModelOption>()); private set
    var modelCatalogStatus by mutableStateOf(""); private set
    private val modelCursors = mutableSetOf<String>()
    var keyLoaded by mutableStateOf(false); private set
    private var key: ByteArray? = null
    private var config: Connection? = null
    private var journal = JournalState()
    private var transport: SshTransport? = null
    internal val attachments = AttachmentState(viewModelScope, { transport }, { config })
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
                selected = data.optString("selected")
                selectedWorkspace = data.optString("selected_workspace")
                messages = readChat(data.optJSONArray("messages") ?: JSONArray())
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
        val snapshot = JSONObject().put("connection", JSONObject().put("host", c.host).put("port", c.port).put("user", c.user).put("pin", c.pin).put("binary", c.binary).put("state", c.state))
            .put("key", Base64.encodeToString(privateKey, Base64.NO_WRAP)).put("instance", journal.instance.orEmpty()).put("cursor", journal.cursor).put("selected", selected)
            .put("selected_workspace", selectedWorkspace)
            .put("messages", JSONArray().apply { messages.forEach { put(it.toJson()) } })
            .put("decisions", JSONArray(decisions)).put("pending", JSONArray().apply { pending.forEach { (id, op) -> put(JSONObject().put("id", id).put("method", op.first).put("tag", op.second)) } })
        val revision = saved.revision()
        saveJob?.cancel()
        saveJob = viewModelScope.launch { kotlinx.coroutines.delay(150); withContext(Dispatchers.IO) {
            try { saved.save(snapshot, revision) } catch (_: Exception) { withContext(Dispatchers.Main) { error = "Could not save connection securely. Keep the app open or import again next time." } }
        } }
    }
    fun foreground() {
        if (reconnectAllowed && !connected && !connecting) savedConnection?.let { connect(it) }
    }
    fun forget() {
        attachments.clear(); sentAttachmentPaths.clear()
        sessionSettings = ""
        disconnect(); transport = null; connected = false; connecting = false; reconnectAllowed = false; saveJob?.cancel()
        saved.clear(); key?.fill(0); key = null; keyLoaded = false; savedConnection = null
        threadWorkspaces = emptyMap(); selectedWorkspace = ""; titles = emptyMap(); projects = emptyList(); projectMembership = emptyMap(); projectStatus = ""
        journal = JournalState(); config = null; savedConnection = null; pending.clear(); selected = ""; messages = emptyList(); decisions = emptyList(); answeringDecisionIds = emptySet(); threads = emptyList(); busy = false
    }
    private fun loadHistory() {
        if (selected.isBlank()) return
        historyLoading = true
        request("threads/read", withWorkspace(JSONObject().put("thread_id", selected).put("include_turns", true), selectedWorkspace), selected)
    }
    fun importKey(bytes: ByteArray) {
        require(!connected && !connecting && bytes.size in 1..(256 * 1024))
        key?.fill(0); key = bytes; keyLoaded = true
    }
    fun reset() {
        if (connected || connecting) return
        sessionSettings = ""; sessionModel = ""; threadListLoading = false; titles = emptyMap()
        journal = JournalState(); pending.clear(); sentAttachmentPaths.clear(); threads = emptyList()
        selected = ""; selectedWorkspace = ""; threadWorkspaces = emptyMap(); messages = emptyList(); decisions = emptyList(); answeringDecisionIds = emptySet(); busy = false; error = ""
        persist()
    }
    fun connect(value: Connection) {
        if (connected || connecting) return
        sessionSettings = ""; sessionModel = ""
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
                        request("models/list", JSONObject().put("limit", 100))
                        refreshThreads()
                        loadHistory()
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
                withContext(Dispatchers.Main) { if (transport === current) { connected = false; connecting = false; threadListLoading = false; getApplication<Application>().stopService(Intent(getApplication(), ConnectionService::class.java)) } }
            }
        }
    }
    fun disconnect() { sessionSettings = ""; sessionModel = ""; reconnectAllowed = false; transport?.close(); getApplication<Application>().stopService(Intent(getApplication(), ConnectionService::class.java)) }
    fun create(model: String, mode: String) {
        if (busy || historyLoading || switching()) return
        sessionSettings = ""; sessionModel = ""
        request("threads/start", withWorkspace(newThreadParams(model, mode), selectedWorkspace))
    }
    fun select(id: String) {
        if (busy || historyLoading || switching()) return
        sessionSettings = ""; sessionModel = ""
        request("threads/resume", withWorkspace(JSONObject().put("thread_id", id), threadWorkspaces[id].orEmpty()), id)
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
        if (!connected || busy || attachments.working || historyLoading || switching() || selected.isEmpty() || (text.isBlank() && files.isEmpty()) || !validModelId(model) || !validEffort(effort)) return false
        messages = messages + ChatEntry(role = "You", text = attachmentMessage(text, files)); busy = true
        val id = request("turn/start", withWorkspace(turnStartParams(selected, text, model, effort, files), selectedWorkspace), model.trim())
        if (id != null) sentAttachmentPaths[id] = selected to files.map { it.path }.toSet()
        return true
    }
    fun answer(decision: JSONObject, result: JSONObject) {
        if (!connected) return
        val id = decision.getString("decision_id")
        if (pending.values.any { it.first == "decision/respond" && it.second == id }) return
        answeringDecisionIds += id
        request("decision/respond", JSONObject().put("decision_id", id).put("result", result), id)
    }
    private fun switching() = pending.values.any { it.first == "threads/start" || it.first == "threads/resume" }
    private fun request(method: String, params: JSONObject, tag: String = ""): Long? {
        if (!connected) return null
        val id = sequence++; pending[id] = method to tag
        persist()
        val command = JSONObject().put("id", id).put("method", method).put("params", params)
        viewModelScope.launch(Dispatchers.IO) {
            try { transport?.send(command) ?: error("Disconnected") }
            catch (_: Exception) { withContext(Dispatchers.Main) { error = "Delivery uncertain. Reconnect and observe before retrying." } }
        }
        return id
    }
    private fun apply(frame: JSONObject) {
        if (frame.has("id")) {
            val id = frame.getLong("id")
            val sentAttachments = sentAttachmentPaths.remove(id)
            val operation = pending.remove(id) ?: return
            if (frame.has("error")) {
                if (operation.first == "projects/list") { projectStatus = "Project list unavailable. Update the host/Codex; chats remain in Recents."; return }
                if (operation.first == "models/list") { modelCatalogStatus = "Model list unavailable. You can enter a custom model ID."; return }
                if (operation.first == "turn/start") busy = false
                if (operation.first == "threads/read") historyLoading = false
                if (operation.first == "threads/list") threadListLoading = false
                if (operation.first == "decision/respond") answeringDecisionIds -= operation.second
                error = explainHostRejection(operation.first, frame.optJSONObject("error")?.optString("message").orEmpty()); return
            }
            val result = frame.optJSONObject("result") ?: return
            when (operation.first) {
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
                    val membership = (0 until list.length()).mapNotNull { i -> val t = list.getJSONObject(i); if (t.isNull("projectId")) null else t.getString("id") to t.optString("projectId") }.toMap()
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
                "threads/start", "threads/resume" -> {
                    error = ""
                    sessionSettings = sessionSettingsLabel(result)
                    sessionModel = result.optString("model")
                    selected = result.getJSONObject("thread").getString("id")
                    selectedWorkspace = result.getJSONObject("thread").optString("cwd")
                    threadWorkspaces = threadWorkspaces + (selected to selectedWorkspace)
                    threads = (listOf(selected) + threads).distinct()
                    if (operation.first == "threads/start") {
                        // Codex cannot read persisted turns until a new thread has its first turn.
                        messages = emptyList()
                        busy = false
                        historyLoading = false
                    } else loadHistory()
                }
                "threads/read" -> {
                    if (operation.second == selected) {
                        historyLoading = false
                        if (!result.has("messages")) {
                            error = "Update modex-host to load saved conversation history."
                        } else {
                            messages = readChat(result.getJSONArray("messages"))
                            busy = result.optBoolean("active_turn")
                        }
                    }
                }
                "decision/respond" -> {
                    answeringDecisionIds -= operation.second
                    decisions = decisions.filterNot { it.optString("decision_id") == operation.second }
                }
                "turn/start" -> { sentAttachments?.let { attachments.sent(it.first, it.second) }; if (operation.second.isNotBlank()) sessionModel = operation.second }
            }
            return
        }
        val params = frame.optJSONObject("params") ?: return
        when (frame.optString("event")) {
            "decision/required" -> if (decisions.none { it.optString("decision_id") == params.optString("decision_id") }) decisions = decisions + params
            "turn/started" -> if (params.optString("threadId") == selected) busy = true
            "item/started", "item/completed", "item/agentMessage/delta", "item/commandExecution/outputDelta" ->
                if (params.optString("threadId") == selected) messages = updateChat(messages, frame.getString("event"), params)
            "error" -> if (params.optString("threadId") == selected) {
                turnFailure(params)?.let { error = it }
            }
            "turn/completed" -> {
                if (params.optString("threadId") == selected) {
                    busy = false
                    turnFailure(params)?.let { error = it }
                }
                decisions = decisions.filterNot { it.optString("thread_id") == params.optString("threadId") && it.optString("turn_id") == params.optJSONObject("turn")?.optString("id") }
            }
        }
    }
    override fun onCleared() { transport?.close(); key?.fill(0) }
}
