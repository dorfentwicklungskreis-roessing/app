# Dev-Container-Image für die Dorf-App (Android + Go).
#
# Basis ist das offizielle Dev-Container-Java-Image (Microsoft, Debian bookworm).
# JDK 21 = die Version, mit der auch die CI baut (.github/workflows/android.yml);
# die App selbst kompiliert weiterhin gegen Java 17
# (compileOptions/jvmTarget in android/app/build.gradle.kts).
#
# Go und die Claude-CLI kommen als Dev-Container-Features dazu (devcontainer.json).
# Für das Android-SDK gibt es kein offizielles Feature, deshalb wird es hier
# explizit und fest gepinnt installiert.
FROM mcr.microsoft.com/devcontainers/java:1-21-bookworm

# Android-SDK-cmdline-tools 22.0 (Prüfsumme aus dem Google-Repository).
ARG ANDROID_CMDLINE_TOOLS_ZIP=commandlinetools-linux-15859902_latest.zip
ARG ANDROID_CMDLINE_TOOLS_SHA256=4e4c464f145a7512b57d088ac6c278c03c9eea610886b35a5e0804e74eedf583
# Passend zu android/: compileSdk = targetSdk = 35, AGP 8.7.3 → build-tools 35.0.0.
ARG ANDROID_PLATFORM=android-35
ARG ANDROID_BUILD_TOOLS=35.0.0

ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    GRADLE_USER_HOME=/home/vscode/.gradle
ENV PATH=${PATH}:/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/platform-tools:/opt/android-sdk/emulator

# Das Basis-Image bringt eine Yarn-APT-Quelle mit, deren Signaturschlüssel
# abgelaufen ist; sie lässt jedes „apt-get update" fehlschlagen. Yarn brauchen
# wir hier nicht (das Binary bleibt erhalten), also fliegt die Quelle raus.
RUN rm -f /etc/apt/sources.list.d/yarn.list

# unzip für die cmdline-tools, der Rest sind Laufzeit-Bibliotheken,
# die der Emulator auch headless (-no-window) braucht.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        unzip \
        libpulse0 libnss3 libxcursor1 libxdamage1 libxrandr2 libxi6 libxtst6 \
        libgl1 libasound2 libc++1 libpng16-16 libice6 libsm6 libxkbfile1 \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    mkdir -p "${ANDROID_HOME}/cmdline-tools"; \
    curl -fsSL -o /tmp/cmdline-tools.zip \
        "https://dl.google.com/android/repository/${ANDROID_CMDLINE_TOOLS_ZIP}"; \
    echo "${ANDROID_CMDLINE_TOOLS_SHA256}  /tmp/cmdline-tools.zip" | sha256sum -c -; \
    unzip -q /tmp/cmdline-tools.zip -d /tmp/cmdline-tools; \
    mv /tmp/cmdline-tools/cmdline-tools "${ANDROID_HOME}/cmdline-tools/latest"; \
    rm -rf /tmp/cmdline-tools.zip /tmp/cmdline-tools

# platform-tools (adb), Plattform + build-tools für den Gradle-Build,
# emulator für lokale UI-/E2E-Tests. Das System-Image wird bewusst NICHT
# mitgebaut (≈1,5 GB Download, ≈3,5 GB entpackt pro API-Level) — siehe devcontainer.json.
RUN set -eux; \
    yes | sdkmanager --licenses > /dev/null; \
    sdkmanager --install \
        "platform-tools" \
        "platforms;${ANDROID_PLATFORM}" \
        "build-tools;${ANDROID_BUILD_TOOLS}" \
        "emulator" > /dev/null; \
    rm -rf "${ANDROID_HOME}/.temp" /root/.android/cache; \
    chown -R vscode:vscode "${ANDROID_HOME}"; \
    install -d -o vscode -g vscode /home/vscode/.gradle /home/vscode/.android; \
    install -o vscode -g vscode /dev/null /home/vscode/.android/repositories.cfg
