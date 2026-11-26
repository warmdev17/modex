package dev.modex

import android.app.Instrumentation
import android.os.Bundle
import java.io.File
import org.json.JSONObject

/** Explicitly launched by scripts/test_android_transport.py against an ephemeral sshd. */
class TransportSmoke : Instrumentation() {
    private lateinit var args: Bundle
    override fun onCreate(arguments: Bundle) { super.onCreate(arguments); args = arguments; start() }
    override fun onStart() {
        if (args.getString("preview") == "stream-render") { streamingRenderSmoke(); return }
        if (args.getString("preview") == "stream-performance") { streamingPerformanceSmoke(); return }
        if (args.getString("preview") == "composer-responsiveness") { composerResponsivenessSmoke(args); return }
        if (args.getString("preview") == "native-session-controls") { nativeSessionControlsSmoke(); return }
        if (args.getString("preview") == "opencode-capabilities") { openCodeCapabilitiesSmoke(); return }
        if (args.getString("preview") == "opencode-projects-read") { openCodeCapabilitiesSmoke(projectsRead = true); return }
        if (args.getString("preview") == "session-management") { sessionManagementSmoke(); return }
        if (args.getString("preview") == "plan-review") { planReviewSmoke(); return }
        if (args.getString("preview") == "prompt-drafts") { promptDraftSmoke(args); return }
        if (args.getString("preview") == "agent-activity") { agentActivitySmoke(); return }
        if (args.getString("preview") == "activity-navigation") { activityNavigationSmoke(); return }
        if (args.getString("preview") == "connection-management") { connectionManagementSmoke(args); return }
        if (args.getString("preview") == "legacy-connections") { legacyConnectionSmoke(); return }
        if (args.getString("preview") == "journal-recovery") { journalRecoverySmoke(args); return }
        if (args.getString("preview") == "upgrade") { upgradeSmoke(args); return }
        if (args.getString("preview") == "add-host") { addHostSmoke(args); return }
        if (args.getString("preview") == "skill-picker") { referencePickerSmoke(); return }
        if (args.getString("preview") == "large-history") { largeHistorySmoke(args); return }
        if (args.getString("preview") == "emulator-profile") { importEmulatorProfile(); return }
        if (args.getString("preview") == "attachment-preview") { attachmentPreviewSmoke(); return }
        if (args.getString("preview") == "remote-picker") { remoteFilePickerSmoke(); return }
        if (args.getString("preview") == "attachments") { attachmentTransportSmoke(args); return }
        if (args.getString("preview") == "remote-project") { remoteProjectSmoke(); return }
        if (args.getString("preview") == "new-chat") { newChatSmoke(); return }
        if (args.getString("preview") == "chat-ui") { chatUiSmoke(); return }
        if (args.getString("preview") == "markdown-table") { markdownTableSmoke(); return }
        if (args.getString("preview") == "command-timeline") { commandTimelineSmoke(); return }
        if (args.getString("preview") == "auto-follow") { autoFollowSmoke(); return }
        if (args.getString("preview") == "history-scroll") { historyScrollSmoke(); return }
        if (args.getString("preview") == "continuity") { continuitySmoke(args); return }
        if (args.getString("preview") == "storage") {
            val result = Bundle()
            val store = SavedHost(targetContext, "test-only")
            try {
                store.save(JSONObject().put("key", "PRIVATE_TEST_MARKER").put("cursor", 12))
                check(store.load()!!.getLong("cursor") == 12L)
                val base = File(targetContext.noBackupFilesDir, "test-only.enc")
                val backup = File(targetContext.noBackupFilesDir, "test-only.enc.bak")
                check(base.renameTo(backup))
                check(store.load()!!.getLong("cursor") == 12L)
                check(base.renameTo(backup))
                store.save(JSONObject().put("key", "PRIVATE_TEST_MARKER").put("cursor", 13))
                check(!backup.exists() && store.load()!!.getLong("cursor") == 13L)
                val raw = base.readBytes()
                check(!String(raw, Charsets.ISO_8859_1).contains("PRIVATE_TEST_MARKER"))
                raw[raw.lastIndex] = (raw.last().toInt() xor 1).toByte()
                File(targetContext.noBackupFilesDir, "test-only.enc").writeBytes(raw)
                var rejected = false
                try { store.load() } catch (_: Exception) { rejected = true }
                check(rejected)
                val oldRevision = store.revision()
                store.clear(); check(store.load() == null)
                store.save(JSONObject().put("key", "stale"), oldRevision)
                check(store.load() == null)
                result.putString("result", "PASS: Keystore encryption, legacy backup recovery and replacement, tamper rejection, forget")
                finish(-1, result)
            } catch (_: Throwable) { result.putString("result", "FAIL: encrypted persistence"); finish(0, result) }
            finally { store.clear() }
            return
        }
        if (args.getString("preview") == "approval") { approvalPreview(); return }
        val result = Bundle()
        try {
            val config = Connection("10.0.2.2", args.getString("port")!!.toInt(), args.getString("user")!!,
                args.getString("pin")!!, args.getString("binary")!!, args.getString("state")!!)
            val key = File(targetContext.filesDir, "smoke-key").readBytes()
            try {
                val journal = JournalState()
                for (id in 900001..900002) {
                    var received = false
                    val transport = SshTransport()
                    try {
                        transport.open(config, key, journal, onHello = {
                            transport.send(JSONObject().put("id", id).put("method", "status"))
                        }, onFrame = { frame ->
                            journal.frame(frame) { message ->
                                if (message.optInt("id") == id) {
                                    check(message.getJSONObject("result").getJSONObject("config").getString("provider") == "fixture")
                                    received = true
                                }
                            }
                            if (received) transport.close()
                        })
                    } catch (e: Exception) { if (!received) throw e }
                    finally { transport.close() }
                    check(received && journal.cursor > 0)
                    Thread.sleep(200)
                }
                var trusted = false
                val wrong = SshTransport()
                try {
                    wrong.open(config.copy(pin = fingerprint(byteArrayOf())), key, JournalState(), { trusted = true }, {})
                } catch (_: Exception) { /* Expected host key rejection. */ }
                finally { wrong.close() }
                check(!trusted)
                result.putString("result", "PASS: Android SSH status, cursor reconnect and wrong-pin rejection")
            } finally { key.fill(0); File(targetContext.filesDir, "smoke-key").delete() }
            finish(-1, result)
        } catch (_: Throwable) {
            result.putString("result", "FAIL: Android transport smoke; inspect fixture and protocol")
            finish(0, result)
        }
    }
}
