package dev.modex

import org.json.JSONObject
import kotlin.math.roundToInt

internal data class RateLimitWindow(val usedPercent: Double?, val windowDurationMins: Long?, val resetsAt: Long?)

internal fun usageLimitWarning(limits: JSONObject?, nowSeconds: Long = System.currentTimeMillis() / 1000): String {
    if (limits == null) return ""
    val reached = limits.optString("rateLimitReachedType")
    when (reached) {
        "workspace_owner_credits_depleted", "workspace_member_credits_depleted" -> return "Codex workspace credits are depleted."
        "workspace_owner_usage_limit_reached", "workspace_member_usage_limit_reached" -> return "Codex workspace usage limit reached."
    }
    val windows = listOfNotNull(limits.optJSONObject("primary"), limits.optJSONObject("secondary"))
    val exhausted = windows.filter {
        (it.opt("usedPercent") as? Number)?.toDouble()?.let { percent -> percent.isFinite() && percent >= 100 } == true &&
            ((it.opt("resetsAt") as? Number)?.toLong()?.takeIf { reset -> reset > 0 }?.let { reset -> reset > nowSeconds } ?: true)
    }
    if (exhausted.isEmpty()) {
        return if (reached == "rate_limit_reached" && windows.isEmpty()) "Codex usage limit reached." else ""
    }
    val label = if (exhausted.size > 1) "Codex usage limits reached." else when (exhausted.single().optInt("windowDurationMins")) {
        300 -> "Codex 5-hour usage limit reached."
        10080 -> "Codex weekly usage limit reached."
        else -> "Codex usage limit reached."
    }
    val resets = exhausted.mapNotNull { (it.opt("resetsAt") as? Number)?.toLong()?.takeIf { reset -> reset in 1..Long.MAX_VALUE / 1000 } }
    if (resets.size != exhausted.size) return label
    val formatted = java.text.DateFormat.getDateTimeInstance(java.text.DateFormat.SHORT, java.text.DateFormat.SHORT)
        .format(java.util.Date(resets.max() * 1000))
    return "$label Resets $formatted."
}

internal data class SessionStatus(
    val mode: String = "",
    val model: String = "",
    val effort: String = "",
    val workspace: String = "",
    val provider: String = "",
    val sandbox: String = "",
    val approvalPolicy: String = "",
    val approvalsReviewer: String = "",
    val permissionProfile: String = "",
    val contextTokens: Long? = null,
    val totalTokens: Long? = null,
    val contextWindow: Long? = null,
    val limitId: String = "",
    val limitName: String = "",
    val planType: String = "",
    val primaryLimit: RateLimitWindow? = null,
    val secondaryLimit: RateLimitWindow? = null,
)

internal data class ModeControl(val label: String, val target: String, val refresh: Boolean, val accented: Boolean, val enabled: Boolean)

internal fun modeControl(mode: String, uncertain: Boolean, changing: Boolean): ModeControl = when {
    changing -> ModeControl("Changing mode…", "", refresh = false, accented = false, enabled = false)
    uncertain -> ModeControl("Mode unknown · Refresh", "", refresh = true, accented = false, enabled = true)
    mode == "plan" -> ModeControl("Plan", "default", refresh = false, accented = true, enabled = true)
    mode == "default" -> ModeControl("Default", "plan", refresh = false, accented = false, enabled = true)
    else -> ModeControl("Enable Plan", "plan", refresh = false, accented = false, enabled = true)
}

internal fun contextSummary(status: SessionStatus?): String {
    val used = status?.contextTokens
    val window = status?.contextWindow
    if (used == null || window == null || window <= 0) return "Context unavailable"
    val format = java.text.NumberFormat.getIntegerInstance(java.util.Locale.US)
    val percent = ((used.toDouble() / window) * 100).coerceIn(0.0, 100.0).toInt()
    return "Context $percent% · ${format.format(used)} / ${format.format(window)}"
}

internal fun contextRemaining(status: SessionStatus): Int? {
    val used = status.contextTokens?.takeIf { it >= 0 } ?: return null
    val window = status.contextWindow?.takeIf { it > 0 } ?: return null
    return ((1.0 - used.toDouble() / window).coerceIn(0.0, 1.0) * 100).roundToInt()
}

internal fun SessionStatus.usageWindow(durationMins: Long): RateLimitWindow? =
    listOfNotNull(primaryLimit, secondaryLimit).firstOrNull { it.windowDurationMins == durationMins }

internal fun RateLimitWindow.remainingPercent(): Int? =
    usedPercent?.takeIf { it.isFinite() }?.let { (100.0 - it.coerceIn(0.0, 100.0)).roundToInt() }

internal fun readSessionStatus(result: JSONObject, previous: SessionStatus? = null): SessionStatus {
    val session = result.optJSONObject("session") ?: result
    val thread = session.optJSONObject("thread")
    fun number(value: JSONObject?, key: String): Long? = (value?.opt(key) as? Number)?.toLong()
    fun limit(value: JSONObject?): RateLimitWindow? = value?.let {
        RateLimitWindow((it.opt("usedPercent") as? Number)?.toDouble(), number(it, "windowDurationMins"), number(it, "resetsAt"))
    }
    val usage = result.optJSONObject("token_usage")
    val limits = result.optJSONObject("rate_limits")
    val preserveMetrics = !result.has("token_usage") && !result.has("rate_limits")
    return SessionStatus(
        mode = session.optString("collaboration_mode"),
        model = session.optString("model"),
        effort = session.optString("effort"),
        workspace = thread?.optString("cwd").orEmpty(),
        provider = thread?.optString("modelProvider").orEmpty(),
        sandbox = session.optString("sandbox"),
        approvalPolicy = session.optString("approval_policy"),
        approvalsReviewer = session.optString("approvals_reviewer"),
        permissionProfile = session.optString("permission_profile"),
        contextTokens = if (preserveMetrics) previous?.contextTokens else number(usage?.optJSONObject("last"), "totalTokens"),
        totalTokens = if (preserveMetrics) previous?.totalTokens else number(usage?.optJSONObject("total"), "totalTokens"),
        contextWindow = if (preserveMetrics) previous?.contextWindow else number(usage, "modelContextWindow"),
        limitId = if (preserveMetrics) previous?.limitId.orEmpty() else limits?.optString("limitId").orEmpty(),
        limitName = if (preserveMetrics) previous?.limitName.orEmpty() else limits?.optString("limitName").orEmpty(),
        planType = if (preserveMetrics) previous?.planType.orEmpty() else limits?.optString("planType").orEmpty(),
        primaryLimit = if (preserveMetrics) previous?.primaryLimit else limit(limits?.optJSONObject("primary")),
        secondaryLimit = if (preserveMetrics) previous?.secondaryLimit else limit(limits?.optJSONObject("secondary")),
    )
}

internal fun threadModeParams(thread: String, workspace: String, mode: String, model: String, effort: String): JSONObject {
    require(thread.isNotBlank() && workspace.isNotBlank() && mode in listOf("default", "plan") && validModelId(model) && validEffort(effort))
    return JSONObject().put("thread_id", thread).put("workspace", workspace).put("mode", mode).put("model", model).apply {
        if (effort.isNotBlank()) put("effort", effort)
    }
}
