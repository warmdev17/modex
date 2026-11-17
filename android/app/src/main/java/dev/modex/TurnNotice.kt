package dev.modex

import org.json.JSONObject

internal data class TurnNotice(val category: String, val message: String, val retrying: Boolean, val httpStatus: Int? = null) {
    fun toJson(): JSONObject = JSONObject().put("category", category).put("message", message).put("retrying", retrying)
        .apply { httpStatus?.let { put("http_status", it) } }
}

// This object is the host's safe projection; never render the native error message.
internal fun readTurnNotice(value: JSONObject?): TurnNotice? = value?.let {
    val category = it.optString("category")
    val message = it.optString("message")
    if (category.isBlank() || message.isBlank()) null else TurnNotice(category, message, it.optBoolean("retrying"), it.optInt("http_status").takeIf { code -> code in 100..599 })
}

internal fun eventTurnNotice(params: JSONObject): TurnNotice? {
    readTurnNotice(params.optJSONObject("notice") ?: params.optJSONObject("turn")?.optJSONObject("notice"))?.let { return it }
    val error = params.optJSONObject("error") ?: params.optJSONObject("turn")?.optJSONObject("error")
    val info = error?.opt("codexErrorInfo") ?: error?.opt("codex_error_info")
    val kind = if (info is JSONObject) info.keys().asSequence().singleOrNull().orEmpty() else info as? String ?: ""
    val message = error?.optString("message").orEmpty()
    val known = when {
        kind == "contextWindowExceeded" -> "context_window_exceeded" to "This conversation exceeds the model context limit. Start a new conversation or compact it on the computer."
        kind in listOf("usageLimitExceeded", "sessionBudgetExceeded", "rateLimitExceeded") -> "usage_limit_exceeded" to "Codex usage or request limit reached. Check the provider account before trying again."
        kind in listOf("unauthorized", "modelNotFound") -> "unauthorized" to "The provider rejected this model or account. Check access on the computer."
        kind == "responseTooManyFailedAttempts" || message.contains("retries exhausted", true) -> "retry_exhausted" to "Codex exhausted its response retries. Review partial output before trying again."
        kind == "responseStreamDisconnected" || message.contains("stream disconnected before completion", true) || message.contains("stream closed before completion", true) -> "stream_disconnected" to "The response stream disconnected before this turn finished."
        else -> null
    }
    val retrying = params.optBoolean("willRetry")
    if (known != null) return TurnNotice(known.first, known.second + if (retrying) " Codex is retrying." else "", retrying)
    if (retrying) return TurnNotice("retrying", "Connection interrupted. Codex is retrying…", true)
    return turnFailure(params)?.let { TurnNotice("turn_failed", it, false) }
}
