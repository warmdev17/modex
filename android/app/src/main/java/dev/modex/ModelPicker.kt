package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import kotlin.math.roundToInt

internal fun reasoningColor(effort: String): Color = when (effort) {
    "none", "minimal", "low" -> Color(0xFFA6E3A1)
    "medium" -> Mocha.Blue
    "high" -> Color(0xFFF9E2AF)
    "xhigh" -> Color(0xFFFAB387)
    "max" -> Color(0xFFCBA6F7)
    "ultra" -> Mocha.Red
    "" -> Mocha.Subtext
    else -> Color(0xFFCBA6F7)
}

@Composable
internal fun ModelPicker(models: List<ModelOption>, status: String, value: String, onChange: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    var custom by remember { mutableStateOf(value.isNotEmpty() && models.none { it.model == value }) }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Box {
            OutlinedButton(onClick = { expanded = true }, shape = RoundedCornerShape(8.dp), modifier = Modifier.fillMaxWidth()) {
                Text(models.firstOrNull { it.model == value }?.name ?: value.ifBlank { "Choose model" }, modifier = Modifier.weight(1f))
                Text("▾")
            }
            DropdownMenu(expanded, { expanded = false }, modifier = Modifier.heightIn(max = 280.dp)) {
                models.forEach { model ->
                    DropdownMenuItem(text = { Column { Text(model.name); if (model.name != model.model) Text(model.model, style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext) } }, onClick = { custom = false; onChange(model.model); expanded = false })
                }
                DropdownMenuItem(text = { Text("Custom model ID…") }, onClick = { custom = true; expanded = false })
            }
        }
        if (status.isNotEmpty()) Text(status, style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        if (custom) OutlinedTextField(value, onChange, singleLine = true, label = { Text("Model ID") }, isError = !validModelId(value), modifier = Modifier.fillMaxWidth())
    }
}

@Composable
internal fun ReasoningControl(model: ModelOption?, value: String, onChange: (String) -> Unit) {
    val efforts = model?.efforts.orEmpty()
    if (efforts.isEmpty()) {
        Text("Reasoning · ${value.ifBlank { "Unavailable for this model" }}", style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        return
    }
    val choices = efforts
    val index = choices.indexOfFirst { it.first == value }.takeIf { it >= 0 } ?: choices.indexOfFirst { it.first == model?.defaultEffort }.coerceAtLeast(0)
    val color = reasoningColor(value)
    Column {
        Text("Reasoning · ${value.ifBlank { choices[index].first }}", color = color, style = MaterialTheme.typography.labelLarge)
        if (choices.size > 1) Slider(value = index.toFloat(), onValueChange = { onChange(choices[it.roundToInt().coerceIn(choices.indices)].first) },
            valueRange = 0f..choices.lastIndex.toFloat(), steps = (choices.size - 2).coerceAtLeast(0),
            colors = SliderDefaults.colors(thumbColor = color, activeTrackColor = color),
            modifier = Modifier.semantics { contentDescription = "Reasoning effort" })
        Box(Modifier.fillMaxWidth()) {
            // Reserve the tallest description at the current width and font scale.
            choices.forEach { (_, description) ->
                Text(description, style = MaterialTheme.typography.bodySmall, color = Color.Transparent, modifier = Modifier.clearAndSetSemantics {})
            }
            Text(choices[index].second, style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        }
        if (model != null && model.defaultEffort.isNotBlank()) Text("Model default: ${model.defaultEffort}", style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
    }
}
