package dev.modex

internal fun workStatusLabel(status: String): String = when (status) {
    "inProgress" -> "Working"
    "completed" -> "Worked"
    "interrupted" -> "Interrupted"
    "cancelled", "canceled" -> "Cancelled"
    "failed" -> "Failed"
    else -> "Ended"
}

internal fun formatWorkDuration(millis: Long): String {
    val seconds = millis.coerceAtLeast(0) / 1000
    return if (seconds < 60) "${seconds}s" else "${seconds / 60}m ${seconds % 60}s"
}
