package dev.modex

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class ConnectionProfilesTest {
    @Test fun legacyProfileRetainsStorageIdentity() {
        val data = connectionProfiles(null, legacy = true)
        assertEquals(listOf(HostProfile("host", "My computer")), data)
        assertEquals(emptyList<HostProfile>(), connectionProfiles(null, legacy = false))
    }
    @Test fun roundTripNamesAndIndependentIds() {
        val profiles = listOf(HostProfile("host", "Laptop"), HostProfile("29ac3da8-6120-4420-b152-eb0225b09dbc", "VPS"))
        assertEquals(profiles, connectionProfiles(profilesJson(profiles), false))
        assertNotEquals(profiles[0].id, profiles[1].id)
    }
    @Test fun rejectInvalidAndDuplicateStorageIdsRatherThanOverwriteCredentials() {
        for (id in listOf("../host", "connections", "", "host.enc")) {
            try { connectionProfiles(JSONObject("""{"hosts":[{"id":"$id","name":"Remote"}]}"""), false); fail(id) }
            catch (_: IllegalArgumentException) { }
        }
        try { connectionProfiles(JSONObject("""{"hosts":[{"id":"host","name":"A"},{"id":"host","name":"B"}]}"""), false); fail() }
        catch (_: IllegalArgumentException) { }
    }
    @Test fun foregroundServiceBelongsToAllActiveRemotes() {
        assertTrue(keepConnectionService(listOf(false to false, true to false)))
        assertTrue(keepConnectionService(listOf(false to true, false to false)))
        assertFalse(keepConnectionService(listOf(false to false)))
        assertFalse(keepConnectionService(emptyList()))
    }
}
