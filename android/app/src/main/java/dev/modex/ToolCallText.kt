package dev.modex

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString

private const val HIGHLIGHT_LIMIT = 16_384
private val shellWrapper = Regex("""^(?:(?:/[\w.-]+)*/)?(?:bash|sh|zsh)[ \t]+-(?:lc|c)[ \t]+""")

internal fun commandSummary(command: String, running: Boolean): AnnotatedString = buildAnnotatedString {
    val display = if (command.length <= HIGHLIGHT_LIMIT) unwrapShell(command) ?: command else command
    val firstLine = display.lineSequence().firstOrNull().orEmpty().trim()
    val preview = firstLine.take(240).trimEnd { it.isHighSurrogate() }.ifBlank { "shell command" }
    val more = running || firstLine.length > preview.length && firstLine.isNotBlank() || '\n' in display || '\r' in display
    append(if (running) "Running " else "Ran ")
    val commandStart = length
    append(preview)
    if (firstLine.isNotBlank()) highlight(preview, commandStart, ToolLanguage.Shell)
    if (more) append("…")
}

private fun unwrapShell(command: String): String? {
    val input = command.trim()
    val prefix = shellWrapper.find(input) ?: return null
    var index = prefix.range.last + 1
    if (input.getOrNull(index) !in listOf('\'', '"')) return null
    val result = StringBuilder()
    while (index < input.length) {
        val quote = input[index++]
        if (quote == '\\' && input.getOrNull(index) == '\'') {
            result.append(input[index++])
            continue
        }
        if (quote != '\'' && quote != '"') return null
        while (index < input.length && input[index] != quote) {
            val ch = input[index++]
            if (quote == '"' && ch == '\\' && index < input.length && input[index] in "\"\\$\u0060\n") {
                val escaped = input[index++]
                if (escaped != '\n') result.append(escaped)
            } else result.append(ch)
        }
        if (index == input.length) return null
        index++
    }
    return result.toString()
}

private enum class ToolLanguage { Shell, Python, JavaScript, Json, Diff }

private fun languageOf(label: String): ToolLanguage? = when (label.lowercase()) {
    "sh", "bash", "shell", "zsh" -> ToolLanguage.Shell
    "py", "python", "python3" -> ToolLanguage.Python
    "js", "javascript", "node", "mjs" -> ToolLanguage.JavaScript
    "json" -> ToolLanguage.Json
    "diff", "patch" -> ToolLanguage.Diff
    else -> null
}

private val heredoc = Regex("""(?<!<)<<(?!<)(-?)[ \t]*(['"]?)([A-Za-z_]\w*)\2(?=[\s;|&"'<>]|$)""")
private val interpreter = Regex("""(?:^|[\s/"';|&])((?:python(?:3(?:\.\d+)?)?|node|apply_patch))(?=\s|$)""")
private val outputFence = Regex("""(?m)^[ \t]{0,3}(\x60{3,}|~{3,})[ \t]*([\w-]+)[^\r\n]*\r?\n""")
private val diffHeader = Regex("""\A(?:diff --git [^\n]+\n|--- [^\n]+\r?\n\+\+\+ [^\n]+\n|\*\*\* Begin Patch\r?\n)""")
private val diffLine = Regex("""(?m)^(?:[+-].*|@@.*|diff --git .*|\*\*\* .*)""")
private val jsonToken = Regex("""\"(?:[^\"\\\x00-\x1F]++|\\(?:[\"\\/bfnrt]|u[0-9a-fA-F]{4}))*+\"|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?|true|false|null|[{}\[\],:]""")

// Check a complete object or array before treating arbitrary terminal output as JSON.
private fun completeJson(text: String): Boolean {
    if (text.firstOrNull() != '{' && text.firstOrNull() != '[') return false
    val tokens = jsonToken.findAll(text).iterator()
    var offset = 0
    var next: MatchResult? = tokens.next()
    fun take(): String {
        val token = next ?: return ""
        if (token.range.first != offset) return ""
        offset = token.range.last + 1
        while (offset < text.length && text[offset] in " \t\r\n") offset++
        next = if (tokens.hasNext()) tokens.next() else null
        return token.value
    }
    fun value(depth: Int): Boolean {
        if (depth > 64 || next == null) return false
        return when (val token = take()) {
            "{", "[" -> {
                val close = if (token == "{") "}" else "]"
                if (next?.value != close) {
                    do {
                        if (token == "{" && (next?.value?.startsWith('"') != true || take().isEmpty() || next == null || take() != ":")) return false
                        if (!value(depth + 1)) return false
                    } while (next?.value == "," && take() == ",")
                }
                next != null && take() == close
            }
            "true", "false", "null" -> true
            else -> token.firstOrNull()?.let { it == '"' || it == '-' || it in '0'..'9' } == true
        }
    }
    return value(0) && offset == text.length
}

private val commonTokens = """(?:"(?:[^"\\\r\n]++|\\[^\r\n])*+"|'(?:[^'\\\r\n]++|\\[^\r\n])*+')|\b[0-9]+(?:\.[0-9]+)?\b"""
private val shellHead = Regex("""(?m)^[ \t]*([\w./][\w./-]*)(?=\s|$)""")
private val shellTokens = Regex(commonTokens + """|#[^\r\n]*|\$\{[^}\r\n]*\}|\$[\w@*?#!-]+|(?<!\S)--?[a-zA-Z][\w-]*|&&|\|\||<<-?|>>?|[|;<>]|\b(?:if|then|else|fi|for|in|do|done|case|esac|export|function)\b""")
private val pythonTokens = Regex(commonTokens + """|#[^\r\n]*|\b(?:and|as|assert|async|await|break|class|continue|def|del|elif|else|except|False|finally|for|from|global|if|import|in|is|lambda|None|nonlocal|not|or|pass|raise|return|True|try|while|with|yield)\b""")
private val jsTokens = Regex(commonTokens + """|\x60[^\x60\r\n]*\x60|//[^\r\n]*|/\*[\s\S]*?\*/|\b(?:async|await|break|case|catch|class|const|continue|default|delete|else|export|extends|false|finally|for|function|if|import|in|instanceof|let|new|null|of|return|switch|throw|true|try|typeof|undefined|var|while|yield)\b""")
private val green = Color(0xFFA6E3A1)
private val peach = Color(0xFFFAB387)
private val mauve = Color(0xFFCBA6F7)

private fun AnnotatedString.Builder.highlight(text: String, offset: Int, language: ToolLanguage) {
    if (language == ToolLanguage.Shell) {
        for (match in shellHead.findAll(text)) {
            val range = match.groups[1]!!.range
            addStyle(SpanStyle(color = Mocha.Blue), offset + range.first, offset + range.last + 1)
        }
    }
    val pattern = when (language) {
        ToolLanguage.Shell -> shellTokens
        ToolLanguage.Python -> pythonTokens
        ToolLanguage.JavaScript -> jsTokens
        ToolLanguage.Json -> jsonToken
        ToolLanguage.Diff -> diffLine
    }
    for (match in pattern.findAll(text)) {
        val token = match.value
        val color = when {
            language == ToolLanguage.Diff -> when {
                token.startsWith("+++") || token.startsWith("---") -> Mocha.Blue
                token.startsWith('+') -> green
                token.startsWith('-') -> Mocha.Red
                else -> mauve
            }
            token.startsWith('"') || token.startsWith('\'') || token.startsWith('\u0060') -> green
            token.startsWith('#') || token.startsWith("//") || token.startsWith("/*") -> Mocha.Subtext
            token.first().isDigit() || token.startsWith('-') -> peach
            language == ToolLanguage.Json && token.first() in "{}[],:" -> Mocha.Blue
            else -> mauve
        }
        addStyle(SpanStyle(color = color), offset + match.range.first, offset + match.range.last + 1)
    }
}

internal fun styledToolCommand(command: String): AnnotatedString = buildAnnotatedString {
    append(command)
    // shortcut: keep giant commands readable as plain text, revisit if a viewport lexer is added.
    if (command.length > HIGHLIGHT_LIMIT) return@buildAnnotatedString
    var offset = 0
    var embedded: ToolLanguage? = null
    var delimiter = ""
    var stripTabs = false
    var bodyStart = 0
    val lines = command.split('\n')
    val scriptLines = unwrapShell(command)?.split('\n')?.takeIf { it.size == lines.size } ?: lines
    for ((index, line) in lines.withIndex()) {
        val scriptLine = scriptLines[index].removeSuffix("\r")
        val language = embedded
        if (language == null) {
            highlight(line, offset, ToolLanguage.Shell)
            val opening = heredoc.find(scriptLine)
            if (opening != null) {
                val program = interpreter.findAll(scriptLine.substring(0, opening.range.first)).lastOrNull()?.groupValues?.get(1)
                embedded = when {
                    program?.startsWith("python") == true -> ToolLanguage.Python
                    program == "node" -> ToolLanguage.JavaScript
                    program == "apply_patch" -> ToolLanguage.Diff
                    else -> null
                }
                delimiter = opening.groupValues[3]
                stripTabs = opening.groupValues[1] == "-"
                bodyStart = offset + line.length + 1
            }
        } else {
            val marker = if (stripTabs) scriptLine.trimStart('\t') else scriptLine
            if (marker == delimiter) {
                highlight(command.substring(bodyStart, offset), bodyStart, language)
                embedded = null
            }
        }
        offset += line.length + 1
    }
}

internal fun styledToolOutput(output: String): AnnotatedString = buildAnnotatedString {
    append(output)
    if (output.length > HIGHLIGHT_LIMIT) return@buildAnnotatedString
    val trimmed = output.trim()
    when {
        diffHeader.containsMatchIn(trimmed) -> highlight(output, 0, ToolLanguage.Diff)
        completeJson(trimmed) -> highlight(output, 0, ToolLanguage.Json)
        else -> {
            var next = 0
            while (next < output.length) {
                val opening = outputFence.find(output, next) ?: break
                val marker = opening.groupValues[1]
                val start = opening.range.last + 1
                val close = Regex("""(?m)^[ \t]{0,3}""" + Regex.escape(marker) + marker.first() + """*[ \t]*\r?$""")
                    .find(output, start) ?: break
                languageOf(opening.groupValues[2])?.let { highlight(output.substring(start, close.range.first), start, it) }
                next = close.range.last + 1
            }
        }
    }
}
