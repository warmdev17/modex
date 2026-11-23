package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject

internal data class PromptDraft(val text: String = "", val references: List<PromptReference> = emptyList())

internal class PromptDrafts(private val store: SavedHost, private val scope: CoroutineScope) {
    private var job: Job? = null
    private var writable = true
    @Volatile private var revision = 0L
    @Volatile private var savedRevision = 0L
    var error by mutableStateOf("")
        private set

    private var drafts by mutableStateOf(load())

    private fun load(): Map<String, PromptDraft> = try {
        val data = store.load()
        buildMap {
            if (data != null) for (thread in data.keys()) {
                val draft = data.getJSONObject(thread)
                put(thread, PromptDraft(draft.getString("text"), readReferenceDraft(draft.optString("references"))))
            }
        }
    } catch (_: Exception) {
        writable = false
        error = "Could not unlock saved drafts on this device."
        emptyMap()
    }

    operator fun get(thread: String): PromptDraft? = drafts[thread]

    fun update(thread: String, draft: PromptDraft) {
        val next = draft.takeIf { it.text.isNotEmpty() }
        if (drafts[thread] == next) return
        drafts = if (next == null) drafts - thread else drafts + (thread to next)
        val requested = ++revision
        if (!writable) return
        job?.cancel()
        job = scope.launch {
            delay(200)
            val snapshot = drafts
            val success = withContext(Dispatchers.IO) { write(snapshot, requested) }
            if (requested == revision) showResult(success)
        }
    }

    fun flush() {
        if (!writable || savedRevision == revision) return
        job?.cancel()
        showResult(write(drafts, revision))
    }

    fun clear() {
        job?.cancel()
        drafts = emptyMap()
        val requested = ++revision
        val success = synchronized(this) {
            try {
                store.clear()
                savedRevision = requested
                true
            } catch (_: Exception) {
                false
            }
        }
        writable = success
        showResult(success)
    }

    @Synchronized
    private fun write(snapshot: Map<String, PromptDraft>, requested: Long): Boolean {
        if (requested != revision || requested == savedRevision) return true
        return try {
            store.save(JSONObject().apply {
                snapshot.forEach { (thread, draft) ->
                    put(thread, JSONObject().put("text", draft.text).put("references", referenceDraftJson(draft.references)))
                }
            })
            savedRevision = requested
            true
        } catch (_: Exception) {
            false
        }
    }

    private fun showResult(success: Boolean) {
        error = if (success) "" else "Could not save drafts on this device."
    }
}
