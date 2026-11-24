package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import org.json.JSONObject

/** A read-only view of a child thread with its own history and live event scope. */
internal class AgentConversation {
    private val session = SessionState()
    var agent by mutableStateOf<AgentTimeline?>(null); private set
    val messages: List<ChatEntry> get() = session.messages
    val busy: Boolean get() = session.busy
    var cursor by mutableStateOf(""); private set
    var loading by mutableStateOf(false); private set
    var error by mutableStateOf(false); private set
    private var request: HistoryRequest? = null
    private val partialText = mutableSetOf<Pair<String, String>>()
    private data class HistoryRequest(val id: Long, val older: Boolean, val before: Set<ChatEntry>, val version: Long)

    fun open(target: AgentTimeline) {
        close()
        session.open(target.id)
        agent = target
    }

    fun close() {
        agent = null
        session.reset()
        cursor = ""
        loading = false
        error = false
        request = null
        partialText.clear()
    }

    fun begin(id: Long, older: Boolean) {
        request = HistoryRequest(id, older, messages.toSet(), session.activityVersion(session.selected))
        loading = true
        error = false
    }

    fun result(id: Long, result: JSONObject?): Boolean {
        val asked = request?.takeIf { it.id == id } ?: return false
        request = null
        loading = false
        val thread = session.selected
        val entries = result?.takeIf { it.optJSONObject("thread")?.optString("id") == thread }
            ?.optJSONArray("messages")
        val next = result?.let { if (it.isNull("next_cursor")) "" else it.optString("next_cursor") }.orEmpty()
        if (result == null || entries == null || (asked.older && next == cursor)) {
            error = true
            return true
        }
        val history = readChat(entries)
        val current = if (asked.older) messages else messages.filterNot {
            it in asked.before && (it.turnId to it.id) !in partialText
        }
        val stored = history.associateBy { Triple(it.turnId, it.id, it.role) }
        val fresh = current.mapNotNull { live ->
            val snapshot = stored[Triple(live.turnId, live.id, live.role)] ?: return@mapNotNull live
            val key = live.turnId to live.id
            when {
                live.status == "inProgress" && snapshot.status == "completed" -> {
                    partialText.remove(key)
                    null
                }
                partialText.remove(key) -> live.copy(text = appendStreamSuffix(snapshot.text, live.text))
                else -> live
            }
        }
        session.loadHistory(thread, mergeHistoryEntries(history, fresh), result.optBoolean("active_turn"),
            if (asked.older) -1 else asked.version, result.optString("active_turn_id"))
        cursor = next
        return true
    }

    fun event(method: String, params: JSONObject) {
        if (agent == null || params.optString("threadId") != session.selected) return
        val turn = params.optString("turnId")
        val item = params.optJSONObject("item")
        val key = turn to (item?.optString("id") ?: params.optString("itemId"))
        val existed = method in setOf("item/agentMessage/delta", "item/plan/delta") &&
            messages.any { it.turnId == turn && it.id == key.second }
        val before = messages
        session.event(method, params)
        if (messages === before) return
        when (method) {
            "item/agentMessage/delta", "item/plan/delta" -> if (!existed) partialText.add(key)
            "item/completed" -> partialText.remove(key)
        }
    }
}

// The snapshot and unpositioned deltas can overlap. The completed item replaces this preview.
private fun appendStreamSuffix(prefix: String, suffix: String): String {
    if (prefix.endsWith(suffix)) return prefix
    val overlap = (minOf(prefix.length, suffix.length, 1024) downTo 1).firstOrNull { size ->
        prefix.regionMatches(prefix.length - size, suffix, 0, size)
    } ?: 0
    return prefix + suffix.drop(overlap)
}
