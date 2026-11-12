ANDROID_DIR ?= android
ADB ?= adb
GRADLE ?= ./gradlew
APK := $(ANDROID_DIR)/app/build/outputs/apk/debug/app-debug.apk
PACKAGE := dev.modex
# Gradle does not run with the system JDK 25; override MODEX_JAVA_HOME when needed.
MODEX_JAVA_HOME ?= $(shell if test -x /home/linuxbrew/.linuxbrew/opt/openjdk@21/bin/java; then printf '%s' /home/linuxbrew/.linuxbrew/opt/openjdk@21; else printf '%s' "$$JAVA_HOME"; fi)
HOST_BINARY ?= /tmp/modex-host
WORKSPACE ?= $(CURDIR)
STATE_DIR ?= $(HOME)/.local/state/modex-phone
CODEX ?= codex
PREFIX ?= $(HOME)/.local

.PHONY: apk install-apk install-android launch-android

apk:
	cd $(ANDROID_DIR) && JAVA_HOME=$(MODEX_JAVA_HOME) GRADLE_USER_HOME=$(if $(GRADLE_USER_HOME),$(GRADLE_USER_HOME),/tmp/modex-gradle) $(GRADLE) assembleDebug

install-apk:
	$(ADB) install -r $(APK)

install-android: apk install-apk

launch-android: install-android
	$(ADB) shell monkey -p $(PACKAGE) 1

.PHONY: host-build host host-attach host-stop

host-build:
	go build -o "$(HOST_BINARY).new" ./cmd/modex-host
	mv -f "$(HOST_BINARY).new" "$(HOST_BINARY)"

host: host-build
	"$(HOST_BINARY)" --background --workspace "$(WORKSPACE)" --state-dir "$(STATE_DIR)" --codex "$(CODEX)"

host-attach:
	"$(HOST_BINARY)" --attach --state-dir "$(STATE_DIR)"

host-stop:
	"$(HOST_BINARY)" --stop --state-dir "$(STATE_DIR)"

.PHONY: cli install-cli

# Choose by title or create new; THREAD remains an optional shortcut.
cli: host-build
	"$(HOST_BINARY)" --cli --thread "$(THREAD)" --workspace "$(WORKSPACE)" --codex "$(CODEX)"

install-cli:
	@mkdir -p "$(PREFIX)/bin"
	go build -o "$(PREFIX)/bin/modex-host.new" ./cmd/modex-host
	go build -o "$(PREFIX)/bin/modex.new" ./cmd/modex
	mv -f "$(PREFIX)/bin/modex-host.new" "$(PREFIX)/bin/modex-host"
	mv -f "$(PREFIX)/bin/modex.new" "$(PREFIX)/bin/modex"
