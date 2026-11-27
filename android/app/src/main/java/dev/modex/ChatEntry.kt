package dev.modex

import org.json.JSONArray
import org.json.JSONObject

internal data class ChatEntry(
    val id: String = "", val role: String, val text: String = "", val command: String = "",
    val status: String = "", val exitCode: Int? = null, val cwd: String = "", val attachments: List<Attachment> = emptyList(),
    val turnId: String = "", val startedAtMs: Long? = null, val durationMs: Long? = null, val summaryParts: List<String> = emptyList(), val planHandled: Boolean = false, val notice: TurnNotice? = null,
    val activity: String = "", val agentActivity: AgentActivity? = null,
    val queuedQuestions: List<QueuedQuestion> = emptyList(), val queuedAnswerIds: List<String> = emptyList(),
) {
    fun toJson(): JSONObject = JSONObject().put("id", id).put("role", when (role) { "You" -> "user"; "Codex" -> "assistant"; else -> role })
        .put("activity_item", agentActivity?.toJson()).put("turn_notice", notice?.toJson() ?: JSONObject.NULL).put("turn_id", turnId).put("started_at_ms", startedAtMs ?: JSONObject.NULL).put("duration_ms", durationMs ?: JSONObject.NULL).put("activity", activity)
        .put("queued_questions", JSONArray(queuedQuestions.map { it.toJson() })).put("queued_answer_ids", JSONArray(queuedAnswerIds))
        .put("plan_handled", planHandled).put("summary_parts", JSONArray(summaryParts)).put("attachments", JSONArray().apply { attachments.forEach { put(it.toJson().put("name", it.name).put("size", it.size)) } }).put("text", text).put("command", command).put("status", status).put("cwd", cwd).put("exit_code", exitCode ?: JSONObject.NULL)
}

private const val OUTPUT_LIMIT = 65536
private const val TRUNCATED = "\n[Output truncated at 64 KiB]"
private fun boundedOutput(text: String): String {
    val bytes = text.toByteArray(Charsets.UTF_8)
    return if (bytes.size <= OUTPUT_LIMIT) text else bytes.decodeToString(0, OUTPUT_LIMIT).trimEnd('\uFFFD') + TRUNCATED
}

private val attachmentDrivePath = Regex("^[A-Za-z]:[\\\\/].*")

private fun isAttachmentPath(path: String): Boolean =
    path.length <= 32768 && path.none { it.isISOControl() } &&
        (path.startsWith("/") || attachmentDrivePath.matches(path))

private fun readChatAttachments(array: JSONArray?): List<Attachment> =
    (0 until minOf(array?.length() ?: 0, 32)).mapNotNull { index ->
        val item = array?.optJSONObject(index) ?: return@mapNotNull null
        val path = item.optString("path")
        val kind = item.optString("kind")
        if (!isAttachmentPath(path) || (kind != "image" && kind != "file")) return@mapNotNull null
        Attachment(path, attachmentName(item.optString("name").ifBlank { path }), kind, item.optLong("size"))
    }

internal fun readChat(list: JSONArray, now: Long = System.currentTimeMillis()): List<ChatEntry> = (0 until list.length()).map { i ->
    val m = list.getJSONObject(i)
    var files = readChatAttachments(m.optJSONArray("attachments"))
    var body = if (m.has("display_text")) m.getString("display_text") else m.optString("text")
    if (m.optString("role") in listOf("user", "You") && files.isEmpty()) {
        val recovered = legacyFileAttachments(body); body = recovered.first; files = recovered.second
    }
    val role = when (val r = m.optString("role")) { "user" -> "You"; "assistant" -> "Codex"; else -> r }
    val replies = if (role == "You") readQueuedReply(body) else emptyList()
    if (replies.isNotEmpty()) body = replies.joinToString("\n\n") { "${it.title}\n${it.answer}" }
    val entry = ChatEntry(m.optString("id"), role, if (role == "command") boundedOutput(m.optString("text")) else body,
        m.optString("command"), m.optString("status").ifBlank { m.optString("turn_status") }, if (m.isNull("exit_code")) null else m.optInt("exit_code"), m.optString("cwd"), files, m.optString("turn_id"),
        when { !m.isNull("started_at_ms") -> m.optLong("started_at_ms").takeIf { it >= 0 }; !m.isNull("elapsed_ms") -> (now - m.optLong("elapsed_ms").coerceAtLeast(0)).coerceAtLeast(0); else -> null },
        if (m.isNull("duration_ms")) null else m.optLong("duration_ms").takeIf { it >= 0 },
        m.optJSONArray("summary_parts")?.let { a -> (0 until a.length()).map { a.optString(it) } }.orEmpty(), m.optBoolean("plan_handled"), readTurnNotice(m.optJSONObject("turn_notice")), m.optString("activity"))
    val cached = m.optJSONArray("queued_questions")
    val questions = if (role != "Codex") emptyList() else if (cached == null) {
        readQueuedQuestions(entry.id, m.optJSONArray("questions"))
    } else {
        (0 until minOf(cached.length(), 16)).mapNotNull { index ->
            val row = cached.optJSONObject(index) ?: return@mapNotNull null
            readQueuedQuestions(entry.id, JSONArray().put(row)).firstOrNull()?.copy(id = row.optString("id"))
        }
    }
    val answerIds = m.optJSONArray("queued_answer_ids")
    val queued = entry.copy(queuedQuestions = questions, queuedAnswerIds = replies.map { it.id }.ifEmpty {
        (0 until minOf(answerIds?.length() ?: 0, 16)).mapNotNull { answerIds?.opt(it) as? String }
    })
    val activityItem = m.optJSONObject("activity_item")
    when {
        role == "agent_activity" -> entry.copy(agentActivity = AgentActivity.read(activityItem))
        activityItem != null -> entry.withToolActivity(activityItem)
        else -> queued
    }
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
                val replies = readQueuedReply(body)
                val recovered = legacyFileAttachments(body)
                val images = if (previous != null || content == null) emptyList() else (0 until content.length()).mapNotNull { n ->
                    val part = content.optJSONObject(n)?.takeIf { it.optString("type") == "localImage" } ?: return@mapNotNull null
                    val path = part.optString("path")
                    if (!isAttachmentPath(path)) return@mapNotNull null
                    Attachment(path, attachmentName(path), "image")
                }
                val files = readChatAttachments(item.optJSONArray("attachments")).ifEmpty { recovered.second + images }
                val user = (previous ?: ChatEntry(id, "You", recovered.first, attachments = files, status = "inProgress"))
                    .copy(id = id, turnId = turnId)
                if (replies.isEmpty()) user else user.copy(
                    text = replies.joinToString("\n\n") { "${it.title}\n${it.answer}" },
                    queuedAnswerIds = replies.map { it.id },
                )
            }
            "plan", "agentMessage" -> ChatEntry(id, if (item.optString("type") == "plan") "plan" else "Codex", if (item.has("text")) item.optString("text") else previous?.text.orEmpty(), status = if (event == "item/started") "inProgress" else "completed", turnId = turnId,
                queuedQuestions = if (item.optString("type") == "agentMessage") readQueuedQuestions(id, item.optJSONArray("questions")).ifEmpty { previous?.queuedQuestions.orEmpty() } else emptyList())
            "reasoning" -> {
                val parts = item.optJSONArray("summary")?.let { a -> (0 until a.length()).map { a.optString(it) } }.orEmpty().ifEmpty { previous?.summaryParts.orEmpty() }
                ChatEntry(id, "reasoning", parts.joinToString("\n\n"), status = if (event == "item/started") "inProgress" else "completed", turnId = turnId, summaryParts = parts)
            }
            "commandExecution" -> ChatEntry(id, "command",
                boundedOutput(if (item.has("aggregatedOutput") && !item.isNull("aggregatedOutput")) item.getString("aggregatedOutput") else previous?.text.orEmpty()),
                item.optString("command", previous?.command.orEmpty()), item.optString("status", if (event == "item/started") "inProgress" else "completed"),
                if (item.isNull("exitCode")) previous?.exitCode else item.optInt("exitCode"), item.optString("cwd", previous?.cwd.orEmpty()), turnId = turnId, activity = previous?.activity.orEmpty()).withToolActivity(item)
            "agentActivity" -> {
                if (turnId.isBlank() || (event == "item/started" && previous != null)) return entries
                val activity = AgentActivity.read(item) ?: return entries
                ChatEntry(id, "agent_activity", turnId = turnId, agentActivity = activity.copy(live = true))
            }
            "webSearch" -> ChatEntry(id, "web", status = if (event == "item/started") "inProgress" else "completed", turnId = turnId).withToolActivity(item)
            "fileChange" -> (previous ?: ChatEntry(id, "file_change", turnId = turnId))
                .copy(status = item.optString("status", if (event == "item/started") "inProgress" else "completed")).withToolActivity(item)
            else -> return entries
        }
        else -> return entries
    }
    return if (index < 0) entries + entry else entries.toMutableList().apply { set(index, entry) }
}
