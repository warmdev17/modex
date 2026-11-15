package dev.modex

import org.json.JSONArray
import org.junit.Assert.*
import org.junit.Test

class QuestionAnswerTest {
    private val questions = JSONArray("""[
        {"id":"approach","question":"How?","options":[{"label":"Safe"},{"label":"Fast"}]},
        {"id":"detail","question":"Any details?"}
    ]""")

    @Test fun requiresEveryQuestionAndRejectsBlankCustomText() {
        assertNull(questionAnswerResult(questions, emptyMap(), emptyMap()))
        assertNull(questionAnswerResult(questions, mapOf("approach" to 0), mapOf("detail" to "  \n")))
        assertNull(questionAnswerResult(questions, mapOf("approach" to -1), mapOf("detail" to "details")))
        assertNull(questionAnswerResult(questions, mapOf("approach" to 99), mapOf("detail" to "details")))
        assertNull(questionAnswerResult(JSONArray(), emptyMap(), emptyMap()))
    }

    @Test fun selectedOptionWinsOverRetainedCustomDraftAndOnlyRequestedIdsAreSent() {
        val result = questionAnswerResult(questions, mapOf("approach" to 1),
            mapOf("approach" to "old custom draft", "detail" to "keep history", "stale" to "do not send"))!!
        val answers = result.getJSONObject("answers")
        assertEquals(2, answers.length())
        assertEquals("Fast", answers.getJSONObject("approach").getJSONArray("answers").getString(0))
        assertEquals("keep history", answers.getJSONObject("detail").getJSONArray("answers").getString(0))
    }

    @Test fun otherAndPlainTextUseTheirOwnDraftsWithoutMutatingThem() {
        val drafts = mapOf("approach" to " My own approach ", "detail" to "Line 1\nLine 2")
        val result = questionAnswerResult(questions, mapOf("approach" to -1), drafts)!!.getJSONObject("answers")
        assertEquals(drafts["approach"], result.getJSONObject("approach").getJSONArray("answers").getString(0))
        assertEquals(drafts["detail"], result.getJSONObject("detail").getJSONArray("answers").getString(0))
    }
}
