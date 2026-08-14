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
adb emu geo fix 9.8162 52.1843 || true

# 1) Instrumented- und E2E-Tests gegen das lokale Backend (Dev-Login).
./gradlew connectedDebugAndroidTest \
  -PapiBaseUrl=http://10.0.2.2:8099 \
  -PdevAuth=true \
  -Pandroid.testInstrumentationRunnerArguments.e2e=true

# 1b) Derselbe Kennungs-Test noch einmal, diesmal mit NICHT erteilter
#     Benachrichtigungs-Berechtigung.
#
#     Gradle installiert die Test-APKs immer mit `pm install -g` — alle
#     Laufzeitrechte erteilt. Für den Nachweis, dass ohne Erlaubnis keine
#     Gerätekennung ans Backend geht, muss die Berechtigung aber gerade
#     fehlen. Deshalb hier von Hand ohne `-g` installieren und die
#     Instrumentierung direkt starten.
#
#     `pm revoke` aus dem Test heraus scheidet aus: Android schießt beim
#     Entzug einer Laufzeitberechtigung den Prozess ab — und das ist derselbe
#     Prozess, in dem der Test läuft. `appops set POST_NOTIFICATION ignore`
#     wirkt nicht: `areNotificationsEnabled()` richtet sich nicht danach.
#
#     Vorher deinstallieren, weil ein Update bereits erteilte Rechte behält.
echo "--- Kennungs-Test ohne erteilte Benachrichtigungs-Berechtigung"
adb uninstall de.roessing.app >/dev/null 2>&1 || true
adb uninstall de.roessing.app.test >/dev/null 2>&1 || true
adb install -r -t app/build/outputs/apk/debug/app-debug.apk
adb install -r -t app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk

# `am instrument` liefert auch bei roten Tests den Exit-Code 0 — das Ergebnis
# steht nur im Text. Deshalb beides prüfen: kein "FAILURES!!!", und ein "OK".
ERGEBNIS=$(adb shell am instrument -w \
  -e class de.roessing.app.GeraetekennungE2eTest \
  -e e2e true \
  -e erlaubnisfrei true \
  de.roessing.app.test/androidx.test.runner.AndroidJUnitRunner)
echo "$ERGEBNIS"
if echo "$ERGEBNIS" | grep -q "FAILURES!!!"; then
  echo "Kennungs-Test ohne Berechtigung fehlgeschlagen." >&2
  exit 1
fi
if ! echo "$ERGEBNIS" | grep -q "^OK "; then
  echo "Kennungs-Test ohne Berechtigung lief nicht durch." >&2
  exit 1
fi

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
