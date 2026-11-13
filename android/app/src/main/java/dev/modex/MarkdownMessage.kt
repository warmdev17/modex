package dev.modex

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.*
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.*
import androidx.compose.ui.text.font.*
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import org.commonmark.node.*
import org.commonmark.parser.Parser
import org.commonmark.ext.gfm.tables.*

private val markdownParser = Parser.builder().extensions(listOf(TablesExtension.create())).build()
internal fun markdownDocument(text: String): Node = markdownParser.parse(text)
internal fun isSafeMarkdownLink(url: String): Boolean = try {
    val uri = java.net.URI(url)
    (uri.scheme.equals("https", true) || uri.scheme.equals("http", true)) && !uri.host.isNullOrBlank()
} catch (_: Exception) { false }

private fun Node.children(): List<Node> = buildList { var child = firstChild; while (child != null) { add(child); child = child.next } }

@Composable
internal fun MarkdownMessage(text: String, selectable: Boolean = true) {
    val document = remember(text) { markdownDocument(text) }
    val content: @Composable () -> Unit = { Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(12.dp)) { MarkdownBlocks(document) } }
    if (selectable) SelectionContainer { content() } else content()
}

@Composable
private fun MarkdownBlocks(parent: Node) {
    for (node in parent.children()) when (node) {
        is org.commonmark.node.Paragraph -> InlineText(node)
        is Heading -> InlineText(node, when (node.level) { 1 -> MaterialTheme.typography.headlineSmall; 2 -> MaterialTheme.typography.titleLarge; else -> MaterialTheme.typography.titleMedium }.copy(fontWeight = FontWeight.SemiBold))
        is FencedCodeBlock -> CodeBlock(node.literal, node.info.substringBefore(' ').ifBlank { "code" })
        is IndentedCodeBlock -> CodeBlock(node.literal, "code")
        is BulletList -> MarkdownList(node, null)
        is OrderedList -> MarkdownList(node, node.markerStartNumber)
        is BlockQuote -> Surface(color = Mocha.Mantle, shape = MaterialTheme.shapes.medium) { Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) { MarkdownBlocks(node) } }
        is ThematicBreak -> HorizontalDivider(color = Mocha.Surface)
        is TableBlock -> MarkdownTable(node)
        is HtmlBlock -> Text(node.literal, style = MaterialTheme.typography.bodyMedium)
        else -> MarkdownBlocks(node)
    }
}

@Composable
private fun MarkdownList(node: Node, start: Int?) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        node.children().forEachIndexed { index, item ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(if (start == null) "•" else "${start + index}.", style = MaterialTheme.typography.bodyLarge, color = Mocha.Subtext)
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(8.dp)) { MarkdownBlocks(item) }
            }
        }
    }
}

@Composable
private fun InlineText(node: Node, style: TextStyle = MaterialTheme.typography.bodyLarge) {
    val uri = LocalUriHandler.current
    val accent = Mocha.Blue
    val content = buildAnnotatedString {
        fun appendNode(n: Node) {
            when (n) {
                is org.commonmark.node.Text -> append(n.literal)
                is Code -> withStyle(SpanStyle(fontFamily = FontFamily.Monospace, background = Mocha.Surface, color = Mocha.Text)) { append(n.literal) }
                is SoftLineBreak -> append("\n")
                is HardLineBreak -> append("\n")
                is StrongEmphasis -> withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { n.children().forEach { appendNode(it) } }
                is Emphasis -> withStyle(SpanStyle(fontStyle = FontStyle.Italic)) { n.children().forEach { appendNode(it) } }
                is Link -> if (isSafeMarkdownLink(n.destination)) {
                    withLink(LinkAnnotation.Url(n.destination, TextLinkStyles(style = SpanStyle(color = accent, textDecoration = TextDecoration.Underline)), LinkInteractionListener { runCatching { uri.openUri(n.destination) } })) { n.children().forEach { appendNode(it) } }
                } else n.children().forEach { appendNode(it) }
                is Image -> { append("[Image: "); n.children().forEach { appendNode(it) }; append("]") }
                is HtmlInline -> append(n.literal)
                else -> n.children().forEach { appendNode(it) }
            }
        }
        node.children().forEach { appendNode(it) }
    }
    Text(content, style = style, modifier = Modifier.fillMaxWidth())
}

@Composable
private fun CodeBlock(code: String, language: String) {
    val context = LocalContext.current
    var copied by remember(code) { mutableStateOf(false) }
    Surface(color = Mocha.Mantle, shape = MaterialTheme.shapes.medium, border = BorderStroke(1.dp, Mocha.Surface), modifier = Modifier.fillMaxWidth()) {
        Column {
            Row(Modifier.fillMaxWidth().padding(start = 14.dp, end = 4.dp), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                Text(language, color = Mocha.Subtext, style = MaterialTheme.typography.labelMedium)
                TextButton(onClick = {
                    (context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText("Code", code)); copied = true
                }) { Text(if (copied) "Copied" else "Copy code") }
            }
            HorizontalDivider(color = Mocha.Surface)
            Text(code.removeSuffix("\n"), fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.horizontalScroll(rememberScrollState()).padding(14.dp))
        }
    }
}

@Composable
private fun MarkdownTable(table: TableBlock) {
    val rows = table.children().flatMap { it.children() }
    Column(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).width(IntrinsicSize.Max)) {
        for (row in rows) {
            Row(Modifier.fillMaxWidth()) {
                row.children().forEach { cell ->
                    Surface(color = if ((cell as? TableCell)?.isHeader == true) Mocha.Surface else Mocha.Mantle, border = BorderStroke(0.5.dp, Mocha.Overlay), modifier = Modifier.width(180.dp)) {
                        Box(Modifier.padding(12.dp)) { InlineText(cell, MaterialTheme.typography.bodyMedium) }
                    }
                }
            }
        }
    }
}
