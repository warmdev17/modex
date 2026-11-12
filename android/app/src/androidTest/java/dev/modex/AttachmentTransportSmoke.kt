package dev.modex

import android.app.Instrumentation
import android.net.Uri
import android.os.Bundle
import java.io.ByteArrayInputStream
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import org.json.JSONObject

/** Real pinned SSH/SFTP against the temporary server; no provider inference or user data. */
internal fun Instrumentation.attachmentTransportSmoke(args: Bundle) {
    val result = Bundle()
    val keyFile = File(targetContext.filesDir, "smoke-key")
    val key = keyFile.readBytes()
    val transport = SshTransport()
    val ready = CountDownLatch(1)
    val response = CountDownLatch(1)
    val failure = AtomicReference<Throwable?>()
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    val document = File(targetContext.cacheDir, "attachment-smoke.txt")
    val reader = Thread {
        try {
            val config = Connection("10.0.2.2", args.getString("port")!!.toInt(), args.getString("user")!!, args.getString("pin")!!, args.getString("binary")!!, args.getString("state")!!)
            val journal = JournalState()
            transport.open(config, key, journal, { ready.countDown() }, { frame -> journal.frame(frame) { message ->
                if (message.optInt("id") == 900003) {
                    check(!message.has("error")) { "Host rejected attachment turn: ${message.optJSONObject("error")?.optString("message")}" }
                    check(message.getJSONObject("result").getJSONObject("turn").getString("id") == "attachment-fixture")
                    response.countDown()
                }
            } })
        } catch (e: Throwable) { failure.set(e); ready.countDown(); response.countDown() }
    }
    try {
        reader.start()
        check(ready.await(20,TimeUnit.SECONDS)) { "SSH timeout" }
        check(failure.get() == null) { "SSH failed" }
        val root = args.getString("root")!!
        val state = args.getString("state")!!
        val listing = transport.listRemoteDirectory(root)
        check(listing.files.any { it.name == "image.png" } && listing.files.any { it.name == "notes.txt" })
        check(transport.listRemoteDirectory("$root/literal*? folder").files.single().name == "inside.txt")
        val note = transport.uploadAttachment(state, "../../report*?.txt", ByteArrayInputStream("mobile-upload".toByteArray())) {}
        check(note.name == "report*?.txt" && note.path.startsWith("$state/attachments/"))
        check(transport.withFiles { it.get(literalSftpPath(note.path)).use { stream -> stream.readBytes().decodeToString() } } == "mobile-upload")
        val previewCache = File(targetContext.cacheDir, "attachment-preview-test")
        try {
            val local = transport.downloadAttachment(note, previewCache, "fixture")
            check(local.readText() == "mobile-upload") { "Preview download changed file bytes" }
            check(transport.downloadAttachment(note, previewCache, "fixture") == local) { "Cache not reused" }
            var cancelled = false
            try { transport.downloadAttachment(note, previewCache, "cancelled") { throw java.util.concurrent.CancellationException("fixture") } } catch (_: java.util.concurrent.CancellationException) { cancelled = true }
            check(cancelled && previewCache.walkTopDown().none { it.name == ".download" }) { "Cancelled download not cleaned" }

            try { transport.downloadAttachment(Attachment(root, "folder", "file"), previewCache, "fixture"); error("Directory downloaded") } catch (_: IllegalArgumentException) { }
        } finally { previewCache.deleteRecursively() }
        check(transport.listRemoteDirectory("~").path == transport.withFiles { it.home }) { "Default home mismatch" }
        val before = transport.listRemoteDirectory("$state/attachments").files.size
        try {
            transport.uploadAttachment(state, "broken.txt", object: InputStream() { override fun read(): Int = throw IOException("fixture") }) {}
            error("Failed stream accepted")
        } catch (_: IOException) { }
        check(transport.listRemoteDirectory("$state/attachments").files.size == before) { "Partial upload not cleaned" }
        document.writeText("document-provider-stream")
        val config = Connection("fixture",22,"fixture","fixture","fixture",state)
        lateinit var drafts: AttachmentState
        runOnMainSync {
            drafts = AttachmentState(scope, { transport }, { config })
            drafts.fromPhone("origin-thread",listOf(Uri.fromFile(document)),drafts.generation,targetContext.contentResolver)
        }
        val deadline = System.currentTimeMillis() + 20000
        var working = true
        while (working && System.currentTimeMillis() < deadline) { runOnMainSync { working = drafts.working }; Thread.sleep(100) }
        runOnMainSync {
            check(!drafts.working && drafts.error.isEmpty()) { "Phone stream upload failed" }
            check(drafts.drafts.keys == setOf("origin-thread")) { "Attachment went to another thread" }
            val oldPath = drafts.drafts.getValue("origin-thread").single().path
            drafts.addRemote("origin-thread",RemoteFile("next.txt","/work/next.txt",false,1))
            drafts.addRemote("other-thread",RemoteFile("other.txt","/work/other.txt",false,1))
            drafts.sent("origin-thread",setOf(oldPath))
            check(drafts.drafts.getValue("origin-thread").single().name == "next.txt") { "Acknowledgement cleared a newer draft" }
            check(drafts.drafts.getValue("other-thread").single().name == "other.txt") { "Acknowledgement cleared another thread" }
            val old = drafts.generation
            drafts.clear()
            drafts.fromPhone("stale-thread",listOf(Uri.fromFile(document)),old,targetContext.contentResolver)
            check(drafts.drafts.isEmpty() && !drafts.working) { "Stale picker accepted" }
        }
        runOnMainSync { drafts.browse(root, "origin-thread") }
        var browsing = true
        while (browsing && System.currentTimeMillis() < deadline + 10000) { runOnMainSync { browsing = drafts.working }; Thread.sleep(100) }
        runOnMainSync {
            check(drafts.directoryFor("origin-thread") == root) { "Last folder forgotten" }
            check(drafts.directoryFor("other-thread") == ".") { "Folder leaked across threads" }
            val restored = AttachmentState(scope, { transport }, { config })
            restored.restoreDirectories(drafts.directoriesJson())
            check(restored.directoryFor("origin-thread") == root) { "Folder snapshot not restored" }
        }
        transport.send(JSONObject().put("id",900003).put("method","turn/start").put("params",turnStartParams("fixture-thread","Inspect files","",attachments = listOf(note, Attachment("$root/image.png","image.png","image")))))
        check(response.await(15,TimeUnit.SECONDS) && failure.get() == null) { "Attachment turn failed: ${failure.get()?.message}" }
        result.putString("result","PASS: SFTP browsing, private atomic upload, stream failure cleanup, phone URI stream, origin-thread isolation and native image/file turn")
        finish(-1,result)
    } catch (e: Throwable) {
        result.putString("result","FAIL: attachment transport fixture: ${e.message?.take(160)}")
        finish(0,result)
    } finally {
        transport.close(); reader.join(5000); scope.cancel(); key.fill(0); keyFile.delete(); document.delete()
    }
}
