package dev.modex

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
internal fun CommandCard(entry: ChatEntry) {
    var expanded by rememberSaveable(entry.id) { mutableStateOf(false) }
    val running = entry.status == "inProgress"
    val failed = entry.status in listOf("failed", "declined") || (entry.exitCode != null && entry.exitCode != 0)
    val status = when { running -> "Running"; failed -> "Failed"; entry.status == "completed" -> "Completed"; else -> "Status unknown" }
    val color = when { failed -> MaterialTheme.colorScheme.error; running -> Mocha.Blue; else -> Mocha.Subtext }
    Column(Modifier.fillMaxWidth()) {
        Surface(onClick = { expanded = !expanded }, color = MaterialTheme.colorScheme.background,
            shape = RoundedCornerShape(6.dp), modifier = Modifier.fillMaxWidth().semantics {
                    contentDescription = if (expanded) "Collapse command" else "Expand command"
                    stateDescription = "$status, ${if (expanded) "expanded" else "collapsed"}"
            }) {
            Row(Modifier.heightIn(min = 48.dp).padding(horizontal = 2.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Canvas(Modifier.size(18.dp).semantics { contentDescription = "Terminal activity" }) {
                    val stroke = 1.6.dp.toPx()
                    drawLine(Mocha.Subtext, Offset(2.dp.toPx(), 4.dp.toPx()), Offset(7.dp.toPx(), 9.dp.toPx()), stroke, StrokeCap.Round)
                    drawLine(Mocha.Subtext, Offset(7.dp.toPx(), 9.dp.toPx()), Offset(2.dp.toPx(), 14.dp.toPx()), stroke, StrokeCap.Round)
                    drawLine(Mocha.Subtext, Offset(9.dp.toPx(), 14.dp.toPx()), Offset(16.dp.toPx(), 14.dp.toPx()), stroke, StrokeCap.Round)
                }
                if (running) CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 1.5.dp, color = color)
                Column(Modifier.weight(1f)) {
                    Text(entry.command.lineSequence().firstOrNull().orEmpty().ifBlank { "Shell command" }, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodySmall)
                    Text(status + (entry.exitCode?.let { " · exit $it" } ?: ""), color = color, style = MaterialTheme.typography.labelSmall)
                }
                Canvas(Modifier.size(14.dp)) {
                    val mid = size.height / 2
                    val dy = if (expanded) -2.dp.toPx() else 2.dp.toPx()
                    drawLine(Mocha.Subtext, Offset(2.dp.toPx(), mid-dy/2), Offset(size.width/2, mid+dy/2), 1.5.dp.toPx(), StrokeCap.Round)
                    drawLine(Mocha.Subtext, Offset(size.width/2, mid+dy/2), Offset(size.width-2.dp.toPx(), mid-dy/2), 1.5.dp.toPx(), StrokeCap.Round)
                }
            }
        }
        if (expanded) Surface(color = Mocha.Mantle, shape = RoundedCornerShape(6.dp), modifier = Modifier.fillMaxWidth().padding(start = 26.dp, top = 4.dp)) {
            SelectionContainer {
                Column(Modifier.padding(10.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (entry.cwd.isNotBlank()) Text(entry.cwd, color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
                    Text(entry.command, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodySmall, modifier = Modifier.horizontalScroll(rememberScrollState()))
                    Text(entry.text.ifEmpty { if (running) "Waiting for output…" else "No output" }, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodySmall, modifier = Modifier.horizontalScroll(rememberScrollState()))
                }
            }
        }
    }
}
