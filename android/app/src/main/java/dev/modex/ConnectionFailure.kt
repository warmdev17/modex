package dev.modex

internal class ConnectionFailure(message: String) : Exception(message)

// Classify locally; never display raw SSH errors, stderr, keys or endpoint tokens.
internal fun connectionFailure(stage: String, error: Exception, stderr: String = ""): ConnectionFailure {
    val detail = (stderr + "\n" + error.message.orEmpty()).lowercase()
    val message = when {
        stage == "fingerprint" -> "SSH host fingerprint does not match. Verify the negotiated host key on the target."
        stage == "key" -> "SSH key could not be loaded. Import a supported unencrypted private key."
        stage == "config" -> "Invalid connection settings. Check hostname, port, SHA256 fingerprint and absolute host/state paths."
        stage == "ssh" && ("auth fail" in detail || "authentication" in detail) -> "SSH authentication failed. Check SSH username and whether this key is authorized on the target."
        stage == "ssh" -> "SSH connection failed. Check the current host IP, port, network, sshd and SSH algorithm compatibility."
        stage == "attach" && "another client is attached" in detail -> "Supervisor already has another client attached. Disconnect that client, then reconnect here."
        (stage == "attach" || stage == "stream") && listOf("supervisor instance changed", "supervisor restarted", "cursor ahead of supervisor journal", "cursor older than retained events", "journal gap.", "replay unavailable.", "replay gap:").any { it in detail } -> "Supervisor restarted or journal replay is unavailable. Review session state, then Reset session cursor. Do not resend uncertain prompts."
        stage == "attach" && "supervisor is unavailable" in detail -> "SSH connected, but the supervisor is unavailable. Check the host state directory and start the supervisor."
        stage == "attach" && ("not found" in detail || "no such file" in detail || "unknown command:" in detail || "permission denied" in detail) -> "SSH connected, but the remote host binary could not run. Check its absolute path and executable permission; /tmp files may disappear after reboot."
        stage == "attach" -> "SSH connected, but supervisor attach failed. Check the remote binary, private state directory and running supervisor."
        else -> "Supervisor connection stopped or returned an invalid frame. Reconnect to observe; do not resend uncertain prompts."
    }
    return ConnectionFailure(message)
}
