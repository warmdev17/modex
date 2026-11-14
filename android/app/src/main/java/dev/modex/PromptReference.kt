package dev.modex

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import org.json.JSONArray
import org.json.JSONObject

internal data class PromptReference(
    val kind: String, val name: String, val path: String, val title: String,
    val description: String, val scope: String, val enabled: Boolean, val marker: String,
    val pluginId: String = ""
) {
    fun selectionJson() = JSONObject().put("kind", kind).put("name", name).put("path", path)
    fun toJson() = selectionJson().put("title", title).put("description", description).put("scope", scope)
        .put("enabled", enabled).put("marker", marker).put("plugin_id", pluginId)
}
internal data class ReferenceCatalog(val entries: List<PromptReference> = emptyList(), val supportedKinds: List<String> = emptyList(), val issues: Int = 0)

private fun readReference(value: JSONObject): PromptReference? {
    val kind = value.optString("kind"); val name = value.optString("name"); val path = value.optString("path")
    val marker = value.optString("marker").ifBlank { "\$$name" }
    if (kind.isBlank() || name.isBlank() || path.isBlank() || name.any { it.isWhitespace() || it.isISOControl() } || marker.any { it.isWhitespace() || it.isISOControl() }) return null
    return PromptReference(kind, name, path, value.optString("title").ifBlank { name }, value.optString("description"),
        value.optString("scope"), value.optBoolean("enabled"), marker, value.optString("plugin_id"))
}
internal fun readReferenceCatalog(value: JSONObject): ReferenceCatalog {
    val kinds = value.getJSONArray("supported_kinds").let { a -> (0 until a.length()).map { a.getString(it) } }
    val entries = value.getJSONArray("entries").let { a -> (0 until a.length()).mapNotNull { a.optJSONObject(it)?.let(::readReference) } }
    return ReferenceCatalog(entries.filter { it.kind in kinds }.distinctBy { it.kind to it.path }, kinds, value.optInt("issues").coerceAtLeast(0))
}
internal fun referenceDraftJson(values: List<PromptReference>): String = JSONArray().apply { values.forEach { put(it.toJson()) } }.toString()
internal fun readReferenceDraft(raw: String): List<PromptReference> = runCatching {
    val a = JSONArray(raw)
    (0 until a.length()).mapNotNull { a.optJSONObject(it)?.let(::readReference) }.take(8)
}.getOrDefault(emptyList())

internal class ReferenceCatalogState {
    var catalog by mutableStateOf(ReferenceCatalog()); private set
    var loading by mutableStateOf(false); private set
    var status by mutableStateOf(""); private set
    private var active: Pair<Long, String>? = null
    fun begin(id: Long, workspace: String) {
        if (active?.second != workspace) catalog = ReferenceCatalog()
        active = id to workspace; loading = true; status = ""
    }
    fun accept(id: Long, workspace: String, currentWorkspace: String, result: JSONObject): Boolean {
        if (active != (id to workspace) || workspace != currentWorkspace) return false
        loading = false
        return try { catalog = readReferenceCatalog(result); status = ""; true }
        catch (_: Exception) { status = "Skills could not be loaded. Refresh to try again."; false }
    }
    fun fail(id: Long, workspace: String, currentWorkspace: String) {
        if (active != (id to workspace) || workspace != currentWorkspace) return
        loading = false; status = "Skills unavailable. Check the host version and refresh."
    }
    fun clear() { active = null; catalog = ReferenceCatalog(); loading = false; status = "" }
}

internal data class ReferenceQuery(val start: Int, val end: Int, val query: String)
private fun referenceNameChar(c: Char) = c.isLetterOrDigit() || c in "_-.:/"
internal fun referenceQuery(text: String, cursor: Int, selectionEnd: Int = cursor): ReferenceQuery? {
    if (cursor != selectionEnd || cursor !in 0..text.length) return null
    val start = text.lastIndexOf('@', cursor - 1)
    if (start < 0 || (start > 0 && !text[start - 1].isWhitespace() && text[start - 1] !in "([{")) return null
    val query = text.substring(start + 1, cursor)
    if (!query.all(::referenceNameChar)) return null
    var end = cursor
    while (end < text.length && referenceNameChar(text[end])) end++
    return ReferenceQuery(start, end, query)
}
internal fun insertReference(text: String, query: ReferenceQuery, reference: PromptReference): Pair<String, Int> {
    val space = if (query.end < text.length && text[query.end].isWhitespace()) "" else " "
    val replacement = reference.marker + space
    val cursor = query.start + replacement.length + if (space.isEmpty()) 1 else 0
    return text.replaceRange(query.start, query.end, replacement) to cursor
}
private fun referenceRanges(text: String, marker: String): List<IntRange> {
    if (marker.isEmpty()) return emptyList()
    val ranges = mutableListOf<IntRange>()
    var at = text.indexOf(marker)
    while (at >= 0) {
        val end = at + marker.length
        val before = at == 0 || (!referenceNameChar(text[at - 1]) && text[at - 1] !in "@$")
        val after = end == text.length || !referenceNameChar(text[end]) ||
            (text[end] == '.' && (end + 1 == text.length || !referenceNameChar(text[end + 1])))
        if (before && after) ranges += at until end
        at = text.indexOf(marker, at + marker.length)
    }
    return ranges
}
internal fun referencesInText(text: String, references: List<PromptReference>) = references.filter { referenceRanges(text, it.marker).isNotEmpty() }
internal fun removeReferenceMarkers(text: String, reference: PromptReference): String = referenceRanges(text, reference.marker).asReversed().fold(text) { value, range -> value.removeRange(range) }
