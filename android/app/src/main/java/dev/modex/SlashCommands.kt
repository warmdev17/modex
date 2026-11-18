package dev.modex

import java.util.Locale

internal enum class AndroidSlashCommand(val token: String, val description: String) {
    HELP("help", "Show Android commands"),
    MODEL("model", "Choose the model and reasoning for your next turn"),
    NEW("new", "Create a conversation"),
    RESUME("resume", "Choose a conversation from the sidebar"),
    FORK("fork", "Fork the current conversation without starting a turn"),
    ARCHIVE("archive", "Archive the current conversation"),
    DELETE("delete", "Delete the current conversation after confirmation"),
    SKILLS("skills", "Choose skills and plugins to add to the prompt"),
    COPY("copy", "Copy the latest completed Codex reply"),
}

internal fun slashSuggestions(draft: String): List<AndroidSlashCommand>? {
    if (!draft.matches(Regex("/[a-zA-Z]*"))) return null
    val prefix = draft.drop(1).lowercase(Locale.ROOT)
    return AndroidSlashCommand.entries.filter { it.token.startsWith(prefix) }
}

internal fun exactSlashCommand(draft: String): AndroidSlashCommand? =
    slashSuggestions(draft)?.firstOrNull { draft.equals("/${it.token}", ignoreCase = true) }
