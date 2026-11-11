package dev.modex

import com.jcraft.jsch.ChannelSftp
import com.jcraft.jsch.SftpException
import java.io.InputStream
import java.util.UUID

internal fun SshTransport.listRemoteDirectory(path: String): RemoteDirectory = withFiles { sftp ->
    val current = sftp.realpath(path.ifBlank { "." })
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
