package dev.modex

import org.json.JSONArray
import org.json.JSONObject

internal data class HostProfile(val id: String, val name: String)
internal fun connectionProfiles(data: JSONObject?, legacy: Boolean): List<HostProfile> {
    if (data == null) return if (legacy) listOf(HostProfile("host", "My computer")) else emptyList()
    val list = data.getJSONArray("hosts")
    val profiles = (0 until list.length()).map { index ->
        val item = list.getJSONObject(index)
        val id = item.getString("id")
        require(id == "host" || id.matches(Regex("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")))
        val name = item.getString("name")
        require(name.isNotBlank() && name.length <= 80 && name.none { it.isISOControl() })
        HostProfile(id, name)
    }
    require(profiles.map { it.id }.distinct().size == profiles.size)
    return profiles
}
internal fun profilesJson(profiles: List<HostProfile>): JSONObject = JSONObject().put("hosts", JSONArray().apply {
    profiles.forEach { put(JSONObject().put("id", it.id).put("name", it.name)) }
})
internal fun keepConnectionService(states: List<Pair<Boolean, Boolean>>) = states.any { it.first || it.second }
