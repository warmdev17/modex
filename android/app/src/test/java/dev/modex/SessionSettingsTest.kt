package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class SessionSettingsTest {
    @Test fun defaultInheritsHostWithoutPermissionOverride() {
        val params = newThreadParams("", "host-default")
        assertEquals(0, params.length())
    }
    @Test fun explicitModeAndModelAreSerialized() {
        val params = newThreadParams("custom-model", "read-only")
        assertEquals("read-only", params.getString("permission_mode"))
        assertEquals("custom-model", params.getString("model"))
    }
    @Test fun turnModelIsOptionalAndValidatedBySharedBuilder() {
        assertFalse(turnStartParams("thread", "hello", "").has("model"))
        assertEquals("gpt-5.6-sol", turnStartParams("thread", "hello", "gpt-5.6-sol").getString("model"))
        assertEquals("custom-model", turnStartParams("thread", "hello", " custom-model ").getString("model"))
    }
    @Test fun composerRejectsInvalidModelIds() {
        assertTrue(validModelId(""))
        assertTrue(validModelId("gpt-5.6-sol"))
        assertFalse(validModelId("   "))
        assertFalse(validModelId("bad\nmodel"))
        assertFalse(validModelId("m".repeat(257)))
        assertFalse(validModelId("é".repeat(129)))
    }
    @Test fun catalogUsesModelIdAndOnlyAdvertisedEfforts() {
        val models = readModels(JSONObject("""{"data":[{"id":"picker-id","model":"custom","displayName":"Custom","defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Fast"},{"reasoningEffort":"medium","description":"Balanced"}]}]}"""))
        assertEquals("custom", models.single().model)
        assertEquals(listOf("low", "medium"), models.single().efforts.map { it.first })
        assertEquals("medium", models.single().defaultEffort)
        assertEquals("high", turnStartParams("t", "hello", "custom", "high").getString("effort"))
        assertFalse(turnStartParams("t", "hello", "custom", "").has("effort"))
        assertFalse(validEffort("bad\neffort"))
    }
    @Test fun projectsKeepSameNamedFoldersSeparateAcrossPlatforms() {
        assertEquals("modex", projectName("/home/me/modex"))
        assertEquals("modex", projectName("C:\\Work\\modex"))
        assertEquals("/", projectName("/"))
        val result = withWorkspace(turnStartParams("t", "hi", ""), "/other")
        assertEquals("/other", result.getString("workspace"))
        assertFalse(withWorkspace(JSONObject(), "").has("workspace"))
    }
    @Test fun missingSettingsAreUnknown() {
        val label = sessionSettingsLabel(JSONObject())
        assertTrue(label.contains("Sandbox: Unknown"))
        assertTrue(label.contains("Approvals: Unknown"))
    }
    @Test fun usesReturnedSettingsRatherThanRequestedMode() {
        val label = sessionSettingsLabel(JSONObject("""{"thread":{"modelProvider":"third-party"},"model":"custom-model","sandbox":"readOnly","approval_policy":"on-request","approvals_reviewer":"user"}"""))
        assertTrue(label.contains("third-party"))
        assertTrue(label.contains("Read-only"))
        assertTrue(label.contains("on-request"))
    }
}
