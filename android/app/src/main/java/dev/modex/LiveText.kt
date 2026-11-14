package dev.modex

import androidx.compose.runtime.*
import kotlinx.coroutines.delay

internal fun nextRevealEnd(text: String, shown: Int): Int {
    if (shown >= text.length) return text.length
    val count = ((text.length - shown) / 5).coerceIn(1, 80)
    return text.offsetByCodePoints(shown, count.coerceAtMost(text.codePointCount(shown, text.length)))
}

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

/** Historical content starts complete; only new live append operations are animated. */
@Composable
internal fun liveMessageText(id: String, text: String, live: Boolean): String {
    var visible by remember(id) { mutableStateOf(text) }
    LaunchedEffect(id, text, live) {
        if (!live || !text.startsWith(visible)) visible = text
        else while (visible.length < text.length) {
            visible = text.substring(0, nextRevealEnd(text, visible.length))
            delay(20)
        }
    }
    return visible
}
