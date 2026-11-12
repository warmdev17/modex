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
    @Test fun standaloneChatNeverBecomesAProjectFromItsFolder() {
        val projects = listOf(RemoteProject("real", "Modex", listOf("/work/modex")), RemoteProject("empty", "Empty", listOf("/work/empty")))
        val groups = projectSessions(projects, listOf("member", "standalone"), mapOf("member" to "real"))
        assertEquals(listOf("member"), groups["real"])
        assertEquals(emptyList<String>(), groups["empty"])
        assertFalse(groups.values.flatten().contains("standalone"))
    }
    @Test fun projectlessAndCanonicalMembershipRemainDistinctFromUnassigned() {
        val data = org.json.JSONArray("""[{"id":"standalone","projectId":null,"projectless":true},{"id":"native","projectId":null},{"id":"empty","projectId":""},{"id":"canonical","projectId":"real","projectless":true}]""")
        val membership = readProjectMembership(data)
        assertEquals(mapOf("standalone" to "", "canonical" to "real"), membership)
    }
    @Test fun nativeSessionsJoinExistingProjectsByRootWithoutInventingProjects() {
        val projects = listOf(RemoteProject("modex", "Modex", listOf("/work/modex")), RemoteProject("nested", "Nested", listOf("/work/modex/tools")))
        val threads = listOf("current", "hello", "child", "outside", "standalone", "explicit", "other")
        val paths = mapOf("current" to "/work/modex", "hello" to "/work/modex/", "child" to "/work/modex/tools/src", "outside" to "/work/modex-other", "standalone" to "/work/modex", "explicit" to "/work/modex/tools", "other" to "/tmp/new-chat")
        val groups = projectSessions(projects, threads, mapOf("standalone" to "", "explicit" to "modex"), paths)
        assertEquals(listOf("current", "hello", "explicit"), groups["modex"])
        assertEquals(listOf("child"), groups["nested"])
        assertEquals(2, groups.size)
    }
    @Test fun projectGroupingHandlesWindowsAndAmbiguousRoots() {
        val projects = listOf(RemoteProject("windows", "Windows", listOf("C:\\Work\\Modex")), RemoteProject("a", "A", listOf("/same")), RemoteProject("b", "B", listOf("/same")))
        val groups = projectSessions(projects, listOf("windows", "ambiguous"), emptyMap(), mapOf("windows" to "c:/work/modex/src", "ambiguous" to "/same"))
        assertEquals(listOf("windows"), groups["windows"])
        assertTrue(groups["a"].isNullOrEmpty())
        assertTrue(groups["b"].isNullOrEmpty())
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
