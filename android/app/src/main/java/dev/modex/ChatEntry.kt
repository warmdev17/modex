package dev.modex

import org.json.JSONArray
import org.json.JSONObject

internal data class ChatEntry(
    val id: String = "", val role: String, val text: String = "", val command: String = "",
    val status: String = "", val exitCode: Int? = null, val cwd: String = "", val attachments: List<Attachment> = emptyList(),
    val turnId: String = "", val startedAtMs: Long? = null, val durationMs: Long? = null, val summaryParts: List<String> = emptyList(), val planHandled: Boolean = false, val notice: TurnNotice? = null,
    val activity: String = "",
) {
    fun toJson(): JSONObject = JSONObject().put("id", id).put("role", when (role) { "You" -> "user"; "Codex" -> "assistant"; else -> role })
        .put("turn_notice", notice?.toJson() ?: JSONObject.NULL).put("turn_id", turnId).put("started_at_ms", startedAtMs ?: JSONObject.NULL).put("duration_ms", durationMs ?: JSONObject.NULL).put("activity", activity)
        .put("plan_handled", planHandled).put("summary_parts", JSONArray(summaryParts)).put("attachments", JSONArray().apply { attachments.forEach { put(it.toJson().put("name", it.name).put("size", it.size)) } }).put("text", text).put("command", command).put("status", status).put("cwd", cwd).put("exit_code", exitCode ?: JSONObject.NULL)
}

private const val OUTPUT_LIMIT = 65536
private const val TRUNCATED = "\n[Output truncated at 64 KiB]"
private fun boundedOutput(text: String): String {
    val bytes = text.toByteArray(Charsets.UTF_8)
    return if (bytes.size <= OUTPUT_LIMIT) text else bytes.decodeToString(0, OUTPUT_LIMIT).trimEnd('\uFFFD') + TRUNCATED
}

internal fun readChat(list: JSONArray, now: Long = System.currentTimeMillis()): List<ChatEntry> = (0 until list.length()).map { i ->
    val m = list.getJSONObject(i)
    var files = m.optJSONArray("attachments")?.let { a -> (0 until a.length()).map { n ->
        val f = a.getJSONObject(n); Attachment(f.getString("path"), f.optString("name").ifBlank { attachmentName(f.getString("path")) }, f.getString("kind"), f.optLong("size"))
    } }.orEmpty()
    var body = if (m.has("display_text")) m.getString("display_text") else m.optString("text")
    if (m.optString("role") in listOf("user", "You") && files.isEmpty()) {
        val recovered = legacyFileAttachments(body); body = recovered.first; files = recovered.second
    }
    val role = when (val r = m.optString("role")) { "user" -> "You"; "assistant" -> "Codex"; else -> r }
    val entry = ChatEntry(m.optString("id"), role, if (role == "command") boundedOutput(m.optString("text")) else body,
        m.optString("command"), m.optString("status").ifBlank { m.optString("turn_status") }, if (m.isNull("exit_code")) null else m.optInt("exit_code"), m.optString("cwd"), files, m.optString("turn_id"),
        when { !m.isNull("started_at_ms") -> m.optLong("started_at_ms").takeIf { it >= 0 }; !m.isNull("elapsed_ms") -> (now - m.optLong("elapsed_ms").coerceAtLeast(0)).coerceAtLeast(0); else -> null },
        if (m.isNull("duration_ms")) null else m.optLong("duration_ms").takeIf { it >= 0 },
        m.optJSONArray("summary_parts")?.let { a -> (0 until a.length()).map { a.optString(it) } }.orEmpty(), m.optBoolean("plan_handled"), readTurnNotice(m.optJSONObject("turn_notice")), m.optString("activity"))
    val activityItem = m.optJSONObject("activity_item")
    if (activityItem != null) entry.withToolActivity(activityItem) else entry
}

private fun JSONObject.activityText(key: String): String =
    if (isNull(key)) "" else optString(key).trim().take(1024)

private fun ChatEntry.withToolActivity(item: JSONObject): ChatEntry = when (role) {
    "command" -> {
        val actions = item.optJSONArray("commandActions")
        val labels = (0 until minOf(actions?.length() ?: 0, 32)).mapNotNull { index ->
            val action = actions?.optJSONObject(index) ?: return@mapNotNull null
            val path = action.activityText("path")
            when (action.optString("type")) {
                "read" -> "Read " + action.activityText("name").ifBlank { path }
                "listFiles" -> if (path.isBlank()) "List files" else "List files in $path"
                "search" -> listOf("Search", action.activityText("query"), if (path.isBlank()) "" else "in $path")
                    .filter { it.isNotBlank() }.joinToString(" ")
                else -> ""
            }.trim().takeIf { it.isNotEmpty() }
        }
        copy(activity = labels.joinToString("\n").ifEmpty { activity })
    }
    "web" -> {
        val action = item.optJSONObject("action")
        val queries = action?.optJSONArray("queries")
        val query = (0 until minOf(queries?.length() ?: 0, 12)).mapNotNull { queries?.opt(it) as? String }
            .filter { it.isNotBlank() }.joinToString(", ").take(2048)
            .ifBlank { action?.activityText("query").orEmpty().ifBlank { item.activityText("query") } }
        val url = action?.activityText("url").orEmpty()
        val title = if (status == "inProgress") "Browsing the web" else when (action?.optString("type")) {
            "openPage" -> if (url.isBlank()) "Opened a web page" else "Opened $url"
            "findInPage" -> listOf("Searched a web page", action.activityText("pattern"), url).filter { it.isNotBlank() }.joinToString(" · ")
            else -> if (query.isBlank()) "Searched the web" else "Searched the web for $query"
        }
        copy(command = title)
    }
    "file_change" -> {
        val changes = item.optJSONArray("changes")
        if (changes == null) this else {
            val files = (0 until minOf(changes.length(), 128)).mapNotNull { changes.optJSONObject(it) }
            val paths = files.map { it.activityText("path") }.filter { it.isNotBlank() }
            val diff = files.joinToString("\n\n") { file ->
                val patch = file.optString("diff")
                if (files.size == 1) patch else "diff -- ${file.activityText("path")}\n$patch"
            }
            copy(command = paths.joinToString("\n"), text = boundedOutput(diff))
        }
    }
    else -> this
}

/** Apply only events already checked against the selected thread by the caller. */
internal fun updateChat(entries: List<ChatEntry>, event: String, params: JSONObject): List<ChatEntry> {
    val item = params.optJSONObject("item")
    val id = item?.optString("id") ?: params.optString("itemId")
    if (id.isBlank()) return entries
    val nativeTurn = params.optString("turnId")
    val found = entries.indexOfFirst { it.id == id }
    val index = if (found >= 0 || item?.optString("type") != "userMessage" || nativeTurn.isBlank()) found
        else entries.indexOfLast { it.role == "You" && it.turnId == nativeTurn && it.id.startsWith("local-") }
    val previous = entries.getOrNull(index)
    val turnId = params.optString("turnId").ifBlank { previous?.turnId.orEmpty() }
    val entry = when (event) {
        "item/plan/delta", "item/agentMessage/delta" -> {
            if (previous?.status == "completed") return entries
            (previous ?: ChatEntry(id, if (event == "item/plan/delta") "plan" else "Codex", turnId = turnId)).copy(text = (previous?.text ?: "") + params.optString("delta"), status = "inProgress")
        }
        "item/reasoning/summaryTextDelta", "item/reasoning/summaryPartAdded" -> {
            val part = params.optInt("summaryIndex", -1)
            if (part !in 0..127 || previous?.status == "completed") return entries
            val parts = previous?.summaryParts.orEmpty().toMutableList()
            while (parts.size <= part) parts.add("")
            if (event.endsWith("summaryTextDelta")) parts[part] = boundedOutput(parts[part] + params.optString("delta"))
            ChatEntry(id, "reasoning", parts.joinToString("\n\n"), status = "inProgress", turnId = turnId, summaryParts = parts)
        }
        "item/commandExecution/outputDelta" -> {
            if (previous?.role != "command") return entries
            previous.copy(text = if (previous.text.endsWith(TRUNCATED)) previous.text else boundedOutput(previous.text + params.optString("delta")))
        }
        "item/fileChange/patchUpdated" -> {
            if (previous != null && (previous.role != "file_change" || previous.status != "inProgress")) return entries
            (previous ?: ChatEntry(id, "file_change", status = "inProgress", turnId = turnId)).withToolActivity(params)
        }
        "item/started", "item/completed" -> when (item?.optString("type")) {
            "userMessage" -> {
                val content = item.optJSONArray("content")
                val body = content?.let { a -> (0 until a.length()).mapNotNull { n -> a.optJSONObject(n)?.takeIf { it.optString("type") == "text" }?.optString("text") }.joinToString("\n") }.orEmpty()
                val recovered = legacyFileAttachments(body)
                (previous ?: ChatEntry(id, "You", recovered.first, attachments = recovered.second, status = "inProgress"))
                    .copy(id = id, turnId = turnId)
            }
            "plan", "agentMessage" -> ChatEntry(id, if (item.optString("type") == "plan") "plan" else "Codex", if (item.has("text")) item.optString("text") else previous?.text.orEmpty(), status = if (event == "item/started") "inProgress" else "completed", turnId = turnId)
            "reasoning" -> {
                val parts = item.optJSONArray("summary")?.let { a -> (0 until a.length()).map { a.optString(it) } }.orEmpty().ifEmpty { previous?.summaryParts.orEmpty() }
                ChatEntry(id, "reasoning", parts.joinToString("\n\n"), status = if (event == "item/started") "inProgress" else "completed", turnId = turnId, summaryParts = parts)
            }
            "commandExecution" -> ChatEntry(id, "command",
                boundedOutput(if (item.has("aggregatedOutput") && !item.isNull("aggregatedOutput")) item.getString("aggregatedOutput") else previous?.text.orEmpty()),
                item.optString("command", previous?.command.orEmpty()), item.optString("status", if (event == "item/started") "inProgress" else "completed"),
                if (item.isNull("exitCode")) previous?.exitCode else item.optInt("exitCode"), item.optString("cwd", previous?.cwd.orEmpty()), turnId = turnId, activity = previous?.activity.orEmpty()).withToolActivity(item)
            "webSearch" -> ChatEntry(id, "web", status = if (event == "item/started") "inProgress" else "completed", turnId = turnId).withToolActivity(item)
            "fileChange" -> (previous ?: ChatEntry(id, "file_change", turnId = turnId))
                .copy(status = item.optString("status", if (event == "item/started") "inProgress" else "completed")).withToolActivity(item)
            else -> return entries
        }
        else -> return entries
    }
    return if (index < 0) entries + entry else entries.toMutableList().apply { set(index, entry) }
}
