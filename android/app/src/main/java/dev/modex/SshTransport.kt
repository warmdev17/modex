package dev.modex

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

// Connection details are intentionally memory-only in this first slice.
data class Connection(val host: String, val port: Int, val user: String, val pin: String, val binary: String, val state: String)

class SshTransport {
    @Volatile private var session: Session? = null
    @Volatile private var channel: ChannelExec? = null
    @Volatile private var closed = false
    private val writeLock = Any()

    fun open(config: Connection, key: ByteArray, journal: JournalState, onHello: () -> Unit, onFrame: (JSONObject) -> Unit) {
        require(config.host.isNotBlank() && config.user.isNotBlank() && config.port in 1..65535)
        require(config.pin.matches(Regex("SHA256:[A-Za-z0-9+/]{43}"))) { "Enter the verified SHA256 fingerprint" }
        val command = remoteCommand(config.binary, config.state, journal.cursor, journal.instance)
        val jsch = JSch()
        jsch.setHostKeyRepository(object : HostKeyRepository {
            override fun check(host: String?, key: ByteArray?): Int = if (key != null && MessageDigestEqual(config.pin, fingerprint(key))) HostKeyRepository.OK else HostKeyRepository.CHANGED
            override fun add(hostkey: HostKey?, ui: UserInfo?) = Unit
            override fun remove(host: String?, type: String?) = Unit
            override fun remove(host: String?, type: String?, key: ByteArray?) = Unit
            override fun getKnownHostsRepositoryID() = "Modex pinned host key"
            override fun getHostKey(): Array<HostKey> = emptyArray()
            override fun getHostKey(host: String?, type: String?): Array<HostKey> = emptyArray()
        })
        jsch.addIdentity("modex-memory-key", key, null, null)
        val ssh = jsch.getSession(config.user, config.host, config.port)
        session = ssh
        try {
            ssh.setConfig("StrictHostKeyChecking", "yes")
            ssh.setConfig("PreferredAuthentications", "publickey")
            ssh.serverAliveInterval = 15000
            ssh.serverAliveCountMax = 3
            check(!closed)
            ssh.connect(15000)
            check(!closed)
            val exec = ssh.openChannel("exec") as ChannelExec
            channel = exec
            exec.setPty(false)
            exec.setCommand(command)
            exec.setErrStream(object : java.io.OutputStream() { override fun write(value: Int) = Unit })
            val input = exec.inputStream
            check(!closed)
            exec.connect(15000)
            check(!closed)
            val first = readFrame(input) ?: error("Supervisor closed before attach")
            journal.hello(first)
            onHello()
            while (!Thread.currentThread().isInterrupted) {
                val frame = readFrame(input) ?: break
                onFrame(frame)
            }
        } finally {
            channel?.disconnect()
            ssh.disconnect()
            jsch.removeAllIdentity()
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

    fun close() { closed = true; channel?.disconnect(); session?.disconnect() }
}

private fun MessageDigestEqual(expected: String, actual: String) = java.security.MessageDigest.isEqual(expected.toByteArray(), actual.toByteArray())

private fun readFrame(input: InputStream): JSONObject? {
    val bytes = ByteArrayOutputStream()
    while (true) {
        val next = input.read()
        if (next < 0) { require(bytes.size() == 0) { "Truncated frame" }; return null }
        if (next == 10) return JSONObject(bytes.toString("UTF-8"))
        require(bytes.size() < 4 * 1024 * 1024) { "Frame limit exceeded" }
        bytes.write(next)
    }
}
