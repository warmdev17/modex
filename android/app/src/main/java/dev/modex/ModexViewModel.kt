package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject

// State survives rotations, but not process death. No automatic command retries.
class ModexViewModel : ViewModel() {
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

    fun importKey(bytes: ByteArray) {
        require(!connected && !connecting && bytes.size in 1..(256 * 1024))
        key?.fill(0); key = bytes; keyLoaded = true
    }
    fun reset() {
        if (connected || connecting) return
        journal = JournalState(); config = null; pending.clear(); threads = emptyList()
        selected = ""; messages = emptyList(); decisions = emptyList(); busy = false; error = ""
    }
    fun connect(value: Connection) {
        if (connected || connecting) return
        if (key == null) { error = "Import an unencrypted SSH private key first."; return }
        if (config != null && config != value) { error = "Reset connection before changing the target."; return }
        config = value; error = ""; connecting = true
        val current = SshTransport(); transport = current
        viewModelScope.launch(Dispatchers.IO) {
            val copy = key!!.copyOf()
            try {
                current.open(value, copy, journal, onHello = {
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        connecting = false; connected = true
                        refreshThreads()
                    } }
                }, onFrame = { frame ->
                    kotlinx.coroutines.runBlocking { withContext(Dispatchers.Main) {
                        journal.frame(frame) { apply(it) }
                    } }
                })
            } catch (_: Exception) {
                withContext(Dispatchers.Main) { error = "Connection stopped. Check host fingerprint, key, host paths or replay gap. Reconnect to observe; do not resend uncertain prompts." }
            } finally {
                copy.fill(0)
                withContext(Dispatchers.Main) { connected = false; connecting = false }
            }
        }
    }
    fun disconnect() { transport?.close() }
    fun create(model: String) { if (busy || switching()) return; request("threads/start", JSONObject().apply { if (model.isNotBlank()) put("model", model) }) }
    fun select(id: String) {
        if (busy || switching()) return
        request("threads/resume", JSONObject().put("thread_id", id), id)
    }
    fun refreshThreads() { request("threads/list", JSONObject().put("limit", 50)) }
    fun more() { nextCursor?.let { request("threads/list", JSONObject().put("limit", 50).put("cursor", it), "more") } }
    fun send(text: String) {
        if (!connected || busy || switching() || selected.isEmpty() || text.isBlank()) return
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
        val command = JSONObject().put("id", id).put("method", method).put("params", params)
        viewModelScope.launch(Dispatchers.IO) {
            try { transport?.send(command) ?: error("Disconnected") }
            catch (_: Exception) { withContext(Dispatchers.Main) { error = "Delivery uncertain. Reconnect and observe before retrying." } }
        }
    }
    private fun apply(frame: JSONObject) {
        if (frame.has("id")) {
            val operation = pending.remove(frame.getLong("id")) ?: return
            if (frame.has("error")) { if (operation.first == "turn/start") busy = false; error = "Host rejected ${operation.first}. Review session state before retrying."; return }
            val result = frame.optJSONObject("result") ?: return
            when (operation.first) {
                "threads/list" -> {
                    val list = result.optJSONArray("data") ?: JSONArray()
                    val ids = (0 until list.length()).map { list.getJSONObject(it).getString("id") }
                    threads = if (operation.second == "more") (threads + ids).distinct() else ids
                    nextCursor = if (result.isNull("next_cursor")) null else result.optString("next_cursor").ifEmpty { null }
                }
                "threads/start", "threads/resume" -> {
                    error = ""
                    selected = result.getJSONObject("thread").getString("id")
                    threads = (listOf(selected) + threads).distinct(); messages = emptyList()
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
