package dev.modex

import android.app.Instrumentation
import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import java.io.File
import org.json.JSONObject

/** Local UI fixture only; no SSH connection or command is executed. */
fun Instrumentation.approvalPreview() {
        val output = Bundle()
        try {
            val activity = startActivitySync(Intent(targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
            val decision = JSONObject("""{"decision_id":"fixture","method":"item/commandExecution/requestApproval","thread_id":"approval-preview","context":{"command":"printf MODEX_APPROVAL_OK","cwd":"/home/warmdev/Workspace/modex","reason":"Run a harmless command outside the read-only sandbox.","availableDecisions":["accept","cancel"]}}""")
            for (dark in listOf(false, true)) {
                runOnMainSync {
                    activity.setContent {
                        MaterialTheme(colorScheme = if (dark) darkColorScheme() else lightColorScheme()) {
                            DecisionDialog(decision) { response -> check(response.getString("decision") in listOf("accept", "cancel")) }
                        }
                    }
                }
                waitForIdleSync()
                Thread.sleep(700)
                val screenshot = uiAutomation.takeScreenshot() ?: error("No screenshot")
                File(targetContext.filesDir, "approval-${if (dark) "dark" else "light"}.png").outputStream().use {
                    check(screenshot.compress(android.graphics.Bitmap.CompressFormat.PNG, 100, it))
                }
                screenshot.recycle()
            }
            output.putString("result", "PASS: approval light/dark screenshots captured")
            finish(-1, output)
        } catch (_: Throwable) { output.putString("result", "FAIL: approval UI smoke"); finish(0, output) }
    }
