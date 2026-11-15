package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import android.view.accessibility.AccessibilityNodeInfo
import androidx.activity.compose.setContent
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import java.io.File

/** Exercises project selection and folder-only browsing without real account data. */
internal fun Instrumentation.remoteProjectSmoke() {
    val result = Bundle()
    try {
        val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        var showFolders by mutableStateOf(false)
        var directory by mutableStateOf(RemoteDirectory("/home/fixture", "/home", listOf(RemoteFile("workspace", "/home/fixture/workspace", true, 0), RemoteFile("hidden.txt", "/home/fixture/hidden.txt", false, 1)), false))
        var createdFolder = ""
        var chosenFolder = ""
        var started = emptyList<String>()
        runOnMainSync { activity.setContent { ModexTheme { Surface {
            ChatWorkspace(host = "fixture", connected = true, connecting = false, selected = "existing", threads = listOf("existing"), titles = emptyMap(), messages = emptyList(), busy = false, historyLoading = false, hasMore = false, error = "", sessionSettings = "", sessionModel = "fixture", decisions = emptyList(), answeringDecisionIds = emptySet(), onSelect = {}, onRefresh = {}, onMore = {}, onCreate = { _, _, _ -> error("Project selection lost") }, onSend = { _, _, _, _ -> false }, onAnswer = { _, _ -> }, onConnection = {}, onDisconnect = {}, hostModel = "fixture", hostEffort = "low",
                threadWorkspaces = mapOf("existing" to "/work/other"), projectMembership = mapOf("existing" to "p2"),
                registeredProjects = listOf(RemoteProject("p2", "other", listOf("/work/other"))),
                onNewProject = { showFolders = true }, onCreateInProject = { model, mode, effort, root, project -> started = listOf(model, mode, effort, root, project) })
            if (showFolders) RemoteFilePicker("~", directory, false, "", true, 0, { path -> directory = RemoteDirectory(path, "/home/fixture", emptyList(), false) }, {}, { showFolders = false }, onChooseDirectory = { chosenFolder = it; showFolders = false }, onCreateDirectory = { createdFolder = it })
        } } } }
        fun node(label: String): AccessibilityNodeInfo? {
            fun walk(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.text?.toString() == label || n.contentDescription?.toString() == label) return n
                for (i in 0 until n.childCount) walk(n.getChild(i))?.let { return it }
                return null
            }
            // Reset the accessibility cache after a Compose dialog closes.
            uiAutomation.serviceInfo = uiAutomation.serviceInfo
            return walk(uiAutomation.rootInActiveWindow)
        }
        fun await(label: String): AccessibilityNodeInfo {
            val deadline = System.currentTimeMillis() + 6000
            while (System.currentTimeMillis() < deadline) { node(label)?.let { return it }; Thread.sleep(100) }
            val labels = mutableListOf<String>()
            fun dump(n: AccessibilityNodeInfo?) { if (n == null) return; labels += "${n.text}|${n.contentDescription}"; for (i in 0 until n.childCount) dump(n.getChild(i)) }
            dump(uiAutomation.rootInActiveWindow)
            File(targetContext.filesDir, "project-failure.txt").writeText(labels.joinToString("\n"))
            val bmp = uiAutomation.takeScreenshot()
            if (bmp != null) { File(targetContext.filesDir, "project-failure.png").outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; bmp.recycle() }
            error("Missing $label")
        }
        fun click(label: String) {
            var n: AccessibilityNodeInfo? = await(label)
            while (n != null && !n.isClickable) n = n.parent
            check(n?.performAction(AccessibilityNodeInfo.ACTION_CLICK) == true) { "Cannot click $label" }
            waitForIdleSync(); Thread.sleep(400)
        }
        click("Open sessions")
        val newChatBounds = android.graphics.Rect().also { await("New chat").getBoundsInScreen(it) }
        val projectBounds = android.graphics.Rect().also { await("New project").getBoundsInScreen(it) }
        check(kotlin.math.abs(newChatBounds.centerX() - projectBounds.centerX()) <= 2) { "Sidebar labels are not centered on the same axis" }
        uiAutomation.takeScreenshot()?.let { bmp -> File(targetContext.filesDir, "sidebar-actions.png").outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG,100,it) }; bmp.recycle() }
        click("New chat"); await("Outside projects"); click("Create chat")
        runOnMainSync { check(started[3].isBlank() && started[4].isBlank()) { "Standalone chat inherited the selected project" } }
        click("Open sessions"); click("New chat in other"); await("/work/other"); click("Create chat")
        runOnMainSync { check(started[3] == "/work/other" && started[4] == "p2") }
        click("Open sessions"); click("New project"); await("Use this folder")
        check(node("hidden.txt") == null) { "Folder mode shows attachments" }
        click("workspace"); await("/home/fixture/workspace")
        val bmp = uiAutomation.takeScreenshot() ?: error("No screenshot")
        File(targetContext.filesDir, "project-folder.png").outputStream().use { bmp.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it) }; bmp.recycle()
        click("New folder")
        val field = await("Folder name").let { label ->
            fun editable(n: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (n == null) return null
                if (n.isEditable) return n
                for (i in 0 until n.childCount) editable(n.getChild(i))?.let { return it }
                return null
            }
            editable(uiAutomation.rootInActiveWindow) ?: label
        }
        check(field.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, "New project") }))
        click("Create folder"); check(createdFolder == "New project")
        click("Use this folder"); check(chosenFolder == "/home/fixture/workspace")
        result.putString("result", "PASS: project-specific chat root and identity, folder-only fullscreen browser, folder creation and folder selection")
        finish(-1, result)
    } catch (failure: Throwable) { result.putString("result", "FAIL: ${failure.message}"); finish(0, result) }
}
