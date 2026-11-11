package dev.modex

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.withStyle

/** Strict bounded formatting keeps numeric literals and string escapes exactly as stored. */
internal fun jsonPreview(source: String): AnnotatedString = try {
    require(source.length <= 256 * 1024)
    buildAnnotatedString {
        var position = 0
        fun space() { while (position < source.length && source[position] in " \t\r\n") position++ }
        fun token(value: String, color: Color = Mocha.Subtext) { require(length + value.length <= 1024 * 1024); withStyle(SpanStyle(color = color)) { append(value) } }
        fun newline(depth: Int) { token("\n" + "  ".repeat(depth)) }
        fun string(color: Color) {
            val start = position
            require(source.getOrNull(position++) == '"')
            while (true) {
                val char = source.getOrNull(position++) ?: error("Unterminated string")
                if (char == '"') break
                require(char >= ' ')
                if (char == '\\') {
                    val escape = source.getOrNull(position++) ?: error("Unterminated escape")
                    if (escape == 'u') repeat(4) { require(source.getOrNull(position++)?.digitToIntOrNull(16) != null) }
                    else require(escape in "\"\\/bfnrt")
                }
            }
            token(source.substring(start, position), color)
        }
        val number = Regex("-?(?:0|[1-9][0-9]*)(?:\\.[0-9]+)?(?:[eE][+-]?[0-9]+)?")
        fun value(depth: Int) {
            require(depth <= 64)
            space()
            when (source.getOrNull(position)) {
                '{', '[' -> {
                    val open = source[position++]
                    val close = if (open == '{') '}' else ']'
                    token(open.toString()); space()
                    if (source.getOrNull(position) != close) {
                        newline(depth + 1)
                        while (true) {
                            space()
                            if (open == '{') { string(Mocha.Blue); space(); require(source.getOrNull(position++) == ':'); token(": ") }
                            value(depth + 1); space()
                            if (source.getOrNull(position) != ',') break
                            position++; token(","); newline(depth + 1)
                        }
                        newline(depth)
                    }
                    require(source.getOrNull(position++) == close); token(close.toString())
                }
                '"' -> string(Color(0xFFA6E3A1))
                't', 'f', 'n' -> {
                    val literal = when (source[position]) { 't' -> "true"; 'f' -> "false"; else -> "null" }
                    require(source.startsWith(literal, position)); position += literal.length
                    token(literal, if (literal == "null") Mocha.Overlay else Color(0xFFCBA6F7))
                }
                else -> {
                    val match = number.find(source, position)
                    require(match != null && match.range.first == position)
                    position += match.value.length; token(match.value, Color(0xFFFAB387))
                }
            }
        }
        value(0); space(); require(position == source.length)
    }
} catch (_: IllegalArgumentException) { AnnotatedString(source) }
  catch (_: IllegalStateException) { AnnotatedString(source) }
