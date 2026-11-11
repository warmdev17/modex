package dev.modex

import org.junit.Assert.*
import org.junit.Test

class JsonPreviewTest {
    @Test fun formatsWithoutChangingNumbersEscapesOrStringWhitespace() {
        val source = """{"big":123456789012345678901,"ratio":1.2300e-9,"text":"  hi\nthere","ok":true,"missing":null,"a":[]}"""
        val formatted = jsonPreview(source)
        assertTrue(formatted.text.contains("\n  \"big\": 123456789012345678901"))
        assertTrue(formatted.text.contains("1.2300e-9"))
        assertTrue(formatted.text.contains("  hi\\nthere"))
        assertTrue(formatted.spanStyles.map { it.item.color }.toSet().size >= 5)
    }
    @Test fun preservesInvalidTruncatedAndOverdeepJsonAsRawText() {
        for (raw in listOf("{bad}", "{\"a\":", "[1,]", "{\"x\":1} trailing", "[".repeat(70)+"0"+"]".repeat(70))) {
            assertEquals(raw, jsonPreview(raw).text)
        }
    }
}
