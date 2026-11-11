package dev.modex

import org.json.JSONObject

internal data class ModelChoice(val model: String = "", val effort: String = "") {
    fun toJson() = JSONObject().put("model", model).put("effort", effort)
}
internal fun readModelChoice(data: JSONObject?): ModelChoice? = data?.let {
    val model = it.optString("model"); val effort = it.optString("effort")
    if (model.isBlank() || !validModelId(model) || !validEffort(effort)) null else ModelChoice(model, effort)
}
internal fun modelChoicesJson(choices: Map<String,ModelChoice>) = JSONObject().apply { choices.forEach { (id, choice) -> put(id, choice.toJson()) } }
internal fun readModelChoices(data: JSONObject?): Map<String,ModelChoice> = data?.keys()?.asSequence()?.mapNotNull { id -> readModelChoice(data.optJSONObject(id))?.let { id to it } }?.toMap().orEmpty()
internal fun resolveModelChoice(saved: ModelChoice?, reported: ModelChoice?, host: ModelChoice, models: List<ModelOption>): ModelChoice {
    val choice = saved ?: reported?.takeIf { it.model.isNotBlank() } ?: host
    if (choice.effort.isNotBlank()) return choice
    val effort = if (choice.model == host.model && host.effort.isNotBlank()) host.effort else models.firstOrNull { it.model == choice.model }?.defaultEffort.orEmpty()
    return choice.copy(effort = effort)
}
