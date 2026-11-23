package dev.modex

import org.junit.Assert.*
import org.junit.Test

class ToolCallTextTest {
    @Test fun summaryShowsCommandAndKeepsUnexpectedWrappers() {
        assertEquals("Ran git status", commandSummary("/usr/bin/bash -lc 'git status'", false))
        assertEquals("Running echo \"hello\"…", commandSummary("/bin/bash -lc \"echo \\\"hello\\\"\nls\"", true))
        assertEquals("Ran printf '%s' hi", commandSummary("printf '%s' hi", false))
        val malformed = "/bin/bash -lc 'git status"
        assertEquals("Ran $malformed", commandSummary(malformed, false))
        val arguments = "/bin/bash -lc 'echo \$0' argument"
        assertEquals("Ran $arguments", commandSummary(arguments, false))
        assertEquals("Ran shell command", commandSummary("", false))
    }

    @Test fun commandUsesShellAndEmbeddedLanguageColorsWithoutRewritingText() {
        val plain = "git status"
        assertTrue(styledToolCommand(plain).spanStyles.any { plain.substring(it.start, it.end) == "git" })
        val command = "/usr/bin/bash -lc \"python3 - <<'PY'\nimport sys\nprint(42)\nPY\""
        val styled = styledToolCommand(command)
        assertEquals(command, styled.text)
        assertTrue(styled.spanStyles.any { command.substring(it.start, it.end) == "import" })
        assertTrue(styled.spanStyles.any { command.substring(it.start, it.end) == "42" })
        val escaped = "/bin/bash -lc 'python3 - <<'\\''PY'\\''\nimport sys\nPY'"
        assertTrue(styledToolCommand(escaped).spanStyles.any { escaped.substring(it.start, it.end) == "import" })
        val raw = "python3 - <<'PY'\nimport sys\nPY"
        val joinedQuotes = "/bin/bash -lc '" + raw.replace("'", "'\"'\"'") + "'"
        assertTrue(styledToolCommand(joinedQuotes).spanStyles.any { joinedQuotes.substring(it.start, it.end) == "import" })
        val js = "node <<'JS'\nconst value = 42;\nJS"
        assertTrue(styledToolCommand(js).spanStyles.any { js.substring(it.start, it.end) == "const" })
        val patch = "apply_patch <<'PATCH'\n*** Begin Patch\n*** Update File: a.kt\n-old\n+new\n*** End Patch\nPATCH"
        assertTrue(styledToolCommand(patch).spanStyles.any { patch.substring(it.start, it.end) == "+new" })
    }

    @Test fun outputHighlightsOnlyRecognizableCodeAndLeavesOriginalTextIntact() {
        val inputs = listOf(
            "{\"enabled\": true, \"value\": 42}",
            "--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+new",
            "result:\n```python\nprint(42)\n```\n"
        )
        for (input in inputs) {
            val styled = styledToolOutput(input)
            assertEquals(input, styled.text)
            assertTrue(input, styled.spanStyles.isNotEmpty())
        }
        for (input in listOf("INFO completed 42", "{\"enabled\": ", "[] followed by logs", "{enabled: true}", "[1,]", "[,]", "{\"x\":,}", "+ a log line")) {
            assertTrue(input, styledToolOutput(input).spanStyles.isEmpty())
        }
    }

    @Test fun longQuotedTokensStayReadable() {
        val output = "[\"" + "\\n".repeat(7500) + "\"]"
        val styled = styledToolOutput(output)
        assertEquals(output, styled.text)
        assertTrue(styled.spanStyles.isNotEmpty())
    }
}
