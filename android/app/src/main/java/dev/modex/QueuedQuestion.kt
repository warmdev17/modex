package dev.modex

import org.json.JSONArray
import org.json.JSONObject
import org.json.JSONTokener

private const val replyTag = "send_user_message_question_reply"
private const val questionTool = "request_user_input_async"

internal data class QueuedQuestion(val id: String, val title: String, val options: List<String>) {
    fun toJson() = JSONObject().put("id", id).put("title", title).put("options", JSONArray(options))
}

internal data class QueuedAnswer(val id: String, val title: String, val answer: String)

internal fun readQueuedQuestions(itemId: String, questions: JSONArray?): List<QueuedQuestion> =
    (0 until minOf(questions?.length() ?: 0, 16)).mapNotNull { index ->
        val question = questions?.optJSONObject(index) ?: return@mapNotNull null
        if (itemId.isBlank() || itemId.length > 256) return@mapNotNull null
        val title = question.opt("title") as? String ?: return@mapNotNull null
        if (title.isBlank() || title.length > 4096) return@mapNotNull null
        val options = question.optJSONArray("options")
        val labels = (0 until minOf(options?.length() ?: 0, 16)).mapNotNull {
            (options?.opt(it) as? String)?.takeIf { label -> label.isNotBlank() && label.length <= 1024 }
        }.distinct()
        val id = JSONArray(listOf(questionTool, itemId, index)).toString()
        QueuedQuestion(id, title, labels)
    }

internal fun readQueuedReply(text: String): List<QueuedAnswer> {
    if (text.length > 131072) return emptyList()
    val trimmed = text.trim()
    val packet = if (trimmed.startsWith("# Context from my IDE setup:\n")) {
        trimmed.substringAfterLast("\n## My request for Codex:\n", "").trim()
    } else trimmed
    val open = "<$replyTag>"
    val close = "</$replyTag>"
    if (!packet.startsWith(open) || !packet.endsWith(close)) return emptyList()
    return runCatching {
        val payload = JSONTokener(packet.substring(open.length, packet.length - close.length))
        val items = when (val value = payload.nextValue()) {
            is JSONObject -> JSONArray().put(value)
            is JSONArray -> value
            else -> return emptyList()
        }
        require(payload.nextClean() == '\u0000' && items.length() in 1..16)
        (0 until items.length()).map { index ->
            val entry = items.getJSONObject(index)
            val id = JSONArray(entry.getString("questionItemId"))
            require(id.length() == 3 && id.getString(0) == questionTool)
            require(id.getString(1).isNotBlank() && id.getString(1).length <= 256 && id.getInt(2) in 0..63)
            val title = entry.get("question") as String
            val answer = entry.get("answer") as String
            require(title.isNotBlank() && answer.isNotBlank())
            QueuedAnswer(id.toString(), title, answer)
        }
    }.getOrDefault(emptyList())
}

internal fun queuedQuestionCards(entries: List<ChatEntry>): List<JSONObject> {
    val answered = entries.flatMap { it.queuedAnswerIds }.toSet()
    // A committed follow-up supersedes pending questions, as it does in the CLI composer.
    val followUp = entries.indexOfLast {
        it.role == "You" && it.id.isNotBlank() && !it.id.startsWith("local-") && it.queuedAnswerIds.isEmpty()
    }
    return entries.asSequence().drop(followUp + 1).mapNotNull { entry ->
        val questions = entry.queuedQuestions.filterNot { it.id in answered }
        if (questions.isEmpty()) return@mapNotNull null
        val form = JSONArray()
        questions.forEach { question ->
            form.put(JSONObject().put("id", question.id).put("question", question.title)
                .put("options", JSONArray(question.options.map { JSONObject().put("label", it) })))
        }
        JSONObject().put("decision_id", "queued:${entry.id}").put("method", "item/tool/requestUserInput")
            .put("context", JSONObject().put("questions", form))
    }.toList()
}

internal fun queuedQuestionReply(card: JSONObject, result: JSONObject): String? {
    val questions = card.optJSONObject("context")?.optJSONArray("questions") ?: return null
    val answers = result.optJSONObject("answers") ?: return null
    val packet = JSONArray()
    for (i in 0 until questions.length()) {
        val question = questions.getJSONObject(i)
        val id = question.getString("id")
        val values = answers.optJSONObject(id)?.optJSONArray("answers") ?: return null
        val answer = (values.opt(0) as? String)?.takeIf { values.length() == 1 && it.isNotBlank() && it.length <= 4096 } ?: return null
        packet.put(JSONObject().put("answer", answer).put("question", question.getString("question")).put("questionItemId", id))
    }
    if (packet.length() == 0 || packet.length() != answers.length()) return null
    return "<$replyTag>\n$packet\n</$replyTag>"
}
