package dev.modex

import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream

class AttachmentTest {
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
}
