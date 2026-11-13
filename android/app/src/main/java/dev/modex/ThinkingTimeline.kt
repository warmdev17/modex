package dev.modex

/** Group for display only; keep canonical items unchanged for replay and persistence. */
internal fun thinkingTimeline(entries: List<ChatEntry>): List<ChatEntry> {
    val result = mutableListOf<ChatEntry>()
    val groups = mutableMapOf<String, Int>()
    var prompt = 0
    for (entry in entries) {
        if (entry.role == "You") prompt++
        if (entry.role != "reasoning") { result.add(entry); continue }
        if (entry.text.isBlank()) continue
        val key = if (prompt > 0) "prompt:$prompt" else "turn:${entry.turnId}"
        val index = groups[key]
        val latest = entry.summaryParts.filter { it.isNotBlank() }.ifEmpty { listOf(entry.text) }
        if (index == null) {
            groups[key] = result.size
            result.add(entry.copy(summaryParts = latest))
        } else {
            val first = result[index]
            result[index] = first.copy(text = first.text + "\n\n" + entry.text, status = entry.status, summaryParts = latest)
        }
    }
    return result
}

internal fun reasoningLabel(text: String): String {
    val heading = Regex("(?m)^\\s*\\*\\*(.+?)\\*\\*\\s*$").findAll(text).lastOrNull()?.groupValues?.get(1)
    return (heading ?: text.lineSequence().firstOrNull { it.isNotBlank() }.orEmpty()).trim().removePrefix("# ").replace("**", "")
}
