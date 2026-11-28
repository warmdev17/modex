package dev.modex

import org.json.JSONObject

internal fun sessionTitle(value: Any?): String? =
    (value as? String)?.trim()?.takeIf { it.isNotEmpty() && it.toByteArray(Charsets.UTF_8).size <= 512 && it.none(Char::isISOControl) }

internal fun sessionLabel(thread: JSONObject, name: String = thread.optString("name").trim()): String =
    name.ifBlank {
        thread.optString("preview").trim().ifBlank { "Untitled session · ${thread.optString("id").take(8)}" }
    }
