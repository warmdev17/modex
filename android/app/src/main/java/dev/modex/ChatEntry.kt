package dev.modex

import org.json.JSONArray
import org.json.JSONObject

internal data class ChatEntry(
    val id: String = "", val role: String, val text: String = "", val command: String = "",
    val status: String = "", val exitCode: Int? = null, val cwd: String = ""
) {
    fun toJson(): JSONObject = JSONObject().put("id", id).put("role", when (role) { "You" -> "user"; "Codex" -> "assistant"; else -> role })
        .put("text", text).put("command", command).put("status", status).put("cwd", cwd).put("exit_code", exitCode ?: JSONObject.NULL)
}

private const val OUTPUT_LIMIT = 65536
private const val TRUNCATED = "\n[Output truncated at 64 KiB]"
private fun boundedOutput(text: String): String {
    val bytes = text.toByteArray(Charsets.UTF_8)
    return if (bytes.size <= OUTPUT_LIMIT) text else bytes.decodeToString(0, OUTPUT_LIMIT).trimEnd('\uFFFD') + TRUNCATED
}

internal fun readChat(list: JSONArray): List<ChatEntry> = (0 until list.length()).map { i ->
    val m = list.getJSONObject(i)
    val role = when (val r = m.optString("role")) { "user" -> "You"; "assistant" -> "Codex"; else -> r }
    ChatEntry(m.optString("id"), role, if (role == "command") boundedOutput(m.optString("text")) else m.optString("text"),
        m.optString("command"), m.optString("status"), if (m.isNull("exit_code")) null else m.optInt("exit_code"), m.optString("cwd"))
}

/** Apply only events already checked against the selected thread by the caller. */
internal fun updateChat(entries: List<ChatEntry>, event: String, params: JSONObject): List<ChatEntry> {
    val item = params.optJSONObject("item")
    val id = item?.optString("id") ?: params.optString("itemId")
    if (id.isBlank()) return entries
    val index = entries.indexOfFirst { it.id == id }
    val previous = entries.getOrNull(index)
    val entry = when (event) {
        "item/agentMessage/delta" -> (previous ?: ChatEntry(id, "Codex")).copy(text = (previous?.text ?: "") + params.optString("delta"))
        "item/commandExecution/outputDelta" -> {
            if (previous?.role != "command") return entries
            previous.copy(text = if (previous.text.endsWith(TRUNCATED)) previous.text else boundedOutput(previous.text + params.optString("delta")))
        }
        "item/started", "item/completed" -> when (item?.optString("type")) {
            "agentMessage" -> ChatEntry(id, "Codex", item.optString("text").ifEmpty { previous?.text.orEmpty() })
            "commandExecution" -> ChatEntry(id, "command",
                boundedOutput(if (item.has("aggregatedOutput") && !item.isNull("aggregatedOutput")) item.getString("aggregatedOutput") else previous?.text.orEmpty()),
                item.optString("command", previous?.command.orEmpty()), item.optString("status", if (event == "item/started") "inProgress" else "completed"),
                if (item.isNull("exitCode")) previous?.exitCode else item.optInt("exitCode"), item.optString("cwd", previous?.cwd.orEmpty()))
            else -> return entries
        }
        else -> return entries
    }
    return if (index < 0) entries + entry else entries.toMutableList().apply { set(index, entry) }
}
