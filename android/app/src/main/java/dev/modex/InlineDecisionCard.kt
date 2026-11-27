package dev.modex

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.selectable
import androidx.compose.ui.semantics.Role
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import org.json.JSONArray
import org.json.JSONObject

@Composable
internal fun InlineDecisionCard(decision: JSONObject, responding: Boolean, enabled: Boolean, respond: (JSONObject) -> Unit) {
    val context = decision.getJSONObject("context")
    val fileMode = decision.optString("method") == "item/fileChange/requestApproval"
    val questionMode = decision.optString("method") == "item/tool/requestUserInput"
    val questions = context.optJSONArray("questions") ?: JSONArray()
    val queuedQuestion = decision.optString("decision_id").startsWith("queued:")
    val identity = decision.toString()
    var answers by rememberSaveable(identity) { mutableStateOf(hashMapOf<String, String>()) }
    var selections by rememberSaveable(identity) { mutableStateOf(hashMapOf<String, Int>()) }
    fun select(id: String, index: Int) { selections = HashMap(selections).apply { put(id, index) } }
    fun answer(id: String, value: String) { answers = HashMap(answers).apply { put(id, value) } }
    val offered = context.optJSONArray("availableDecisions")
    fun offered(choice: String): Boolean = offered == null || (0 until offered.length()).any { offered.opt(it) == choice }

    Surface(
        color = Mocha.Mantle,
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, Mocha.Surface),
        modifier = Modifier.fillMaxWidth().semantics { contentDescription = if (questionMode) "Agent input request" else "Pending approval" }
    ) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                DecisionGlyph(questionMode)
                Column(Modifier.weight(1f)) {
                    Text(if (queuedQuestion) "Queued question" else if (questionMode) "Agent needs your input" else if (fileMode) "Review file access" else "Permission required", style = MaterialTheme.typography.titleSmall)
                    if (!queuedQuestion || !enabled) Text(if (enabled) "Agent is waiting" else "Reconnect to respond", color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
                }
                if (responding) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            }
            if (questionMode) {
                for (i in 0 until questions.length()) {
                    val question = questions.getJSONObject(i)
                    val id = question.getString("id")
                    Text(question.getString("question"), style = MaterialTheme.typography.bodyMedium)
                    val options = question.optJSONArray("options")
                    val hasOptions = options != null && options.length() > 0
                    if (hasOptions) {
                        for (n in 0 until options!!.length()) {
                            val option = options.getJSONObject(n)
                            QuestionOption(option.getString("label"), option.optString("description"),
                                selections[id] == n, enabled && !responding) { select(id, n) }
                        }
                        QuestionOption("Other", "Write your own answer", selections[id] == -1,
                            enabled && !responding) { select(id, -1) }
                    }
                    if (!hasOptions || selections[id] == -1) OutlinedTextField(
                        answers[id].orEmpty(), { answer(id, it) }, enabled = enabled && !responding,
                        label = { Text("Your answer") }, modifier = Modifier.fillMaxWidth(), minLines = 2
                    )
                }
                val result = questionAnswerResult(questions, selections, answers)
                Button(
                    enabled = enabled && !responding && result != null,
                    onClick = { if (enabled && !responding && result != null) respond(result) },
                    modifier = Modifier.align(Alignment.End).heightIn(min = 48.dp)
                ) { Text(if (responding) "Sending…" else "Submit answer") }
            } else {
                Text(
                    if (fileMode) "File changes cannot be allowed until a diff viewer is available. You can decline or cancel this request."
                    else "Allow this action once. Future commands still require their own approval.",
                    color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall
                )
                for ((label, value) in approvalDetails(context)) {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(label, color = Mocha.Subtext, style = MaterialTheme.typography.labelSmall)
                        if (label == "Command") Surface(color = Mocha.Surface, shape = MaterialTheme.shapes.small) {
                            SelectionContainer { Text(value, Modifier.fillMaxWidth().padding(10.dp), fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodySmall) }
                        } else SelectionContainer { Text(value, style = MaterialTheme.typography.bodySmall) }
                    }
                }
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End), verticalAlignment = Alignment.CenterVertically) {
                    if (offered("decline")) TextButton(onClick = { respond(JSONObject().put("decision", "decline")) }, enabled = enabled && !responding, modifier = Modifier.heightIn(min = 48.dp)) { Text("Decline") }
                    if (offered("cancel")) TextButton(onClick = { respond(JSONObject().put("decision", "cancel")) }, enabled = enabled && !responding, modifier = Modifier.heightIn(min = 48.dp)) { Text("Cancel") }
                    if (!fileMode && offered("accept")) Button(onClick = { respond(JSONObject().put("decision", "accept")) }, enabled = enabled && !responding, modifier = Modifier.heightIn(min = 48.dp)) { Text("Allow once") }
                }
            }
        }
    }
}

@Composable
private fun DecisionGlyph(question: Boolean) {
    val color = Mocha.Blue
    Canvas(Modifier.size(20.dp)) {
        val stroke = 1.7.dp.toPx()
        if (question) {
            drawCircle(color, size.minDimension / 2 - stroke, center, style = androidx.compose.ui.graphics.drawscope.Stroke(stroke))
            drawLine(color, Offset(size.width * .42f, size.height * .38f), Offset(size.width * .5f, size.height * .31f), stroke, StrokeCap.Round)
            drawLine(color, Offset(size.width * .5f, size.height * .31f), Offset(size.width * .62f, size.height * .4f), stroke, StrokeCap.Round)
            drawLine(color, Offset(size.width * .62f, size.height * .4f), Offset(size.width * .5f, size.height * .55f), stroke, StrokeCap.Round)
            drawCircle(color, stroke / 1.6f, Offset(size.width * .5f, size.height * .72f))
        } else {
            val path = androidx.compose.ui.graphics.Path().apply {
                moveTo(size.width * .5f, stroke)
                lineTo(size.width - stroke, size.height * .22f)
                lineTo(size.width * .83f, size.height * .68f)
                lineTo(size.width * .5f, size.height - stroke)
                lineTo(size.width * .17f, size.height * .68f)
                lineTo(stroke, size.height * .22f)
                close()
            }
            drawPath(path, color, style = androidx.compose.ui.graphics.drawscope.Stroke(stroke, join = androidx.compose.ui.graphics.StrokeJoin.Round))
            drawLine(color, Offset(size.width * .5f, size.height * .28f), Offset(size.width * .5f, size.height * .58f), stroke, StrokeCap.Round)
            drawCircle(color, stroke / 1.6f, Offset(size.width * .5f, size.height * .72f))
        }
    }
}

@Composable
private fun QuestionOption(label: String, description: String, selected: Boolean, enabled: Boolean, onClick: () -> Unit) {
    Surface(color = if (selected) Mocha.Surface else Mocha.Mantle,
        border = BorderStroke(1.dp, if (selected) Mocha.Blue else Mocha.Surface),
        shape = MaterialTheme.shapes.small) {
        Row(Modifier.fillMaxWidth().heightIn(min = 48.dp)
            .selectable(selected = selected, enabled = enabled, role = Role.RadioButton, onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            RadioButton(selected = selected, onClick = null, enabled = enabled)
            Column(Modifier.weight(1f).padding(start = 8.dp)) {
                Text(label, color = if (selected) Mocha.Blue else Mocha.Text, style = MaterialTheme.typography.bodyMedium)
                if (description.isNotBlank()) Text(description, color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

internal fun questionAnswerResult(questions: JSONArray, selections: Map<String, Int>, drafts: Map<String, String>): JSONObject? {
    if (questions.length() == 0) return null
    val answers = JSONObject()
    for (i in 0 until questions.length()) {
        val question = questions.getJSONObject(i)
        val id = question.getString("id")
        val options = question.optJSONArray("options")
        val answer = if (options != null && options.length() > 0) {
            val selection = selections[id] ?: return null
            if (selection == -1) drafts[id] else options.optJSONObject(selection)?.optString("label")
        } else drafts[id]
        if (answer.isNullOrBlank()) return null
        answers.put(id, JSONObject().put("answers", JSONArray().put(answer)))
    }
    return JSONObject().put("answers", answers)
}
