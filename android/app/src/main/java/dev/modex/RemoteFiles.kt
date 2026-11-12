package dev.modex

import com.jcraft.jsch.ChannelSftp
import com.jcraft.jsch.SftpException
import java.io.InputStream
import java.util.UUID

internal fun SshTransport.listRemoteDirectory(path: String): RemoteDirectory = withFiles { sftp ->
    val current = sftp.realpath(if (path.isBlank() || path == "~") "." else if (path.startsWith("~/")) sftp.home.trimEnd('/') + path.drop(1) else path)
    val parent = sftp.realpath("$current/..").takeUnless { it == current }
    val entries = mutableListOf<RemoteFile>()
    var truncated = false
    sftp.ls(literalSftpPath(current), ChannelSftp.LsEntrySelector { entry ->
        val name = entry.filename
        if (name == "." || name == ".." || name.any { it.isISOControl() } || (!entry.attrs.isDir && !entry.attrs.isReg)) ChannelSftp.LsEntrySelector.CONTINUE
        else if (entries.size >= 500) { truncated = true; ChannelSftp.LsEntrySelector.BREAK }
        else {
            entries += RemoteFile(name, current.trimEnd('/') + "/" + name, entry.attrs.isDir, entry.attrs.size)
            ChannelSftp.LsEntrySelector.CONTINUE
        }
    })
    RemoteDirectory(current, parent, entries.sortedWith(compareBy<RemoteFile> { !it.directory }.thenBy { it.name.lowercase() }), truncated)
}

internal fun SshTransport.uploadAttachment(state: String, name: String, input: InputStream, progress: (Long) -> Unit): Attachment = withFiles { sftp ->
    val root = state.trimEnd('/') + "/attachments"
    try { sftp.lstat(literalSftpPath(root)) } catch (e: SftpException) {
        if (e.id != ChannelSftp.SSH_FX_NO_SUCH_FILE) throw e
        sftp.mkdir(root)
    }
    val attrs = sftp.lstat(literalSftpPath(root))
    require(attrs.isDir && !attrs.isLink)
    sftp.chmod(448, literalSftpPath(root)) // 0700; uploads are private to the SSH account.
    val directory = root + "/" + UUID.randomUUID().toString()
    sftp.mkdir(directory)
    sftp.chmod(448, literalSftpPath(directory))
    val partial = "$directory/.upload"
    val target = "$directory/${attachmentName(name)}"
    try {
        val size = sftp.put(literalSftpPath(partial)).use { copyAttachment(input, it, progress = progress) }
        sftp.chmod(384, literalSftpPath(partial)) // 0600 before publication.
        sftp.rename(literalSftpPath(partial), literalSftpPath(target))
        Attachment(sftp.realpath(target), attachmentName(name), attachmentKind(name), size)
    } catch (failure: Exception) {
        runCatching { sftp.rm(literalSftpPath(partial)) }
        runCatching { sftp.rmdir(literalSftpPath(directory)) }
        throw failure
    }
    // shortcut: retain completed uploads for stored conversations; add retention with explicit user controls.
}

internal fun SshTransport.downloadAttachment(file: Attachment, cache: java.io.File, host: String, checkCancelled: () -> Unit = {}): java.io.File = withFiles { sftp ->
    checkCancelled()
    val path = literalSftpPath(file.path)
    val attrs = sftp.lstat(path)
    require(attrs.isReg && !attrs.isLink && attrs.size <= MAX_ATTACHMENT_BYTES) { "File unavailable or over 20 MiB." }
    val key = java.security.MessageDigest.getInstance("SHA-256").digest((host + "\n" + file.path + "\n" + attrs.mTime + "\n" + attrs.size).toByteArray()).joinToString("") { "%02x".format(it) }
    val directory = java.io.File(cache, key).apply { mkdirs() }
    val target = java.io.File(directory, attachmentName(file.name))
    if (target.isFile && target.length() == attrs.size) return@withFiles target
    val partial = java.io.File(directory, ".download")
    try {
        sftp.get(path).use { input -> partial.outputStream().use { copyAttachment(input, it, progress = { checkCancelled() }) } }
        check(partial.length() == attrs.size && partial.renameTo(target))
        var total = cache.walkTopDown().filter { it.isFile }.sumOf { it.length() }
        cache.listFiles().orEmpty().filter { it != directory }.sortedBy { it.lastModified() }.forEach {
            if (total > 100L * 1024 * 1024) { val size = it.walkTopDown().filter { f -> f.isFile }.sumOf { f -> f.length() }; if (it.deleteRecursively()) total -= size }
        }
        target
    } catch (e: Exception) { partial.delete(); throw e }
}
