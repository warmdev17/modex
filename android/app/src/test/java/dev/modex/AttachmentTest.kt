package dev.modex

import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream

class AttachmentTest {
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
