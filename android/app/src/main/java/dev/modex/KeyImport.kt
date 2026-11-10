package dev.modex

import java.io.ByteArrayOutputStream
import java.io.InputStream

internal fun readImportedKey(input: InputStream): ByteArray {
    val output = ByteArrayOutputStream()
    val chunk = ByteArray(4096)
    while (true) {
        val count = input.read(chunk)
        if (count < 0) break
        require(output.size() + count <= 256 * 1024) { "The selected file exceeds 256 KiB." }
        output.write(chunk, 0, count)
    }
    require(output.size() > 0) { "The selected file is empty." }
    return output.toByteArray()
}
