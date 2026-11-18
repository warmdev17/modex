package dev.modex

import org.junit.Assert.*
import org.junit.Test

class SlashCommandsTest {
    @Test fun filtersWholeCommandDraftsWithoutInterceptingPathsOrProse() {
        assertEquals(listOf(AndroidSlashCommand.MODEL), slashSuggestions("/m"))
        assertEquals(listOf(AndroidSlashCommand.COPY), slashSuggestions("/CO"))
        assertTrue(slashSuggestions("/")!!.contains(AndroidSlashCommand.HELP))
        assertEquals(emptyList<AndroidSlashCommand>(), slashSuggestions("/vim"))
        for (draft in listOf("", "hello /model", "/tmp/file", "/model please", "/model\n", "@skill")) {
            assertNull(draft, slashSuggestions(draft))
        }
    }

    @Test fun onlyAnExactSupportedCommandCanDispatch() {
        assertEquals(AndroidSlashCommand.MODEL, exactSlashCommand("/MODEL"))
        for (draft in listOf("/", "/mod", "/keymap", "/model please", "/model\n")) assertNull(exactSlashCommand(draft))
    }
}
