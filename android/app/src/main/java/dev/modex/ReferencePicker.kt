package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
internal fun ReferencePicker(
    catalog: ReferenceCatalog, query: String, loading: Boolean, status: String,
    selected: List<PromptReference>, onRefresh: () -> Unit, onDismiss: () -> Unit,
    onSelect: (PromptReference) -> Unit
) {
    val matches = catalog.entries.filter { entry ->
        query.isBlank() || listOf(entry.name, entry.title, entry.description, entry.scope, entry.pluginId).any { it.contains(query, ignoreCase = true) }
    }
    Column(Modifier.fillMaxWidth().semantics { contentDescription = "Prompt references" }) {
        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text("Add to prompt", style = MaterialTheme.typography.labelLarge, color = Mocha.Subtext, modifier = Modifier.weight(1f))
            TextButton(onClick = onRefresh, enabled = !loading) { Text("Refresh") }
            IconButton(onClick = onDismiss) { ChatGlyph(ChatIcon.Close, "Close reference suggestions") }
        }
        if (loading) LinearProgressIndicator(Modifier.fillMaxWidth(), color = Mocha.Blue, trackColor = Mocha.Surface)
        if (status.isNotEmpty()) Text(status, Modifier.padding(vertical = 8.dp), style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        if (matches.isEmpty() && !loading && status.isEmpty()) Text(if (catalog.entries.isEmpty()) "No skills or plugins found for this project." else "No matching skills or plugins.", Modifier.padding(vertical = 12.dp), color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
        LazyColumn(Modifier.fillMaxWidth().heightIn(max = 160.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            matches.groupBy { it.kind }.forEach { (kind, entries) ->
                item(key = "kind:$kind") { Text(if (kind == "skill") "Skills" else if (kind == "plugin") "Plugins" else kind.replaceFirstChar { it.titlecase() }, Modifier.padding(vertical = 6.dp), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) }
                items(entries, key = { it.kind + ":" + it.path }) { entry ->
                    val enabled = entry.enabled && (selected.size < 8 || selected.any { it.kind == entry.kind && it.name == entry.name })
                    Surface(onClick = { onSelect(entry) }, enabled = enabled, color = Mocha.Mantle, shape = RoundedCornerShape(6.dp), modifier = Modifier.fillMaxWidth().semantics { contentDescription = "Select ${entry.kind} ${entry.name}" }) {
                        Row(Modifier.padding(horizontal = 4.dp, vertical = 8.dp).heightIn(min = 48.dp), horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                            CompositionLocalProvider(LocalContentColor provides if (enabled) Mocha.Blue else Mocha.Subtext) { ChatGlyph(if (entry.kind == "plugin") ChatIcon.Plugin else ChatIcon.Skill) }
                            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                Text(entry.title, style = MaterialTheme.typography.bodyMedium, color = if (enabled) Mocha.Text else Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                if (entry.description.isNotBlank()) Text(entry.description, style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext, maxLines = 2, overflow = TextOverflow.Ellipsis)
                                Text(if (!entry.enabled) "Disabled on host" else listOf(entry.marker, entry.scope, entry.path).filter { it.isNotBlank() }.joinToString(" · "), style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
            }
        }
        if (selected.size >= 8) Text("Up to 8 references per prompt. Remove one to add another.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        if (catalog.issues > 0) Text("${catalog.issues} reference discovery issues on host. Refresh after fixing them.", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        HorizontalDivider(Modifier.padding(top = 6.dp), color = Mocha.Surface)
    }
}
