package dev.modex

import android.app.Instrumentation
import android.os.Bundle
import androidx.core.content.pm.PackageInfoCompat
import org.json.JSONObject

/** Synthetic state only; never reads the user's saved host or starts SSH. */
internal fun Instrumentation.upgradeSmoke(args: Bundle) {
    val output = Bundle()
    val store = SavedHost(targetContext, "upgrade-fixture")
    try {
        val info = targetContext.packageManager.getPackageInfo(targetContext.packageName, 0)
        check(PackageInfoCompat.getLongVersionCode(info) == args.getString("versionCode")!!.toLong()) { "Unexpected installed version" }
        when (args.getString("stage")) {
            "seed" -> store.save(JSONObject().put("host", "fixture.invalid").put("key", "UPGRADE_TEST_ONLY")
                .put("selected", "fixture-thread").put("cursor", 105)
                .put("model", JSONObject().put("name", "fixture-model").put("effort", "high")))
            "verify" -> {
                val restored = store.load() ?: error("Saved state was lost during APK upgrade")
                check(restored.getString("host") == "fixture.invalid")
                check(restored.getString("key") == "UPGRADE_TEST_ONLY")
                check(restored.getString("selected") == "fixture-thread" && restored.getInt("cursor") == 105)
                check(restored.getJSONObject("model").getString("effort") == "high")
                store.clear()
            }
            else -> error("Expected seed or verify stage")
        }
        output.putString("result", "PASS: upgrade ${args.getString("stage")} at version ${PackageInfoCompat.getLongVersionCode(info)}")
        finish(-1, output)
    } catch (failure: Throwable) {
        output.putString("result", "FAIL: ${failure.javaClass.simpleName}: ${failure.message}")
        finish(0, output)
    }
}
