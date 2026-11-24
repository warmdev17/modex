package dev.modex

import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.runBlocking

class AttachmentTest {
    @Test fun oldHostJoinedFileReferencesRestoreClickableFilesWithoutChangingPrompt() {
        val marker = "Attached files selected by the user (JSON paths):\n"
        val source = "Please inspect this" + marker + """[{"name":"notes.json","path":"/work/notes.json"},{"name":"manual.pdf","path":"/work/manual.pdf"}]"""
        val restored = readChat(org.json.JSONArray().put(org.json.JSONObject().put("role", "user").put("text", source))).single()
        assertEquals("Please inspect this", restored.text)
        assertEquals(listOf("notes.json", "manual.pdf"), restored.attachments.map { it.name })
        assertEquals(restored, readChat(org.json.JSONArray().put(restored.toJson())).single())
        val trailing = source + " preserve this sentence"
        assertEquals(trailing, readChat(org.json.JSONArray().put(org.json.JSONObject().put("role", "user").put("text", trailing))).single().text)
    }
    @Test fun previewTextIsBoundedAndBinaryIsNotRenderedAsText() {
        val f = java.io.File.createTempFile("modex-preview", ".txt")
        try {
            f.writeText("hello\nworld")
            assertEquals("hello\nworld", attachmentTextPreview(f))
            f.writeBytes(byteArrayOf(0, 1, 2))
            assertNull(attachmentTextPreview(f))
            f.writeBytes(ByteArray(300 * 1024) { 97 })
            val preview = attachmentTextPreview(f)!!
            assertTrue(preview.contains("Preview limited"))
            assertTrue(preview.length < 263000)
        } finally { f.delete() }
    }
    @Test fun readHistoryRemovesOnlyAttachmentDisplayLabelsAndKeepsPaths() {
        val m = org.json.JSONObject().put("role", "user").put("display_text", "please inspect").put("text", "please inspect\n[Image: photo.png]\n[File: notes.txt]")
            .put("attachments", org.json.JSONArray("""[{"path":"/work/photo.png","name":"photo.png","kind":"image"},{"path":"/work/notes.txt","kind":"file"}]"""))
        val read = readChat(org.json.JSONArray().put(m)).single()
        assertEquals("please inspect", read.text)
        assertEquals("notes.txt", read.attachments.last().name)
    }
    @Test fun historyKeepsPreviewMetadataThroughEncryptedSnapshotSerialization() {
        val files = listOf(Attachment("/home/me/p.png", "p.png", "image"), Attachment("/home/me/n.txt", "n.txt", "file"))
        val entry = ChatEntry(role = "You", text = "see these\n[Image: p.png]", attachments = files)
        val restored = readChat(org.json.JSONArray().put(entry.toJson())).single()
        assertEquals(files, restored.attachments)
        assertEquals("see these\n[Image: p.png]", restored.text)
    }
    @Test fun attachmentOnlyTurnAndTextTurnKeepNativeKinds() {
        val files = listOf(Attachment("/state/picture.png", "picture.png", "image"), Attachment("/work/notes.txt", "notes.txt", "file"))
        val p = turnStartParams("t", "", "", attachments = files)
        assertEquals("image", p.getJSONArray("attachments").getJSONObject(0).getString("kind"))
        assertEquals("/work/notes.txt", p.getJSONArray("attachments").getJSONObject(1).getString("path"))
        assertFalse(turnStartParams("t", "hello", "").has("attachments"))
    }
    @Test fun literalSftpPathEscapesGlobsWithoutChangingSlashesOrSpaces() {
        assertEquals("/work/a\\*b\\?c\\\\d.txt", literalSftpPath("/work/a*b?c\\d.txt"))
        assertEquals("/work/a file.txt", literalSftpPath("/work/a file.txt"))
    }
    @Test fun documentNamesCannotEscapeUploadDirectory() {
        assertEquals("photo.png", attachmentName("../../photo.png"))
        assertEquals("report.txt", attachmentName("C:\\temp\\report.txt"))
        assertFalse(attachmentName("a\nb.txt").contains('\n'))
        assertNotEquals("..", attachmentName(".."))
    }
    @Test fun streamingBoundEnforcedEvenIfDocumentProviderLies() {
        val out = ByteArrayOutputStream()
        assertEquals(4L, copyAttachment(ByteArrayInputStream(byteArrayOf(1,2,3,4)),out,4))
        assertArrayEquals(byteArrayOf(1,2,3,4),out.toByteArray())
        try { copyAttachment(ByteArrayInputStream(ByteArray(5)), ByteArrayOutputStream(),4); fail("oversized stream accepted") } catch (_: IllegalArgumentException) { }
    }
    @Test fun exportKeepsOriginalBinaryBytesAndPublishesOnlyAfterClose() = runBlocking {
        val source = java.io.File.createTempFile("modex-export", ".apk")
        val bytes = byteArrayOf(0, 1, 2, 3, 127, -1)
        val output = object : ByteArrayOutputStream() { var closed = false; override fun close() { closed = true; super.close() } }
        try {
            source.writeBytes(bytes)
            val destination = exportAttachment(source, { "download-1" }, { output }, { check(output.closed) }, {})
            assertEquals("download-1", destination)
            assertArrayEquals(bytes, output.toByteArray())
            assertEquals("application/vnd.android.package-archive", attachmentMime("app.APK"))
            assertEquals("application/octet-stream", attachmentMime("archive.unknown"))
        } finally { source.delete() }
    }
    @Test fun eachExportCreatesANewDestinationInsteadOfOverwriting() = runBlocking {
        val source = java.io.File.createTempFile("modex-export", ".txt").apply { writeText("same bytes") }
        var next = 0
        val created = mutableListOf<Int>()
        try {
            repeat(2) { exportAttachment(source, { (++next).also(created::add) }, { ByteArrayOutputStream() }, {}, {}) }
            assertEquals(listOf(1, 2), created)
        } finally { source.delete() }
    }
    @Test fun failedOrCancelledExportCleansOnlyItsOwnedDestination() = runBlocking {
        val source = java.io.File.createTempFile("modex-export", ".bin").apply { writeBytes(byteArrayOf(4,5,6)) }
        val cleaned = mutableListOf<String>()
        var published = false
        try {
            try {
                exportAttachment(source, { "failed" }, { throw IOException("provider failed") }, { published = true }, cleaned::add)
                fail("write failure accepted")
            } catch (_: IOException) { }
            try {
                exportAttachment(source, { "cancelled" }, { ByteArrayOutputStream() }, {}, cleaned::add) { throw CancellationException() }
                fail("cancellation accepted")
            } catch (_: CancellationException) { }
            assertEquals(listOf("failed", "cancelled"), cleaned)
            assertFalse(published)
        } finally { source.delete() }
    }
    @Test fun pendingLegacyExportStaysBoundToItsHostAndAttachment() {
        val original = Attachment("/work/original.bin", "original.bin", "file", 6)
        val pending = PendingAttachmentExport("host-a", original, "/cache/original.bin", "application/octet-stream")
        assertEquals("/cache/original.bin", pending.sourceFor("host-a", original)?.path)
        assertNull(pending.sourceFor("host-b", original))
        assertNull(pending.sourceFor("host-a", original.copy(path = "/work/other.bin")))
    }
}
