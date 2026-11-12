package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import org.json.JSONObject

/** Keep navigation separate from worker activity; history stays authoritative on the host. */
internal class SessionState {
    var selected by mutableStateOf(""); private set
    var messages by mutableStateOf(emptyList<ChatEntry>()); private set
    private var activeTurns by mutableStateOf(emptyMap<String, String>())
    private val activityVersions = mutableMapOf<String, Long>()
    val runningThreads: Set<String> get() = activeTurns.keys
    val busy: Boolean get() = selected in activeTurns

    fun clearSelection() { selected = ""; messages = emptyList() }
    fun open(id: String) { clearSelection(); selected = id }
    fun reset() { clearSelection(); activeTurns = emptyMap(); activityVersions.clear() }
    fun activityVersion(id: String): Long = activityVersions[id] ?: 0
    private fun changed(id: String) { activityVersions[id] = activityVersion(id) + 1 }

    fun loadHistory(id: String, entries: List<ChatEntry>, active: Boolean, expectedActivityVersion: Long? = null) {
        if (id != selected) return
        messages = entries
        // An older history response cannot undo a newer live turn event.
        if (expectedActivityVersion == null || expectedActivityVersion == activityVersion(id)) {
            activeTurns = if (active) activeTurns + (id to activeTurns[id].orEmpty()) else activeTurns - id
        }
    }

    fun sent(entry: ChatEntry) {
        check(selected.isNotBlank() && !busy)
        messages = messages + entry
        activeTurns = activeTurns + (selected to "")
        changed(selected)
    }

    fun rejectTurn(id: String) {
        if (activeTurns[id] == "") { activeTurns = activeTurns - id; changed(id) }
    }

    fun event(method: String, params: JSONObject) {
        val id = params.optString("threadId")
        if (id.isBlank()) return
        val turn = params.optJSONObject("turn")?.optString("id").orEmpty()
        when (method) {
            "turn/started" -> {
                if (turn.isBlank()) return
                activeTurns = activeTurns + (id to turn)
                changed(id)
            }
            "turn/completed" -> {
                if (turn.isBlank()) return
                if (activeTurns[id].isNullOrEmpty() || activeTurns[id] == turn) {
                    activeTurns = activeTurns - id
                    changed(id)
                }
            }
            "item/started", "item/completed", "item/agentMessage/delta", "item/commandExecution/outputDelta" ->
                if (id == selected) messages = updateChat(messages, method, params)
        }
    }

    fun activityJson(): JSONObject = JSONObject().apply { activeTurns.forEach { (id, turn) -> put(id, turn) } }
    fun restoreActivity(data: JSONObject?) {
        activeTurns = data?.keys()?.asSequence()?.filter { it.isNotBlank() }?.mapNotNull { id ->
            (data.opt(id) as? String)?.let { id to it }
        }?.toMap().orEmpty()
        activityVersions.clear()
    }
}
