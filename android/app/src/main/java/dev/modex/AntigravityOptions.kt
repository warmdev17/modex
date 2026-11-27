package dev.modex

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.runtime.key
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
    val model = options.firstOrNull { it.id == "model" }
    val thinking = options.firstOrNull { it.id == "thought_level" }
    val models = remember(model?.values) {
        model?.values.orEmpty().map { (id, label) -> ModelOption(id, label, "", emptyList()) }
    }
    val enabled = canChange && !loading && error.isBlank()
    ModelSettingsDialog(onClose, actions = { TextButton(onClick = onClose) { Text("Done") } }) {
        if (loading) Text("Checking session settings…", color = Mocha.Subtext)
        if (error.isNotBlank()) {
            Text(error, color = MaterialTheme.colorScheme.error)
            TextButton(onClick = onRefresh, enabled = !loading) { Text("Refresh") }
        }
        if (model != null) {
            ModelPicker(models, "", model.current, enabled = enabled, allowCustom = false) {
                if (it != model.current) onSelect(model.id, it)
            }
        } else if (!loading && error.isBlank()) {
            Text("This session uses Antigravity’s default model.", color = Mocha.Subtext)
        }
        if (thinking != null || (!loading && error.isBlank())) key(model?.current) {
            ModelLevelControl("Thinking level", "Thinking level",
                thinking?.values.orEmpty().keys.map { it to "" }, thinking?.current.orEmpty(),
                labels = thinking?.values.orEmpty(), enabled = enabled, commitOnRelease = true) {
                if (thinking != null) onSelect(thinking.id, it)
            }
        }
    }
}
