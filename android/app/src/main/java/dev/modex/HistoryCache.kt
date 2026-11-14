package dev.modex

import org.json.JSONArray
import org.json.JSONObject

internal data class CachedHistory(val entries: List<ChatEntry>, val cursor: String, val revision: String) {
    fun toJson() = JSONObject().put("messages", JSONArray().apply { entries.forEach { put(it.toJson()) } }).put("cursor", cursor).put("revision", revision)
}

/** Cached display data never represents an acquired remote writer. */
internal class HistoryCache(private val maxSessions: Int = 10, private val maxBytes: Int = 4 * 1024 * 1024) {
    private val sessions = linkedMapOf<String, CachedHistory>()
    fun get(id: String): CachedHistory? = sessions[id]
    fun clear() = sessions.clear()
    fun latest(id: String, entries: List<ChatEntry>, cursor: String, revision: String): CachedHistory {
        val old = sessions[id]
        val page = if (revision.isNotBlank() && old?.revision == revision) old else CachedHistory(entries, cursor, revision)
        remember(id, page)
        return page
    }
    fun remember(id: String, page: CachedHistory) {
        sessions.remove(id)
        if (JSONObject().put(id, page.toJson()).toString().toByteArray(Charsets.UTF_8).size > maxBytes) return
        sessions[id] = page
        while (sessions.isNotEmpty() && (sessions.size > maxSessions || toJson().toString().toByteArray(Charsets.UTF_8).size > maxBytes)) sessions.remove(sessions.keys.first())
    }
    fun toJson() = JSONObject().apply {
        sessions.forEach { (id, page) -> put(id, page.toJson()) }
    }
    fun restore(data: JSONObject?) {
        clear()
        data?.keys()?.asSequence()?.toList()?.takeLast(maxSessions)?.forEach { id ->
            runCatching {
                val page = data.getJSONObject(id)
                remember(id, CachedHistory(readChat(page.getJSONArray("messages")), page.optString("cursor"), page.optString("revision")))
            }
        }
    }
}


internal fun mergeHistoryEntries(older: List<ChatEntry>, newer: List<ChatEntry>): List<ChatEntry> {
    fun key(entry: ChatEntry) = if (entry.id.isNotBlank()) "${entry.turnId}/${entry.role}/${entry.id}" else entry.toJson().toString()
    val newest = newer.associateBy(::key)
    val seen = mutableSetOf<String>()
    return (older + newer).mapNotNull { entry -> val k = key(entry); if (seen.add(k)) newest[k] ?: entry else null }
}
