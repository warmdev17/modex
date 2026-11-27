package dev.modex

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class QueuedQuestionTest {
    @Test fun nativeQuestionAndReplyRoundTripThroughTranscriptCache() {
        val item = JSONObject("""{"id":"call_fixture","type":"agentMessage","text":"Choose a platform","questions":[{"title":"Platform?","options":["Android","Linux"]}]}""")
        val question = updateChat(emptyList(), "item/completed", JSONObject().put("item", item)).single()
        val pending = queuedQuestionCards(listOf(question)).single()
        val id = pending.getJSONObject("context").getJSONArray("questions").getJSONObject(0).getString("id")
        assertEquals(listOf("request_user_input_async", "call_fixture", 0), JSONArray(id).let { listOf(it.getString(0), it.getString(1), it.getInt(2)) })
        val answer = JSONObject().put("answers", JSONObject().put(id, JSONObject().put("answers", JSONArray().put("Android"))))
        val encoded = queuedQuestionReply(pending, answer)!!
        val desktop = """<send_user_message_question_reply>
            [{"answer":"Android","question":"Platform?","questionItemId":"[\"request_user_input_async\",\"call_fixture\",0]"}]
            </send_user_message_question_reply>""".trimIndent()
        assertEquals(readQueuedReply(desktop), readQueuedReply(encoded))
        val reply = JSONObject().put("id", "reply").put("type", "userMessage")
            .put("content", JSONArray().put(JSONObject().put("type", "text").put("text", encoded)))
        val live = updateChat(listOf(question), "item/completed", JSONObject().put("item", reply))
        assertEquals("Platform?\nAndroid", live.last().text)
        assertTrue(queuedQuestionCards(live).isEmpty())
        val restored = readChat(JSONArray(live.map { it.toJson() }))
        assertTrue(queuedQuestionCards(restored).isEmpty())
        assertEquals(live.last().text, restored.last().text)
        val hostHistory = readChat(JSONArray().put(JSONObject().put("id", "call_fixture")
            .put("role", "assistant").put("questions", item.getJSONArray("questions"))))
        assertEquals(id, hostHistory.single().queuedQuestions.single().id)
    }

    @Test fun malformedEnvelopeStaysPlainText() {
        val text = "<send_user_message_question_reply>plain text</send_user_message_question_reply>"
        val history = readChat(JSONArray().put(JSONObject().put("role", "user").put("text", text)))
        assertEquals(text, history.single().text)
    }

    @Test fun desktopAnswerWithIdeContextDismissesOnlyTheMatchingQuestion() {
        val history = readChat(JSONArray("""[
            {"id":"call_fixture","role":"assistant","questions":[{"title":"Same title?"},{"title":"Same title?"}]}
        ]"""))
        val desktop = """# Context from my IDE setup:
            |
            |## My request for Codex:
            |<send_user_message_question_reply>
            |{"answer":"Yes","question":"Same title?","questionItemId":"[\"request_user_input_async\",\"call_fixture\",0]"}
            |</send_user_message_question_reply>""".trimMargin()
        val reply = JSONObject().put("id", "reply").put("type", "userMessage")
            .put("content", JSONArray().put(JSONObject().put("type", "text").put("text", desktop)))
        val live = updateChat(history, "item/completed", JSONObject().put("item", reply))
        assertEquals("Same title?\nYes", live.last().text)
        val pending = queuedQuestionCards(readChat(JSONArray(live.map { it.toJson() }))).single()
            .getJSONObject("context").getJSONArray("questions")
        assertEquals(1, pending.length())
        assertEquals("""["request_user_input_async","call_fixture",1]""", pending.getJSONObject(0).getString("id"))
    }

    @Test fun committedFollowUpRetiresEarlierQuestionsWithoutHidingNewOnes() {
        val history = readChat(JSONArray("""[
            {"id":"old","role":"assistant","questions":[{"title":"Old?"}]},
            {"id":"follow-up","role":"user","text":"Ask me a new question."},
            {"id":"new","role":"assistant","questions":[{"title":"New?"}]}
        ]"""))
        assertEquals("queued:new", queuedQuestionCards(history).single().getString("decision_id"))
        val unsent = ChatEntry(id = "local-draft", role = "You", text = "New prompt", status = "failed")
        assertEquals("queued:new", queuedQuestionCards(history + unsent).single().getString("decision_id"))
    }
}
