#!/usr/bin/env bash
#
# E2E-Testlauf auf dem CI-Emulator.
#
# Muss ein eigenes Skript sein: reactivecircus/android-emulator-runner führt sein
# `script:` zeilenweise über `sh -c` aus, mehrzeilige Konstrukte wie if/fi zerfallen
# dabei und scheitern mit „Syntax error: end of file unexpected".
#
# Erwartete Umgebungsvariablen:
#   API_LEVEL                            API-Level des laufenden Emulators
#   REAL_LOGIN_USER, REAL_LOGIN_PASSWORD Zugangsdaten für den echten Rössing-ID-Login
set -euo pipefail

# 0) Emulator-Standort auf Rössing setzen — davon lebt der Standort-Test.
#    (Ohne Fix liefert der Emulator gar keine Position, der Test überspringt dann.)
adb emu geo fix 9.8700 52.2110 || true

# 1) Instrumented- und E2E-Tests gegen das lokale Backend (Dev-Login).
./gradlew connectedDebugAndroidTest \
  -PapiBaseUrl=http://10.0.2.2:8099 \
  -PdevAuth=true \
  -Pandroid.testInstrumentationRunnerArguments.e2e=true

# 2) Echter Rössing-ID-Login gegen die Produktion — bewusst OHNE devAuth und ohne
#    apiBaseUrl-Override. Deckt den Weg ab, der zuvor kaputt war: Browser-Login →
#    Rücksprung über AppAuth → Token-Tausch → angemeldete Ansicht.
#    Nur auf API 35, weil erst das google_apis-Image Chrome (und damit Custom Tabs)
#    mitbringt; ohne Secrets wird der Lauf übersprungen.
if [ "${API_LEVEL:-}" = "35" ] && [ -n "${REAL_LOGIN_USER:-}" ] && [ -n "${REAL_LOGIN_PASSWORD:-}" ]; then
  # Chrome-Erststart-Dialoge unterdrücken, damit der Custom Tab direkt die
  # Zitadel-Anmeldung zeigt.
  adb shell 'echo "chrome --disable-fre --no-default-browser-check --no-first-run" > /data/local/tmp/chrome-command-line' || true

  ./gradlew connectedDebugAndroidTest \
    -Pandroid.testInstrumentationRunnerArguments.class=de.roessing.app.RealLoginE2eTest \
    -Pandroid.testInstrumentationRunnerArguments.realLoginUser="$REAL_LOGIN_USER" \
    -Pandroid.testInstrumentationRunnerArguments.realLoginPassword="$REAL_LOGIN_PASSWORD"
else
  echo "Echter Login-Test übersprungen (API ${API_LEVEL:-unbekannt} bzw. keine Secrets)."
fi
