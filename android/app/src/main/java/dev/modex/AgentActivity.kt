package dev.modex

import org.json.JSONArray
import org.json.JSONObject

internal data class AgentTarget(val id: String, val label: String, val status: String)

internal data class AgentActivity(val action: String, val targets: List<AgentTarget>, val live: Boolean = false) {
    fun toJson() = JSONObject().put("action", action).put("targets", JSONArray().apply {
        targets.forEach { put(JSONObject().put("id", it.id).put("label", it.label).put("status", it.status)) }
    })

    companion object {
        fun read(item: JSONObject?): AgentActivity? {
            val action = item?.optString("action") ?: return null
            if (action !in setOf("started", "assigned", "interacted", "resumed", "waited", "stopRequested", "completed", "interrupted")) return null
            val targets = item.optJSONArray("targets") ?: return null
            val agents = (0 until minOf(targets.length(), 64)).mapNotNull { index ->
                val target = targets.optJSONObject(index) ?: return@mapNotNull null
                val id = (target.opt("id") as? String)?.takeIf { it.isNotBlank() && it.length <= 128 } ?: return@mapNotNull null
                val label = (target.opt("label") as? String).orEmpty().take(96)
                val status = (target.opt("status") as? String).orEmpty()
                AgentTarget(id, label, status.takeIf { it in setOf("unknown", "pendingInit", "running", "completed", "invocationCompleted", "errored", "interrupted", "shutdown", "notFound") }.orEmpty())
            }.distinctBy { it.id }
            return agents.takeIf { it.isNotEmpty() }?.let { AgentActivity(action, it) }
        }
    }
}

internal data class AgentTimeline(val id: String, val label: String, val status: String, val milestones: List<String>)

internal fun agentTimeline(entries: List<ChatEntry>, activeTurn: String, connected: Boolean): List<AgentTimeline> {
    data class Progress(
        var name: String = "", var status: String = "unknown", var observedTurn: String = "", var observedLive: Boolean = false,
        val milestones: MutableList<String> = mutableListOf(),
    )
    val agents = linkedMapOf<String, Progress>()
    entries.forEach { entry ->
        val activity = entry.agentActivity ?: return@forEach
        activity.targets.forEach { target ->
            val agent = agents.getOrPut(target.id) { Progress() }
            if (target.label.isNotBlank()) agent.name = target.label
            val status = target.status.ifBlank {
                if (activity.action in setOf("assigned", "resumed")) "unknown" else ""
            }
            val milestone = when (activity.action) {
                "started" -> "Started"
                "assigned" -> "Received a task"
                "interacted" -> "Interaction"
                "resumed" -> "Resumed"
                "stopRequested" -> "Stop requested"
                else -> null
            }
            fun add(label: String) {
                if (agent.milestones.lastOrNull() != label) agent.milestones.add(label)
            }
            milestone?.let(::add)
            if (status.isNotEmpty()) {
                if (status != agent.status && status !in setOf("unknown", "pendingInit") &&
                    !(status == "running" && activity.action == "started")) {
                    add(agentStatusLabel(status))
                }
                agent.status = status
                agent.observedTurn = entry.turnId
                agent.observedLive = activity.live
            }
        }
    }
    return agents.map { (id, agent) ->
        val fresh = connected && (agent.observedLive || (activeTurn.isNotBlank() && agent.observedTurn == activeTurn))
        val status = if (!fresh && agent.status in setOf("running", "pendingInit")) "unknown" else agent.status
        AgentTimeline(id, agent.name.ifBlank { "Agent ${id.takeLast(8)}" }, status, agent.milestones)
    }
}

internal fun agentStatusLabel(status: String): String = when (status) {
    "pendingInit" -> "Starting"
    "running" -> "Running"
    "completed" -> "Completed"
    "invocationCompleted" -> "Invocation completed"
    "errored" -> "Error"
    "interrupted" -> "Interrupted"
    "shutdown" -> "Stopped"
    "notFound" -> "Unavailable"
    else -> "Status unknown"
}
