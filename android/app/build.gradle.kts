plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}
android {
    signingConfigs.getByName("debug") {
        // Keep upgrades compatible when Gradle caches move or are recreated.
        storeFile = file(System.getProperty("user.home") + "/.android/debug.keystore")
    }
    namespace = "dev.modex"
    compileSdk = 36
    defaultConfig { applicationId = "dev.modex"; minSdk = 26; targetSdk = 36; versionCode = 1; versionName = "0.1.0" }
    packaging { resources.excludes += "META-INF/versions/**/OSGI-INF/MANIFEST.MF" }
    buildFeatures { compose = true }
    compileOptions { sourceCompatibility = JavaVersion.VERSION_17; targetCompatibility = JavaVersion.VERSION_17 }
    kotlinOptions { jvmTarget = "17" }
}
dependencies {
    implementation(platform("androidx.compose:compose-bom:2025.04.01"))
    implementation("androidx.activity:activity-compose:1.10.1")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.9.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.10.2")
    implementation("com.github.mwiede:jsch:0.2.24")
    // Android lacks the Java 15 Ed25519 implementation used by desktop JSch.
    implementation("org.bouncycastle:bcprov-jdk18on:1.80")
    implementation("org.commonmark:commonmark:0.24.0")
    implementation("org.commonmark:commonmark-ext-gfm-tables:0.24.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
