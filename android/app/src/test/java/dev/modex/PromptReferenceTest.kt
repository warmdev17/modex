package dev.modex

import org.json.JSONObject
import org.json.JSONArray
import org.junit.Assert.*
import org.junit.Test

class PromptReferenceTest {
    private val skill = PromptReference("skill", "review", "/skills/review/SKILL.md", "Review changes", "Review a diff", "user", true, "\$review")

    @Test fun mentionQueryUsesCursorAndIgnoresEmailsOrSelections() {
        assertNull(referenceQuery("mail me@review", 14))
        assertNull(referenceQuery("@@review", 8))
        assertNull(referenceQuery("@review", 7, 0))
        val query = referenceQuery("Hi @rev and suffix", 7)!!
        assertEquals("rev", query.query)
        assertEquals(3, query.start)
        assertEquals(7, query.end)
        assertEquals("", referenceQuery("Hi @", 4)!!.query)
    }

    @Test fun selectionReplacesWholeTokenAndPreservesSuffix() {
        val text = "Hi @review later"
        val query = referenceQuery(text, 7)!!
        val inserted = insertReference(text, query, skill)
        assertEquals("Hi \$review later", inserted.first)
        assertEquals(11, inserted.second)
    }

    @Test fun bindingsRequireExactMarkersAndRemovalCannotImplicitlyInvokeSkill() {
        assertEquals(listOf(skill), referencesInText("Use \$review, please", listOf(skill)))
        assertEquals(listOf(skill), referencesInText("Use \$review.", listOf(skill)))
        assertTrue(referencesInText("Use \$review-other", listOf(skill)).isEmpty())
        assertTrue(referencesInText("Use \$review.other", listOf(skill)).isEmpty())
        assertEquals("Use , then ", removeReferenceMarkers("Use \$review, then \$review", skill))
        assertTrue(referencesInText("deleted", listOf(skill)).isEmpty())
    }

    @Test fun catalogueAndDraftCodecRetainIdentityButSendOnlyNativeSelection() {
        val full = JSONObject().put("supported_kinds", JSONArray().put("skill"))
            .put("entries", JSONArray().put(skill.toJson())).put("issues", 1)
        val catalog = readReferenceCatalog(full)
        assertEquals(skill, catalog.entries.single())
        assertEquals(1, catalog.issues)
        assertEquals(listOf(skill), readReferenceDraft(referenceDraftJson(listOf(skill))))
        val params = turnStartParams("one", "\$review", "", references = listOf(skill))
        val ref = params.getJSONArray("references").getJSONObject(0)
        assertEquals(setOf("kind", "name", "path"), ref.keys().asSequence().toSet())
        assertFalse(turnStartParams("one", "plain", "").has("references"))
    }

    @Test fun staleCatalogueRepliesCannotLeakBetweenWorkspacesOrAfterDisconnect() {
        val state = ReferenceCatalogState()
        val empty = JSONObject().put("supported_kinds", JSONArray().put("skill")).put("entries", JSONArray())
        state.begin(1, "/a")
        state.begin(2, "/b")
        assertFalse(state.accept(1, "/a", "/b", empty))
        assertTrue(state.loading)
        assertTrue(state.accept(2, "/b", "/b", empty))
        state.begin(3, "/b")
        state.clear()
        assertFalse(state.accept(3, "/b", "/b", empty))
        assertTrue(state.catalog.entries.isEmpty())
        assertFalse(state.loading)
    }
}
