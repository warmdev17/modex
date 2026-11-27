package dev.modex

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.json.JSONObject

internal data class AntigravityOption(val id: String, val current: String, val values: Map<String, String>) {
    val currentLabel get() = values[current] ?: current
}

internal fun readAntigravityOptions(result: JSONObject): List<AntigravityOption> {
    val options = result.optJSONArray("options") ?: return emptyList()
    return (0 until minOf(options.length(), 8)).mapNotNull { index ->
        val item = options.optJSONObject(index) ?: return@mapNotNull null
        val id = item.optString("id").takeIf { it == "model" || it == "thought_level" } ?: return@mapNotNull null
        val values = item.optJSONArray("options") ?: return@mapNotNull null
        val choices = buildMap {
            for (i in 0 until minOf(values.length(), 100)) {
                val entry = values.optJSONObject(i) ?: continue
                val value = (entry.opt("value") as? String)?.takeIf { it.isNotBlank() && it.length <= 256 } ?: continue
                put(value, (entry.opt("name") as? String).orEmpty().take(128).ifBlank { value })
            }
        }
        val current = (item.opt("currentValue") as? String)?.take(256).orEmpty()
        AntigravityOption(id, current, choices).takeIf { choices.isNotEmpty() }
    }.distinctBy { it.id }
}

@Composable
internal fun AntigravityOptionsDialog(
    options: List<AntigravityOption>, loading: Boolean, error: String, canChange: Boolean,
    onSelect: (String, String) -> Unit, onRefresh: () -> Unit, onClose: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text("Antigravity model settings") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (loading) Text("Checking session settings…", color = Mocha.Subtext)
                if (error.isNotBlank()) {
                    Text(error, color = MaterialTheme.colorScheme.error)
                    TextButton(onClick = onRefresh, enabled = !loading) { Text("Refresh") }
                }
                if (options.isEmpty() && !loading && error.isBlank()) {
                    Text("This session uses Antigravity’s default model.", color = Mocha.Subtext)
                }
                for (option in options) {
                    var expanded by remember(option.id) { mutableStateOf(false) }
                    Text(if (option.id == "model") "Model" else "Thinking level", style = MaterialTheme.typography.labelMedium)
                    Box {
                        OutlinedButton(onClick = { expanded = true }, enabled = canChange && !loading && error.isBlank(), modifier = Modifier.fillMaxWidth()) {
                            Text(option.currentLabel.ifBlank { "Choose an option" }, modifier = Modifier.weight(1f))
                            ChatGlyph(ChatIcon.ChevronDown, color = Mocha.Subtext)
                        }
                        ModexDropdownMenu(expanded && canChange && !loading, { expanded = false }, modifier = Modifier.heightIn(max = 260.dp)) {
                            option.values.forEach { (value, label) ->
                                DropdownMenuItem(text = { Text(label) }, onClick = {
                                    expanded = false
                                    if (value != option.current) onSelect(option.id, value)
                                })
                            }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onClose) { Text("Done") } },
    )
}
