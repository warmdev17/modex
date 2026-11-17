package dev.modex

import org.json.JSONObject

internal data class HostActivity(val hostId: String, val threadId: String, val title: String, val running: Boolean, val waiting: Int) {
    val key: String get() = "$hostId:$threadId"
}

internal fun hostActivity(hostId: String, running: Set<String>, decisions: List<JSONObject>, titles: Map<String, String>): List<HostActivity> {
    val waiting = decisions.groupingBy { it.optString("thread_id") }.eachCount()
    return (running.filter { it.isNotBlank() }.toSet() + waiting.keys).map { thread ->
        HostActivity(hostId, thread, titles[thread]?.takeIf { it.isNotBlank() }
            ?: if (thread.isBlank()) "Host needs input" else "Conversation ${thread.take(8)}", thread in running, waiting[thread] ?: 0)
    }.sortedWith(compareByDescending<HostActivity> { it.waiting > 0 }.thenBy { it.title }.thenBy { it.threadId })
}
