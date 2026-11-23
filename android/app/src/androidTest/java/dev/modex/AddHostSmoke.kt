package dev.modex

import android.app.Instrumentation
import android.app.Activity
import android.content.Intent
import android.content.IntentFilter
import android.net.Uri
import android.os.Bundle
import android.os.SystemClock
import android.graphics.Rect
import android.util.Base64
import android.view.KeyEvent
import android.view.InputDevice
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import java.io.File
import com.jcraft.jsch.JSch
import com.jcraft.jsch.KeyPair
import org.json.JSONObject

internal fun Instrumentation.addHostSmoke(args: Bundle) {
    val output = Bundle()
    try {
        val app = targetContext.applicationContext as ModexApplication
        val catalog = SavedHost(app, "connections")
        val legacy = HostProfile("12345678-1234-1234-1234-123456789012", "Legacy empty fixture")
        val saved = HostProfile("12345678-1234-1234-1234-123456789013", "Configured fixture")
        val existing = listOf(legacy, saved)
        val pin = "SHA256:" + "A".repeat(43)
        val fixture = JSONObject().put("host", "old.invalid").put("port", 22)
            .put("user", "fixture").put("pin", pin).put("binary", "").put("state", "")
        val fixtureKey = Base64.encodeToString("SYNTHETIC_KEY".toByteArray(), Base64.NO_WRAP)
        if (args.getString("stage") == "seed") {
            check(catalog.load()?.let { connectionProfiles(it, false) == listOf(legacy) || connectionProfiles(it, false) == existing } != false)
            SavedHost(app, saved.id).save(JSONObject().put("connection", fixture).put("key", fixtureKey)
                .put("cursor", 105).put("instance", "fixture-instance")
                .put("host_choice", JSONObject().put("model", "fixture-model").put("effort", "high")))
            catalog.save(profilesJson(existing).put("reconnect_on_open", false))
        } else {
            val stored = SavedHost(app, saved.id).load() ?: error("Existing host was lost across upgrade")
            check(stored.getInt("cursor") == 105 && stored.getString("key") == fixtureKey &&
                stored.getJSONObject("host_choice").getString("model") == "fixture-model")
            val manager = app.connections
            val stage = args.getString("stage")
            check(if (stage != null) manager.profiles.containsAll(existing) else manager.profiles == existing)
            val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))

            fun find(label: String): AccessibilityNodeInfo? {
                fun walk(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                    if (node == null) return null
                    if (node.isVisibleToUser && listOf(node.text?.toString(), node.contentDescription?.toString()).contains(label)) return node
                    for (i in 0 until node.childCount) walk(node.getChild(i))?.let { return it }
                    return null
                }
                return walk(uiAutomation.rootInActiveWindow)
            }
            fun node(label: String): AccessibilityNodeInfo {
                val deadline = SystemClock.uptimeMillis() + 5000
                do {
                    find(label)?.let { return it }
                    Thread.sleep(80)
                } while (SystemClock.uptimeMillis() < deadline)
                error("Missing UI: $label")
            }
            fun tap(label: String) {
                val bounds = Rect().also(node(label)::getBoundsInScreen)
                val start = SystemClock.uptimeMillis()
                for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                    val event = MotionEvent.obtain(start, SystemClock.uptimeMillis(), action, bounds.exactCenterX(), bounds.exactCenterY(), 0)
                    event.source = InputDevice.SOURCE_TOUCHSCREEN
                    try { check(uiAutomation.injectInputEvent(event, true)) } finally { event.recycle() }
                }
                waitForIdleSync()
                Thread.sleep(180)
            }
            fun set(label: String, value: String) {
                val field = node(label)
                val edit = if (field.isEditable) field else field.parent
                check(edit.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, Bundle().apply {
                    putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, value)
                })) { "Cannot edit $label" }
            }
            fun back() = sendKeyDownUpSync(KeyEvent.KEYCODE_BACK)

            fun importKey() {
                val key = KeyPair.genKeyPair(JSch(), KeyPair.RSA, 1024)
                val file = File.createTempFile("add-host-key", ".fixture", app.cacheDir)
                try {
                    file.outputStream().use(key::writePrivateKey)
                    val picked = Instrumentation.ActivityResult(Activity.RESULT_OK, Intent().setData(Uri.fromFile(file)))
                    val filter = IntentFilter(Intent.ACTION_OPEN_DOCUMENT).apply {
                        addCategory(Intent.CATEGORY_OPENABLE)
                        addDataType("*/*")
                    }
                    val monitor = addMonitor(filter, picked, true)
                    try {
                        tap("Import SSH key")
                        node("Replace SSH key")
                        check(monitor.hits == 1)
                    } finally {
                        removeMonitor(monitor)
                    }
                } finally {
                    key.dispose()
                    file.delete()
                }
            }
            fun fill(address: String) {
                tap("Add host")
                set("Hostname or IP", address)
                set("SSH username", "fixture")
                set("SSH fingerprint", pin)
                importKey()
            }
            fun connectUnavailableHost() {
                fill("127.0.0.1")
                set("Port", "1")
                tap("Connect")
                node("SSH connection failed. Check the current host IP, port, network, sshd and SSH algorithm compatibility.")
                tap("Close")
                node("Navigation toolbar")
            }

            node("Navigation toolbar")
            if (stage == "add-connect") {
                val before = manager.profiles.size
                connectUnavailableHost()
                val added = manager.profiles.last()
                check(manager.profiles.size == before + 1 && added.name == "127.0.0.1")
                check(SavedHost(app, added.id).load()?.getJSONObject("connection")?.getInt("port") == 1)
                runOnMainSync { manager.remove(added.id) }
                output.putString("result", "PASS: Add host Connect opens SSH error on Hosts and retains the saved profile")
                finish(-1, output)
                return
            }
            if (stage == "connect-issue") {
                val error = "SSH key could not be loaded. Import a supported unencrypted private key."
                tap("Connect")
                node(error)
                tap("Close")
                node("Navigation toolbar")
                node("Issue")
                tap("Manage Configured fixture")
                tap("Edit host")
                node("Host settings")
                tap("Connect")
                node(error)
                tap("Close")
                node("Navigation toolbar")
                runOnMainSync {
                    val controller = manager.controller(saved.id)
                    controller.connect(controller.savedConnection!!)
                }
                node("Issue")
                check(find("Close") == null) { "A background connection opened a dialog" }
                output.putString("result", "PASS: manual Connect from Hosts and Edit host opens the error on Hosts, background failure stays on the card")
                finish(-1, output)
                return
            }
            if (stage == "issue-layout") {
                val manage = "Manage ${saved.name}"
                val before = Rect().also(node(manage)::getBoundsInScreen)
                val offline = Rect().also(node("Offline")::getBoundsInScreen)
                runOnMainSync {
                    val controller = manager.controller(saved.id)
                    check(!controller.configure(controller.savedConnection!!.copy(port = 0)))
                }
                val issue = Rect().also(node("Issue")::getBoundsInScreen)
                check(issue.right == offline.right) { "Issue ends at ${issue.right}, Offline at ${offline.right}" }
                val after = Rect().also(node(manage)::getBoundsInScreen)
                check(before == after) { "Issue moved the host controls from $before to $after" }
                tap("Issue")
                node("Check the address, port, fingerprint and any custom paths.")
                tap("Close")
                output.putString("result", "PASS: Issue keeps host controls in place and opens details ($after)")
                finish(-1, output)
                return
            }
            tap("Add host")
            node("Hostname or IP")
            check(manager.profiles == existing)
            back()
            node("Navigation toolbar")
            tap("Add host")
            tap("Advanced settings")
            tap("Back")
            node("Navigation toolbar")
            tap("Add host")
            set("Hostname or IP", "draft.invalid")
            tap("All hosts")
            node("Discard draft?")
            tap("Keep editing")
            node("draft.invalid")
            back()
            node("Discard draft?")
            tap("Discard draft")
            node("Navigation toolbar")
            check(manager.profiles == existing)
            tap("Set up")
            node("Host settings")
            node(legacy.name)
            back()
            node("Navigation toolbar")
            tap("Manage Configured fixture")
            tap("Edit host")
            node("old.invalid")
            node("Replace SSH key")
            back()
            node("Navigation toolbar")

            tap("Add host")
            set("Display name (optional)", "   ")
            tap("All hosts")
            node("Navigation toolbar")
            tap("Add host")
            set("Port", "2222")
            tap("Back")
            node("Discard draft?")
            tap("Discard draft")
            node("Navigation toolbar")
            tap("Add host")
            tap("Advanced settings")
            set("Host binary", "/fixture/tool")
            tap("All hosts")
            node("Discard draft?")
            tap("Discard draft")
            node("Navigation toolbar")

            fun screenshot(name: String) {
                val bitmap = uiAutomation.takeScreenshot() ?: error("Could not capture $name")
                File(targetContext.filesDir, name).outputStream().use {
                    bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
                }
                bitmap.recycle()
            }

            tap("Add host")
            node("Hostname or IP")
            screenshot("add-host-form.png")
            importKey()
            runOnMainSync { activity.recreate() }
            waitForIdleSync()
            node("Replace SSH key")
            tap("Back")
            node("Discard draft?")
            screenshot("add-host-discard.png")
            tap("Discard draft")
            node("Navigation toolbar")
            check(manager.profiles == existing)

            fill("fixture.invalid")
            set("Port", "0")
            tap("Save host")
            node("Check the address, port, fingerprint and any custom paths.")
            check(manager.profiles == existing)
            set("0", "22")
            val storeDir = app.noBackupFilesDir
            fun encryptedStores() = storeDir.list().orEmpty().filter { it.endsWith(".enc") }.toSet()
            val before = encryptedStores()
            val blockedCatalogWrite = File(storeDir, "connections.enc")
            val originalCatalog = File(storeDir, "fixture-catalog-backup")
            check(blockedCatalogWrite.renameTo(originalCatalog))
            check(blockedCatalogWrite.mkdir())
            try {
                tap("Save host")
                node("Could not save this host securely. Try again.")
                check(manager.profiles == existing && encryptedStores() == before)
            } finally {
                check(blockedCatalogWrite.delete())
                check(originalCatalog.renameTo(blockedCatalogWrite))
            }
            node("fixture.invalid")
            node("Replace SSH key")
            tap("Save host")
            node("Navigation toolbar")
            check(manager.profiles.size == existing.size + 1)
            val added = manager.profiles.single { it.name == "fixture.invalid" }
            val details = SavedHost(app, added.id).load() ?: error("Saved host has no encrypted credentials")
            check(details.getJSONObject("connection").getString("host") == "fixture.invalid" && details.getString("key").isNotBlank())

            connectUnavailableHost()
            node("Issue")
            check(manager.profiles.size == existing.size + 2)
            val failed = manager.profiles.single { it.name == "127.0.0.1" }
            check(SavedHost(app, failed.id).load()?.getJSONObject("connection")?.getInt("port") == 1)
            check(catalog.load()?.let { connectionProfiles(it, false) } == manager.profiles)
        }
        output.putString("result", "PASS: ${args.getString("stage") ?: "direct draft, encrypted upgrade, guarded exits, rollback/retry, Save and failed SSH retention"}")
        finish(-1, output)
    } catch (failure: Throwable) {
        uiAutomation.takeScreenshot()?.let { bitmap ->
            File(targetContext.filesDir, "add-host-failure.png").outputStream().use {
                bitmap.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it)
            }
            bitmap.recycle()
        }
        output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message} at ${failure.stackTrace.filter { it.fileName == "AddHostSmoke.kt" }.take(4)}")
        finish(0, output)
    }
}
