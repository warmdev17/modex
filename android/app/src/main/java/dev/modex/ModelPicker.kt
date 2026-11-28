package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
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
internal fun ModelSettingsDialog(
    onClose: () -> Unit,
    actions: @Composable () -> Unit,
    dismiss: (@Composable () -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text("Model settings") },
        text = { Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp), content = content) },
        confirmButton = actions,
        dismissButton = dismiss,
    )
}

@Composable
internal fun ModelPicker(
    models: List<ModelOption>, status: String, value: String,
    enabled: Boolean = true, allowCustom: Boolean = true, onChange: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    var custom by remember { mutableStateOf(value.isNotEmpty() && models.none { it.model == value }) }
    val selected = models.firstOrNull { it.model == value }
    LaunchedEffect(enabled) { if (!enabled) expanded = false }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Box {
            OutlinedButton(onClick = { expanded = true }, enabled = enabled && (models.isNotEmpty() || allowCustom),
                shape = RoundedCornerShape(8.dp), modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                ModelLogo(modelBrand(value, selected?.name.orEmpty()))
                Text(selected?.name ?: value.ifBlank { "Choose model" },
                    modifier = Modifier.weight(1f).padding(horizontal = 8.dp), maxLines = 2, overflow = TextOverflow.Ellipsis)
                ChatGlyph(ChatIcon.ChevronDown)
            }
            ModexDropdownMenu(expanded && enabled, { expanded = false }, modifier = Modifier.heightIn(max = 280.dp)) {
                models.forEach { model ->
                    DropdownMenuItem(
                        text = { Column {
                            Text(model.name)
                            if (allowCustom && model.name != model.model) Text(model.model, style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
                        } },
                        leadingIcon = { ModelLogo(modelBrand(model.model, model.name)) },
                        trailingIcon = if (model.model == value) ({ Text("Selected", style = MaterialTheme.typography.labelSmall, color = Mocha.Blue) }) else null,
                        onClick = { custom = false; expanded = false; onChange(model.model) },
                    )
                }
                if (allowCustom) DropdownMenuItem(text = { Text("Custom model ID…") }, leadingIcon = { ChatGlyph(ChatIcon.Edit) }, onClick = { custom = true; expanded = false })
            }
        }
        if (status.isNotEmpty()) Text(status, style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        if (allowCustom && custom) OutlinedTextField(value, onChange, enabled = enabled, singleLine = true, label = { Text("Model ID") }, isError = !validModelId(value), modifier = Modifier.fillMaxWidth())
    }
}

@Composable
internal fun ReasoningControl(model: ModelOption?, value: String, onChange: (String) -> Unit) {
    ModelLevelControl("Reasoning", "Reasoning effort", model?.efforts.orEmpty(), value,
        defaultValue = model?.defaultEffort.orEmpty(), onChange = onChange)
}

@Composable
internal fun ModelLevelControl(
    title: String, accessibilityLabel: String, choices: List<Pair<String, String>>, value: String,
    defaultValue: String = "", labels: Map<String, String> = emptyMap(), enabled: Boolean = true,
    commitOnRelease: Boolean = false, onChange: (String) -> Unit,
) {
    val acknowledged = choices.indexOfFirst { it.first == value }
    val currentLabel = labels[value] ?: value
    if (choices.isEmpty()) {
        Text("$title · ${currentLabel.ifBlank { "Unavailable for this model" }}",
            style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        return
    }
    if (commitOnRelease && acknowledged < 0) {
        // A slider thumb would imply the host acknowledged one of these choices.
        var expanded by remember(value, enabled) { mutableStateOf(false) }
        Box {
            OutlinedButton(onClick = { expanded = true }, enabled = enabled,
                shape = RoundedCornerShape(8.dp), modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Text("$title · ${currentLabel.ifBlank { "Choose a level" }}", Modifier.weight(1f))
                ChatGlyph(ChatIcon.ChevronDown)
            }
            ModexDropdownMenu(expanded && enabled, { expanded = false }, modifier = Modifier.heightIn(max = 280.dp)) {
                choices.forEach { (id, _) ->
                    DropdownMenuItem(text = { Text(labels[id] ?: id) }, onClick = { expanded = false; onChange(id) })
                }
            }
        }
        return
    }
    val index = acknowledged.takeIf { it >= 0 } ?: choices.indexOfFirst { it.first == defaultValue }.coerceAtLeast(0)
    var dragIndex by remember(choices, value, enabled) { mutableStateOf<Int?>(null) }
    val displayIndex = dragIndex ?: index
    val displayValue = choices[displayIndex].first
    val color = if (enabled) reasoningColor(displayValue) else Mocha.Subtext
    Column {
        Text("$title · ${labels[displayValue] ?: displayValue}", color = color, style = MaterialTheme.typography.labelLarge)
        if (choices.size > 1) Slider(value = displayIndex.toFloat(), enabled = enabled, onValueChange = {
            val next = it.roundToInt().coerceIn(choices.indices)
            if (commitOnRelease) dragIndex = next else onChange(choices[next].first)
        }, onValueChangeFinished = if (commitOnRelease) ({
            val next = dragIndex
            dragIndex = null
            if (enabled && next != null && next != index) onChange(choices[next].first)
        }) else null,
            valueRange = 0f..choices.lastIndex.toFloat(), steps = (choices.size - 2).coerceAtLeast(0),
            colors = SliderDefaults.colors(thumbColor = color, activeTrackColor = color),
            modifier = Modifier.semantics { contentDescription = accessibilityLabel })
        if (choices.any { it.second.isNotBlank() }) Box(Modifier.fillMaxWidth()) {
            // Reserve the tallest description at the current width and font scale.
            choices.forEach { (_, description) ->
                Text(description, style = MaterialTheme.typography.bodySmall, color = Color.Transparent, modifier = Modifier.clearAndSetSemantics {})
            }
            Text(choices[displayIndex].second, style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
        }
        if (defaultValue.isNotBlank()) Text("Model default: $defaultValue", style = MaterialTheme.typography.labelSmall, color = Mocha.Subtext)
    }
}
