package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import org.json.JSONObject

/** Keep navigation separate from worker activity; history stays authoritative on the host. */
internal class SessionState(private val now: () -> Long = System::currentTimeMillis) {
    var selected by mutableStateOf(""); private set
    var messages by mutableStateOf(emptyList<ChatEntry>()); private set
    private var activeTurns by mutableStateOf(emptyMap<String, String>())
    private var stoppingTurns by mutableStateOf(emptyMap<String, String>())
    val stopping: Boolean get() = selected in stoppingTurns
    private val activityVersions = mutableMapOf<String, Long>()
    val runningThreads: Set<String> get() = activeTurns.keys
    val busy: Boolean get() = selected in activeTurns

    fun clearSelection() { selected = ""; messages = emptyList() }
    fun open(id: String) { clearSelection(); selected = id }
    fun reset() { clearSelection(); activeTurns = emptyMap(); stoppingTurns = emptyMap(); activityVersions.clear() }
    fun activityVersion(id: String): Long = activityVersions[id] ?: 0
    private fun changed(id: String) { activityVersions[id] = activityVersion(id) + 1 }

    fun loadHistory(id: String, entries: List<ChatEntry>, active: Boolean, expectedActivityVersion: Long? = null, activeTurnId: String = "") {
        if (id != selected) return
        messages = entries
        // An older history response cannot undo a newer live turn event.
        if (expectedActivityVersion == null || expectedActivityVersion == activityVersion(id)) {
            if (!active || (activeTurnId.isNotBlank() && stoppingTurns[id] != activeTurnId)) stoppingTurns = stoppingTurns - id
            activeTurns = if (active) activeTurns + (id to activeTurnId.ifBlank { activeTurns[id].orEmpty() }) else activeTurns - id
        }
    }

    fun sent(entry: ChatEntry) {
        check(selected.isNotBlank() && !busy)
        messages = messages + entry.copy(id = entry.id.ifBlank { "local-${java.util.UUID.randomUUID()}" }, status = "inProgress", startedAtMs = now())
        activeTurns = activeTurns + (selected to "")
        changed(selected)
    }

    fun rejectTurn(id: String) {
        if (activeTurns[id] == "") {
            activeTurns = activeTurns - id; changed(id)
            if (id == selected) messages = messages.map { if (it.role == "You" && it.status == "inProgress" && it.turnId.isBlank()) it.copy(status = "failed", durationMs = it.startedAtMs?.let { start -> (now() - start).coerceAtLeast(0) }) else it }
        }
    }

    fun requestStop(): JSONObject? {
        val turn = activeTurns[selected].orEmpty()
        if (turn.isBlank() || stopping) return null
        stoppingTurns = stoppingTurns + (selected to turn)
        return JSONObject().put("thread_id", selected).put("turn_id", turn)
    }

    fun interruptRejected(id: String, turn: String) {
        if (stoppingTurns[id] == turn) stoppingTurns = stoppingTurns - id
    }

    fun acknowledgeTurn(id: String, turn: JSONObject) {
        if (activeTurns[id] == "") event("turn/started", JSONObject().put("threadId", id).put("turn", turn))
    }

    fun event(method: String, params: JSONObject) {
        val id = params.optString("threadId")
        if (id.isBlank()) return
        val turn = params.optJSONObject("turn")?.optString("id").orEmpty()
        when (method) {
            "turn/started" -> {
                if (turn.isBlank()) return
                activeTurns = activeTurns + (id to turn)
                if (id == selected) {
                    val index = messages.indexOfLast { it.role == "You" && it.status == "inProgress" && it.turnId.isBlank() }
                    if (index >= 0) messages = messages.toMutableList().apply { this[index] = this[index].copy(turnId = turn) }
                }
                changed(id)
            }
            "turn/completed" -> {
                if (turn.isBlank()) return
                if (activeTurns[id].isNullOrEmpty() || activeTurns[id] == turn) {
                    activeTurns = activeTurns - id
                    interruptRejected(id, turn)
                    if (id == selected) {
                        val status = params.optJSONObject("turn")?.optString("status").orEmpty().ifBlank { "completed" }
                        val reported = params.optJSONObject("turn")?.takeUnless { it.isNull("durationMs") }?.optLong("durationMs")?.takeIf { it >= 0 }
                        messages = messages.map { entry ->
                            if (entry.turnId == turn) entry.copy(status = if (entry.role == "You") status else if (entry.status == "inProgress") "completed" else entry.status,
                                durationMs = if (entry.role == "You") reported ?: entry.startedAtMs?.let { (now() - it).coerceAtLeast(0) } else entry.durationMs) else entry
                        }
                    }
                    changed(id)
                }
            }
            "item/started", "item/completed", "item/agentMessage/delta", "item/commandExecution/outputDelta", "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded" ->
                if (id == selected) {
                    messages = updateChat(messages, method, params.apply { if (optString("turnId").isBlank()) put("turnId", activeTurns[id].orEmpty()) })
                    changed(id)
                }
        }
    }

    fun activityJson(): JSONObject = JSONObject().apply { activeTurns.forEach { (id, turn) -> put(id, turn) } }
    fun restoreActivity(data: JSONObject?) {
        activeTurns = data?.keys()?.asSequence()?.filter { it.isNotBlank() }?.mapNotNull { id ->
            (data.opt(id) as? String)?.let { id to it }
        }?.toMap().orEmpty()
        stoppingTurns = emptyMap()
        activityVersions.clear()
    }
}
