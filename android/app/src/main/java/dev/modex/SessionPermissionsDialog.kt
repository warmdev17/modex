package dev.modex

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp

@Composable
internal fun SessionPermissionsDialog(
    status: SessionStatus?,
    profiles: List<SessionPermissionProfile>,
    working: Boolean,
    message: String,
    canApply: Boolean,
    onRequest: (String) -> Unit,
    onClose: () -> Unit,
) {
    var chosen by remember { mutableStateOf("") }
    val active = status?.permissionProfile.orEmpty()
    val current = profiles.find { it.id == active }?.label ?: active.ifBlank {
        when (status?.sandbox) {
            "readOnly" -> "Read-only"
            "workspaceWrite" -> "Workspace write"
            "dangerFullAccess" -> "Full access"
            "externalSandbox" -> "External sandbox"
            else -> "Unknown"
        }
    }
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text("Session settings") },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("Permissions", style = MaterialTheme.typography.titleSmall)
                Text("Current: $current")
                if (status != null) {
                    Text("Approvals: ${status.approvalPolicy.ifBlank { "Unknown" }} · Reviewer: ${status.approvalsReviewer.ifBlank { "Unknown" }}",
                        style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
                    Text(listOf(status.provider, status.model).filter { it.isNotBlank() }.joinToString(" · "),
                        style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
                }
                Text(if (canApply) "Changes apply to subsequent turns in this session." else "Wait for the session to be idle and resolve pending decisions.",
                    style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
                Column(Modifier.selectableGroup()) {
                    profiles.forEach { profile ->
                        val selected = profile.id == chosen.ifBlank { active }
                        val enabled = profile.allowed && canApply && !working
                        Row(Modifier.fillMaxWidth().heightIn(min = 48.dp).selectable(selected, enabled = enabled, role = Role.RadioButton) { chosen = profile.id },
                            verticalAlignment = Alignment.CenterVertically) {
                            RadioButton(selected, onClick = null, enabled = enabled, modifier = Modifier.padding(end = 12.dp))
                            Column {
                                Text(profile.label)
                                val description = if (!profile.allowed) "Restricted by host policy" else profile.description
                                if (description.isNotBlank()) Text(description, style = MaterialTheme.typography.bodySmall, color = Mocha.Subtext)
                            }
                        }
                    }
                }
                if (chosen == ":danger-full-access") Text("Full access removes sandbox restrictions and command approval prompts.", color = Mocha.Red)
                if (working) LinearProgressIndicator(Modifier.fillMaxWidth())
                if (message.isNotBlank()) Text(message, color = Mocha.Subtext)
                TextButton(onClick = { chosen = ""; onRequest("") }, enabled = !working) { Text("Reload permissions") }
            }
        },
        confirmButton = {
            TextButton(enabled = canApply && !working && profiles.any { it.id == chosen && it.allowed },
                onClick = { onRequest(chosen); chosen = "" }) { Text("Apply") }
        },
        dismissButton = { TextButton(onClick = onClose) { Text("Close") } },
    )
}
