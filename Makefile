ANDROID_DIR ?= android
ADB ?= adb
GRADLE ?= ./gradlew
APK := $(ANDROID_DIR)/app/build/outputs/apk/debug/app-debug.apk
PACKAGE := dev.modex
# Gradle does not run with the system JDK 25; override MODEX_JAVA_HOME when needed.
MODEX_JAVA_HOME ?= $(shell if test -x /home/linuxbrew/.linuxbrew/opt/openjdk@21/bin/java; then printf '%s' /home/linuxbrew/.linuxbrew/opt/openjdk@21; else printf '%s' "$$JAVA_HOME"; fi)

.PHONY: apk install-apk install-android launch-android

apk:
	cd $(ANDROID_DIR) && JAVA_HOME=$(MODEX_JAVA_HOME) GRADLE_USER_HOME=$(if $(GRADLE_USER_HOME),$(GRADLE_USER_HOME),/tmp/modex-gradle) $(GRADLE) assembleDebug

install-apk:
	$(ADB) install -r $(APK)

install-android: apk install-apk

launch-android: install-android
	$(ADB) shell monkey -p $(PACKAGE) 1
