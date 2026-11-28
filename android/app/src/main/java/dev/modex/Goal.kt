package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import org.json.JSONObject

internal data class Goal(
    val threadId: String,
    val objective: String,
    val status: String,
    val tokenBudget: Long?,
    val tokensUsed: Long,
    val timeUsedSeconds: Long,
    val createdAt: Long,
    val updatedAt: Long,
)

private val goalStatuses = setOf("active", "paused", "blocked", "usageLimited", "budgetLimited", "complete")

private fun readGoal(value: JSONObject, expectedThread: String, native: Boolean): Goal {
    fun key(snake: String, camel: String) = if (native) camel else snake
    fun string(snake: String, camel: String): String {
        val name = key(snake, camel)
        require(value.has(name) && !value.isNull(name) && value.opt(name) is String)
        return value.getString(name)
    }
    fun number(snake: String, camel: String): Long {
        val name = key(snake, camel)
        require(value.has(name) && !value.isNull(name))
        return value.get(name).toString().toLongOrNull() ?: throw IllegalArgumentException("Invalid goal number")
    }
    val thread = string("thread_id", "threadId")
    val objective = string("objective", "objective")
    val status = string("status", "status")
    val budgetKey = key("token_budget", "tokenBudget")
    val budget = if (!value.has(budgetKey) || value.isNull(budgetKey)) null
        else value.get(budgetKey).toString().toLongOrNull()?.takeIf { it > 0 }
            ?: throw IllegalArgumentException("Invalid goal budget")
    val tokens = number("tokens_used", "tokensUsed")
    val time = number("time_used_seconds", "timeUsedSeconds")
    val created = number("created_at", "createdAt")
    val updated = number("updated_at", "updatedAt")
    require(thread == expectedThread && validGoalObjective(objective) && status in goalStatuses)
    require(tokens >= 0 && time >= 0 && created >= 0 && updated >= 0)
    return Goal(thread, objective, status, budget, tokens, time, created, updated)
}

internal fun readGoalResult(result: JSONObject, expectedThread: String): Goal? {
    require(result.has("goal"))
    if (result.isNull("goal")) return null
    return readGoal(result.getJSONObject("goal"), expectedThread, native = false)
}

internal fun readNativeGoal(params: JSONObject): Goal {
    val thread = params.optString("threadId")
    require(thread.isNotBlank())
    return readGoal(params.getJSONObject("goal"), thread, native = true)
}

internal fun validGoalObjective(value: String): Boolean =
    value.trim().isNotEmpty() && '\u0000' !in value && value.toByteArray().size <= 32768

internal fun validGoalBudget(value: String): Boolean =
    value.isBlank() || value.trim().toLongOrNull()?.let { it > 0 } == true

internal fun validGoalUpdateBudget(goal: Goal, value: String): Boolean =
    validGoalBudget(value) && (goal.status != "budgetLimited" ||
        value.trim().toLongOrNull()?.let { it > maxOf(goal.tokensUsed, goal.tokenBudget ?: 0L) } == true)

internal fun goalThreadParams(thread: String, workspace: String): JSONObject {
    require(thread.isNotBlank())
    return JSONObject().put("thread_id", thread).apply { if (workspace.isNotBlank()) put("workspace", workspace) }
}

internal fun goalSetParams(
    thread: String,
    workspace: String,
    objective: String = "",
    budget: String = "",
    status: String = "",
): JSONObject = goalThreadParams(thread, workspace).apply {
    require(objective.isNotBlank() || status.isNotBlank())
    if (objective.isNotBlank()) {
        require(validGoalObjective(objective))
        put("objective", objective.trim())
    }
    if (status.isNotBlank()) {
        require(status in setOf("active", "paused"))
        put("status", status)
    }
    require(validGoalBudget(budget))
    if (budget.isNotBlank()) put("token_budget", budget.trim().toLong())
}

private fun goalStatusLabel(status: String) = when (status) {
    "active" -> "Active"
    "paused" -> "Paused"
    "blocked" -> "Blocked"
    "usageLimited" -> "Usage limited"
    "budgetLimited" -> "Budget reached"
    "complete" -> "Complete"
    else -> "Unknown"
}

private fun goalDuration(seconds: Long): String {
    val hours = seconds / 3600
    val minutes = seconds % 3600 / 60
    val remaining = seconds % 60
    return listOfNotNull("${hours}h".takeIf { hours > 0 }, "${minutes}m".takeIf { minutes > 0 }, "${remaining}s".takeIf { remaining > 0 || seconds == 0L }).joinToString(" ")
}

@Composable
internal fun GoalDialog(
    goal: Goal?,
    loading: Boolean,
    working: Boolean,
    uncertain: Boolean,
    error: String,
    connected: Boolean,
    onRefresh: () -> Unit,
    onSet: (String, String, String) -> Unit,
    onClear: () -> Unit,
    onClose: () -> Unit,
) {
    var objective by rememberSaveable(goal?.threadId) { mutableStateOf("") }
    var budget by rememberSaveable(goal?.threadId) { mutableStateOf("") }
    var editing by rememberSaveable(goal?.threadId) { mutableStateOf(false) }
    var confirmClear by rememberSaveable(goal?.threadId) { mutableStateOf(false) }
    val canAct = connected && !loading && !working && !uncertain
    if (confirmClear && goal != null) AlertDialog(
        onDismissRequest = { confirmClear = false },
        title = { Text("Clear goal?") },
        text = { Text("This removes the goal and stops future continuation. It does not interrupt the current turn.") },
        confirmButton = { TextButton(enabled = canAct, onClick = { confirmClear = false; onClear() }) { Text("Clear", color = Mocha.Red) } },
        dismissButton = { TextButton(onClick = { confirmClear = false }) { Text("Cancel") } },
    )
    AlertDialog(
        onDismissRequest = onClose,
        title = {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (loading || working) CircularProgressIndicator(
                    Modifier.size(18.dp).semantics { contentDescription = if (working) "Updating goal" else "Refreshing goal" },
                    strokeWidth = 2.dp,
                )
                Text("Goal")
            }
        },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                if (error.isNotBlank()) Text(error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                if (!connected) Text("Reconnect to manage this goal.", color = Mocha.Subtext)
                if (uncertain) Text("Refresh to confirm the current goal before making another change.", color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                if (goal == null) {
                    Text(if (loading) "Loading goal…" else "No goal is configured for this chat.", color = Mocha.Subtext)
                    OutlinedTextField(
                        objective, { objective = it }, enabled = canAct, label = { Text("Objective") },
                        supportingText = { if (objective.isNotEmpty() && !validGoalObjective(objective)) Text("Enter an objective up to 32 KiB.") },
                        minLines = 2, maxLines = 5, modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        budget, { budget = it }, enabled = canAct, label = { Text("Token budget (optional)") }, singleLine = true,
                        supportingText = { if (budget.isNotEmpty() && !validGoalBudget(budget)) Text("Enter a positive whole number.") },
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth(),
                    )
                    Button(
                        enabled = canAct && validGoalObjective(objective) && validGoalBudget(budget),
                        onClick = { onSet(objective, budget, "") }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                    ) { Text("Create goal") }
                } else {
                    Text(goalStatusLabel(goal.status), style = MaterialTheme.typography.titleSmall, color = if (goal.status == "active") Mocha.Blue else Mocha.Text)
                    if (editing) {
                        OutlinedTextField(
                            objective, { objective = it }, enabled = canAct, label = { Text("Objective") },
                            supportingText = { if (!validGoalObjective(objective)) Text("Enter an objective up to 32 KiB.") },
                            minLines = 2, maxLines = 5, modifier = Modifier.fillMaxWidth(),
                        )
                        OutlinedTextField(
                            budget, { budget = it }, enabled = canAct, label = { Text("Token budget (optional)") }, singleLine = true,
                            supportingText = {
                                when {
                                    goal.status == "budgetLimited" && !validGoalUpdateBudget(goal, budget) -> Text("Enter a new budget above ${java.text.NumberFormat.getIntegerInstance().format(maxOf(goal.tokensUsed, goal.tokenBudget ?: 0L))} tokens.")
                                    !validGoalBudget(budget) -> Text("Enter a positive whole number.")
                                    budget.isBlank() && goal.tokenBudget != null -> Text("Leave blank to keep the current budget.")
                                }
                            },
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth(),
                        )
                        Button(
                            enabled = canAct && validGoalObjective(objective) && validGoalUpdateBudget(goal, budget),
                            onClick = {
                                onSet(objective, budget, if (goal.status == "budgetLimited") "active" else "")
                                editing = false
                            },
                            modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                        ) { Text(if (goal.status == "budgetLimited") "Update & resume" else "Save changes") }
                        TextButton(onClick = { editing = false }, modifier = Modifier.heightIn(min = 48.dp)) { Text("Cancel edit") }
                    } else Text(goal.objective)
                    HorizontalDivider(color = Mocha.Surface)
                    val format = java.text.NumberFormat.getIntegerInstance()
                    Text(
                        if (goal.tokenBudget == null) "${format.format(goal.tokensUsed)} tokens used"
                        else "${format.format(goal.tokensUsed)} / ${format.format(goal.tokenBudget)} tokens",
                    )
                    Text(goalDuration(goal.timeUsedSeconds), color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall)
                    Text(
                        "Active goals may continue when reopening this chat; Pause stops future continuation, Stop interrupts the current turn.",
                        color = Mocha.Subtext, style = MaterialTheme.typography.bodySmall,
                    )
                    if (!editing) Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        if (goal.status == "active") OutlinedButton(
                            enabled = canAct, onClick = { onSet("", "", "paused") }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                        ) { Text("Pause") }
                        else if (goal.status in setOf("paused", "blocked", "usageLimited")) Button(
                            enabled = canAct, onClick = { onSet("", "", "active") }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                        ) { Text("Resume") }
                        OutlinedButton(
                            enabled = canAct, onClick = {
                                objective = goal.objective
                                budget = goal.tokenBudget?.toString().orEmpty()
                                editing = true
                            }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                        ) { Text(if (goal.status == "budgetLimited") "Increase budget" else "Edit goal") }
                        OutlinedButton(
                            enabled = canAct, onClick = { confirmClear = true }, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                        ) { Text("Clear goal", color = if (canAct) Mocha.Red else Mocha.Subtext) }
                    }
                }
                TextButton(enabled = connected && !loading && !working, onClick = onRefresh, modifier = Modifier.heightIn(min = 48.dp)) { Text("Refresh") }
            }
        },
        confirmButton = { TextButton(onClick = onClose) { Text("Close") } },
    )
}
