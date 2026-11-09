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
    var titles by mutableStateOf(mapOf<String, String>()); private set
    var historyLoading by mutableStateOf(false); private set
    private var saveJob: kotlinx.coroutines.Job? = null
    var connected by mutableStateOf(false); private set
    var connecting by mutableStateOf(false); private set
    var error by mutableStateOf(""); private set
    var threads by mutableStateOf(listOf<String>()); private set
    var selected by mutableStateOf(""); private set
    var messages by mutableStateOf(listOf<Pair<String, String>>()); private set
    var decisions by mutableStateOf(listOf<JSONObject>()); private set
    var busy by mutableStateOf(false); private set
    var nextCursor by mutableStateOf<String?>(null); private set
    var keyLoaded by mutableStateOf(false); private set
    private var key: ByteArray? = null
    private var config: Connection? = null
    private var journal = JournalState()
    private var transport: SshTransport? = null
    // Avoid matching responses retained from an earlier app process.
    private var sequence = java.security.SecureRandom().nextLong().ushr(12) + 1
    private val pending = mutableMapOf<Long, Pair<String, String>>()

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
                messages = readMessages(data.optJSONArray("messages") ?: JSONArray())
                decisions = data.optJSONArray("decisions")?.let { list -> (0 until list.length()).map { list.getJSONObject(it) } } ?: emptyList()
                val requests = data.optJSONArray("pending") ?: JSONArray()
                for (i in 0 until requests.length()) { val r = requests.getJSONObject(i); pending[r.getLong("id")] = r.getString("method") to r.getString("tag") }
            }
        } catch (_: Exception) { error = "Saved host could not be unlocked. Forget it and import the SSH key again." }
    }
    private fun persist() {
        val c = config ?: return
        val privateKey = key ?: return
        val snapshot = JSONObject().put("connection", JSONObject().put("host", c.host).put("port", c.port).put("user", c.user).put("pin", c.pin).put("binary", c.binary).put("state", c.state))
            .put("key", Base64.encodeToString(privateKey, Base64.NO_WRAP)).put("instance", journal.instance.orEmpty()).put("cursor", journal.cursor).put("selected", selected)
            .put("messages", JSONArray().apply { messages.forEach { put(JSONObject().put("role", if (it.first == "You") "user" else "assistant").put("text", it.second)) } })
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
        disconnect(); transport = null; connected = false; connecting = false; reconnectAllowed = false; saveJob?.cancel()
        saved.clear(); key?.fill(0); key = null; keyLoaded = false; savedConnection = null
        journal = JournalState(); config = null; savedConnection = null; pending.clear(); selected = ""; messages = emptyList(); decisions = emptyList(); threads = emptyList(); busy = false
    }
    private fun readMessages(list: JSONArray) = (0 until list.length()).map { list.getJSONObject(it).let { m -> (if (m.getString("role") == "user") "You" else "Codex") to m.getString("text") } }
    private fun loadHistory() {
        if (selected.isBlank()) return
        historyLoading = true
        request("threads/read", JSONObject().put("thread_id", selected).put("include_turns", true), selected)
    }
    fun importKey(bytes: ByteArray) {
        require(!connected && !connecting && bytes.size in 1..(256 * 1024))
        key?.fill(0); key = bytes; keyLoaded = true
    }
    fun reset() {
        if (connected || connecting) return
        journal = JournalState(); pending.clear(); threads = emptyList()
        selected = ""; messages = emptyList(); decisions = emptyList(); busy = false; error = ""
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
                        connecting = false; connected = true
                        refreshThreads()
                        loadHistory()
                    } }
                }, onFrame = { frame ->
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        check(transport === current)
                        journal.frame(frame) { apply(it) }; persist()
                    } }
                })
            } catch (_: Exception) {
                withContext(Dispatchers.Main) { if (transport === current) error = "Connection stopped. Check host fingerprint, key, host paths or replay gap. Reconnect to observe; do not resend uncertain prompts." }
            } finally {
                copy.fill(0)
                withContext(Dispatchers.Main) { if (transport === current) { connected = false; connecting = false; getApplication<Application>().stopService(Intent(getApplication(), ConnectionService::class.java)) } }
            }
        }
    }
    fun disconnect() { reconnectAllowed = false; transport?.close(); getApplication<Application>().stopService(Intent(getApplication(), ConnectionService::class.java)) }
    fun create(model: String) { if (busy || historyLoading || switching()) return; request("threads/start", JSONObject().apply { if (model.isNotBlank()) put("model", model) }) }
    fun select(id: String) {
        if (busy || historyLoading || switching()) return
        request("threads/resume", JSONObject().put("thread_id", id), id)
    }
    fun refreshThreads() { request("threads/list", JSONObject().put("limit", 50)) }
    fun more() { nextCursor?.let { request("threads/list", JSONObject().put("limit", 50).put("cursor", it), "more") } }
    fun send(text: String) {
        if (!connected || busy || historyLoading || switching() || selected.isEmpty() || text.isBlank()) return
        messages = messages + ("You" to text); busy = true
        request("turn/start", JSONObject().put("thread_id", selected).put("text", text))
    }
    fun answer(decision: JSONObject, result: JSONObject) {
        val id = decision.getString("decision_id")
        if (pending.values.any { it.first == "decision/respond" && it.second == id }) return
        request("decision/respond", JSONObject().put("decision_id", id).put("result", result), id)
    }
    private fun switching() = pending.values.any { it.first == "threads/start" || it.first == "threads/resume" }
    private fun request(method: String, params: JSONObject, tag: String = "") {
        if (!connected) return
        val id = sequence++; pending[id] = method to tag
        persist()
        val command = JSONObject().put("id", id).put("method", method).put("params", params)
        viewModelScope.launch(Dispatchers.IO) {
            try { transport?.send(command) ?: error("Disconnected") }
            catch (_: Exception) { withContext(Dispatchers.Main) { error = "Delivery uncertain. Reconnect and observe before retrying." } }
        }
    }
    private fun apply(frame: JSONObject) {
        if (frame.has("id")) {
            val operation = pending.remove(frame.getLong("id")) ?: return
            if (frame.has("error")) { if (operation.first == "turn/start") busy = false; if (operation.first == "threads/read") historyLoading = false; error = explainHostRejection(operation.first, frame.optJSONObject("error")?.optString("message").orEmpty()); return }
            val result = frame.optJSONObject("result") ?: return
            when (operation.first) {
                "threads/list" -> {
                    if (error.startsWith("Host rejected threads/list")) error = ""
                    val list = result.optJSONArray("data") ?: JSONArray()
                    titles = titles + (0 until list.length()).associate { i -> val t = list.getJSONObject(i); t.getString("id") to t.optString("preview").ifBlank { t.getString("id") } }
                    val ids = (0 until list.length()).map { list.getJSONObject(it).getString("id") }
                    threads = if (operation.second == "more") (threads + ids).distinct() else ids
                    nextCursor = if (result.isNull("next_cursor")) null else result.optString("next_cursor").ifEmpty { null }
                }
                "threads/start", "threads/resume" -> {
                    error = ""
                    selected = result.getJSONObject("thread").getString("id")
                    threads = (listOf(selected) + threads).distinct()
                    loadHistory()
                }
                "threads/read" -> {
                    if (operation.second == selected) {
                        historyLoading = false
                        if (!result.has("messages")) {
                            error = "Update modex-host to load saved conversation history."
                        } else {
                            messages = readMessages(result.getJSONArray("messages"))
                            busy = result.optBoolean("active_turn")
                        }
                    }
                }
                "decision/respond" -> decisions = decisions.filterNot { it.optString("decision_id") == operation.second }
            }
            return
        }
        val params = frame.optJSONObject("params") ?: return
        when (frame.optString("event")) {
            "decision/required" -> if (decisions.none { it.optString("decision_id") == params.optString("decision_id") }) decisions = decisions + params
            "turn/started" -> if (params.optString("threadId") == selected) busy = true
            "item/agentMessage/delta" -> if (params.optString("threadId") == selected) {
                val text = params.optString("delta")
                messages = if (messages.lastOrNull()?.first == "Codex") messages.dropLast(1) + ("Codex" to (messages.last().second + text)) else messages + ("Codex" to text)
            }
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
