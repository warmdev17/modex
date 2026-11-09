package dev.modex

import org.json.JSONObject

internal fun sessionLabel(thread: JSONObject): String =
    thread.optString("name").trim().ifBlank {
        thread.optString("preview").trim().ifBlank { "Untitled session · ${thread.optString("id").take(8)}" }
    }
