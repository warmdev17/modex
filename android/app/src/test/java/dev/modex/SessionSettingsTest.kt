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
