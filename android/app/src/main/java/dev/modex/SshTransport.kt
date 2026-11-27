package dev.modex

import com.jcraft.jsch.ChannelSftp
import com.jcraft.jsch.ChannelExec
import com.jcraft.jsch.HostKey
import com.jcraft.jsch.HostKeyRepository
import com.jcraft.jsch.JSch
import com.jcraft.jsch.Session
import com.jcraft.jsch.UserInfo
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.nio.charset.StandardCharsets
import org.json.JSONObject

data class Connection(val host: String, val port: Int, val user: String, val pin: String, val binary: String, val state: String)

internal const val INVALID_CONNECTION = "Check the address, port, fingerprint and any custom paths."

internal data class HostFormErrors(
    val host: String = "",
    val port: String = "",
    val user: String = "",
    val pin: String = "",
    val binary: String = "",
    val state: String = "",
    val key: String = "",
) {
    fun isEmpty() = listOf(host, port, user, pin, binary, state, key).all(String::isEmpty)
}

internal fun validateHostForm(
    host: String,
    port: String,
    user: String,
    pin: String,
    binary: String,
    state: String,
    keyLoaded: Boolean,
): HostFormErrors {
    fun invalidPath(value: String) = value.isNotEmpty() &&
        (!value.startsWith('/') || value.any { it == '\n' || it == '\r' || it == '\u0000' })
    val address = host.trim()
    val portNumber = port.trim().takeIf { it.all(Char::isDigit) }?.toIntOrNull()
    return HostFormErrors(
        host = if (address.isEmpty() || address.length > 253 || address.any { it.isWhitespace() || it.isISOControl() }) "Enter a hostname or IP address without spaces." else "",
        port = if (portNumber !in 1..65535) "Enter a port from 1 to 65535." else "",
        user = if (user.trim().isEmpty()) "Enter the SSH username." else "",
        pin = if (!pin.trim().matches(Regex("SHA256:[A-Za-z0-9+/]{43}"))) "Paste the verified SHA256 fingerprint." else "",
        binary = if (invalidPath(binary.trim())) "Use an absolute path or leave blank." else "",
        state = if (invalidPath(state.trim())) "Use an absolute path or leave blank." else "",
        key = if (keyLoaded) "" else "Import an unencrypted SSH private key.",
    )
}

internal fun Connection.isValid(): Boolean =
    validateHostForm(host, port.toString(), user, pin, binary, state, true).isEmpty()

private fun JSch.loadIdentity(key: ByteArray) {
    addIdentity("modex-memory-key", key, null, null)
    require(identityRepository.identities.any { !it.isEncrypted })
}

internal fun isUnencryptedSshKey(key: ByteArray): Boolean {
    if (key.size !in 1..(256 * 1024)) return false
    val copy = key.copyOf()
    val jsch = JSch()
    return try {
        jsch.loadIdentity(copy)
        true
    } catch (_: Exception) {
        false
    } finally {
        copy.fill(0)
        jsch.removeAllIdentity()
    }
}

internal fun validateImportedKey(key: ByteArray): String? {
    if (isUnencryptedSshKey(key)) return null
    key.fill(0)
    return "Import a supported unencrypted SSH private key."
}

class SshTransport {
    @Volatile private var session: Session? = null
    @Volatile private var channel: ChannelExec? = null
    @Volatile private var closed = false
    private val writeLock = Any()
    private val fileLock = Any()
    @Volatile private var fileChannel: ChannelSftp? = null

    internal fun open(config: Connection, key: ByteArray, journal: JournalState, onHello: (BackendCapabilities) -> Unit, onFrame: (JSONObject) -> Unit) {
        var stage = "config"
        var heartbeat: Thread? = null
        val remoteError = object : java.io.OutputStream() {
            private val bytes = ByteArrayOutputStream()
            @Synchronized override fun write(value: Int) { if (bytes.size() < 4096) bytes.write(value) }
            @Synchronized fun text() = bytes.toString("UTF-8")
        }
        try {
            require(config.host.isNotBlank() && config.user.isNotBlank() && config.port in 1..65535)
            require(config.pin.matches(Regex("SHA256:[A-Za-z0-9+/]{43}"))) { "Enter the verified SHA256 fingerprint" }
            val command = remoteCommand(config.binary, config.state, journal.cursor, journal.instance)
            val jsch = JSch()
            jsch.setHostKeyRepository(object : HostKeyRepository {
                override fun check(host: String?, key: ByteArray?): Int {
                    val matches = key != null && MessageDigestEqual(config.pin, fingerprint(key))
                    if (!matches) stage = "fingerprint"
                    return if (matches) HostKeyRepository.OK else HostKeyRepository.CHANGED
                }
                override fun add(hostkey: HostKey?, ui: UserInfo?) = Unit
                override fun remove(host: String?, type: String?) = Unit
                override fun remove(host: String?, type: String?, key: ByteArray?) = Unit
                override fun getKnownHostsRepositoryID() = "Modex pinned host key"
                override fun getHostKey(): Array<HostKey> = emptyArray()
                override fun getHostKey(host: String?, type: String?): Array<HostKey> = emptyArray()
            })
            stage = "key"
            try { jsch.loadIdentity(key) }
            catch (failure: Exception) { jsch.removeAllIdentity(); throw failure }
            val ssh = jsch.getSession(config.user, config.host, config.port)
            session = ssh
            try {
                ssh.setConfig("StrictHostKeyChecking", "yes")
                ssh.setConfig("PreferredAuthentications", "publickey")
                ssh.serverAliveInterval = 15000
                ssh.serverAliveCountMax = 3
                check(!closed)
                stage = "ssh"
                ssh.connect(15000)
                check(!closed)
                stage = "attach"
                val exec = ssh.openChannel("exec") as ChannelExec
                channel = exec
                exec.setPty(false)
                exec.setCommand(command)
                exec.setErrStream(remoteError)
                val input = exec.inputStream.buffered()
                check(!closed)
                exec.connect(15000)
                check(!closed)
                val first = readFrame(input) ?: error("Supervisor closed before attach")
                journal.hello(first)
                stage = "stream"
                heartbeat = Thread({
                    try {
                        while (!closed && exec.isConnected) {
                            Thread.sleep(15_000)
                            synchronized(writeLock) {
                                if (closed || !exec.isConnected) return@synchronized
                                val output = exec.outputStream
                                output.write("{\"heartbeat\":true}\n".toByteArray(StandardCharsets.UTF_8))
                                output.flush()
                            }
                        }
                    } catch (_: InterruptedException) {
                    } catch (_: Exception) {
                        exec.disconnect()
                    }
                }, "modex-attach-heartbeat").apply { isDaemon = true; start() }
                onHello(journal.backendCapabilities)
                while (!Thread.currentThread().isInterrupted) {
                    val frame = readFrame(input) ?: error("Supervisor stream closed")
                    onFrame(frame)
                }
            } finally {
                heartbeat?.interrupt()
                channel?.disconnect()
                ssh.disconnect()
                jsch.removeAllIdentity()
            }
        } catch (error: ConnectionFailure) {
            throw error
        } catch (error: Exception) {
            if (closed) return
            throw connectionFailure(stage, error, remoteError.text())
        }
    }

    fun send(command: JSONObject) = synchronized(writeLock) {
        val exec = channel ?: error("Disconnected")
        require(exec.isConnected) { "Disconnected" }
        val bytes = (command.toString() + "\n").toByteArray(StandardCharsets.UTF_8)
        require(bytes.size < 4 * 1024 * 1024) { "Message too large" }
        val output = exec.outputStream // JSch creates a new buffered stream on each getter call.
        output.write(bytes)
        output.flush()
    }

    internal fun <T> withFiles(action: (ChannelSftp) -> T): T = synchronized(fileLock) {
        check(!closed)
        val ssh = session ?: error("Disconnected")
        // Session.isConnected becomes true before the SSH handshake finishes.
        check(ssh.isConnected && channel?.isConnected == true) { "SSH attach is not ready" }
        val files = ssh.openChannel("sftp") as ChannelSftp
        fileChannel = files
        try {
            files.connect(15000)
            check(!closed)
            action(files)
        } finally { files.disconnect(); fileChannel = null }
    }

    fun close() { closed = true; fileChannel?.disconnect(); channel?.disconnect(); session?.disconnect() }
}

private fun MessageDigestEqual(expected: String, actual: String) = java.security.MessageDigest.isEqual(expected.toByteArray(), actual.toByteArray())

internal fun readFrame(input: InputStream): JSONObject? {
    val bytes = ByteArrayOutputStream()
    while (true) {
        val next = input.read()
        if (next < 0) { require(bytes.size() == 0) { "Truncated frame" }; return null }
        if (next == 10) return JSONObject(bytes.toString("UTF-8"))
        require(bytes.size() < 32 * 1024 * 1024) { "Frame limit exceeded" }
        bytes.write(next)
    }
}
