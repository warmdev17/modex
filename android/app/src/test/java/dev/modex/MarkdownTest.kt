package dev.modex

import org.commonmark.node.*
import org.commonmark.ext.gfm.tables.TableBlock
import org.junit.Assert.*
import org.junit.Test

class MarkdownTest {
    @Test fun fencedCodePreservesWhitespaceAndLanguage() {
        val code = markdownDocument("```kotlin\n  val x = 1\n\n  println(x)\n```").firstChild as FencedCodeBlock
        assertEquals("kotlin", code.info)
        assertEquals("  val x = 1\n\n  println(x)\n", code.literal)
    }
    @Test fun streamingFenceStillRendersCode() {
        assertEquals("echo hi\n", (markdownDocument("```sh\necho hi").firstChild as FencedCodeBlock).literal)
    }
    @Test fun parsesNestedListsAndTable() {
        val doc = markdownDocument("- one\n  - two\n\n| A | B |\n| --- | --- |\n| 1 | 2 |")
        assertTrue(doc.firstChild is BulletList)
        assertTrue(doc.lastChild is TableBlock)
        assertTrue(doc.firstChild.firstChild.firstChild.next is BulletList)
    }
    @Test fun unsafeLinkDoesNotBecomeClickable() {
        assertFalse(isSafeMarkdownLink("javascript:alert(1)"))
        assertFalse(isSafeMarkdownLink("file:///etc/passwd"))
        assertTrue(isSafeMarkdownLink("https://example.com"))
    }
    @Test fun htmlRemainsAnInertNode() {
        assertTrue(markdownDocument("<script>alert(1)</script>").firstChild is HtmlBlock)
    }
}
