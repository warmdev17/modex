package dev.modex

import org.junit.Assert.*
import org.junit.Test

class RemoteProjectTest {
    @Test fun projectAndWorkspaceTravelTogetherWithoutChangingPermissions() {
        val params = newThreadParams("custom", "read-only", "/remote/other", "project-1")
        assertEquals("/remote/other", params.getString("workspace"))
        assertEquals("project-1", params.getString("project_id"))
        assertEquals("read-only", params.getString("permission_mode"))
        assertFalse(newThreadParams("", "host-default").has("project_id"))
    }
    @Test fun folderCreationOnlyAcceptsOneLiteralChild() {
        listOf("new project", "dự án", "literal*? folder").forEach { assertTrue(it, validDirectoryName(it)) }
        listOf("", " ", ".", "..", "../escape", "/absolute", "a/b", "a\\b", "bad\nname", "bad\u0000name", "x".repeat(256)).forEach { assertFalse(it, validDirectoryName(it)) }
    }
}
