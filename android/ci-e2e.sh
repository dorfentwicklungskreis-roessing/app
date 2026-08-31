#!/usr/bin/env bash
#
# E2E-Testlauf auf dem CI-Emulator.
#
# Muss ein eigenes Skript sein: reactivecircus/android-emulator-runner führt sein
# `script:` zeilenweise über `sh -c` aus, mehrzeilige Konstrukte wie if/fi zerfallen
# dabei und scheitern mit „Syntax error: end of file unexpected".
#
# ALLES läuft gegen lokale Dienste auf dem Runner — Backend, Zitadel, Website-
# Feed und Kartenstil. Kein Schritt hier darf einen entfernten Server anfassen,
# schon gar nicht die Produktion: Ein Test, der sich an der echten Rössing-ID
# anmeldet, wird rot, sobald der Server hustet, und kann dort obendrein Daten
# verändern.
#
# Erwartete Umgebungsvariablen:
#   API_LEVEL             API-Level des laufenden Emulators
#   E2E_API_BASE_URL      Backend im Dev-Login-Modus (Schritte 1 und 1b)
#   E2E_WEBSITE_BASE_URL  lokale Ablage mit events.json
#   E2E_MAP_STYLE_URL     lokaler Kartenstil
#   E2E_MIETEN_BASE_URL   lokale Ablage anstelle der Mietplattform
#   E2E_OIDC_ISSUER       lokales Zitadel aus dem docker compose (nur API 35)
#   E2E_OIDC_CLIENT_ID    Client-ID der dort angelegten nativen App
#   E2E_OIDC_API_BASE_URL Backend im OIDC-Modus gegen dieses Zitadel
#   E2E_LOGIN_USER, E2E_LOGIN_PASSWORD  im lokalen Zitadel angelegtes Testkonto
set -euo pipefail

# Vorbelegungen für den Fall, dass jemand das Skript von Hand startet.
API_BASE_URL="${E2E_API_BASE_URL:-http://10.0.2.2:8099}"
WEBSITE_BASE_URL="${E2E_WEBSITE_BASE_URL:-http://10.0.2.2:8097}"
MAP_STYLE_URL="${E2E_MAP_STYLE_URL:-http://10.0.2.2:8097/map-style.json}"
# Die Mietplattform ist ein eigener, entfernter Dienst. Im Testlauf zeigt die
# App auf den Runner: sonst holte der Bereich „Maschinchenring" beim ersten
# Öffnen seine Geräteliste aus der Produktion. Die statische Ablage kennt die
# Routen unter /api/v1/ nicht — der Bereich zeigt dann seinen Hinweis „gerade
# nicht erreichbar", und genau das ist hier richtig: Was der Bereich mit
# echten Antworten macht, prüfen RentalUiTest und RentalApiTest ohne Netz.
MIETEN_BASE_URL="${E2E_MIETEN_BASE_URL:-http://10.0.2.2:8097}"

# --- Beweisaufnahme ---------------------------------------------------------
# „Instrumentation run failed due to Process crashed" sagt für sich genommen
# nichts: Gradle merkt nur, dass der App-Prozess weg ist. Was ihn umgebracht
# hat, steht ausschließlich im logcat des Emulators — und der war bisher nach
# dem Job weg. Deshalb läuft ab hier ein Mitschnitt, und daneben ein Takt, der
# Speicher und Threadzahl des App-Prozesses über die Testfolge protokolliert.
# Beides landet als Artefakt im Lauf und wird bei einem Absturz zusätzlich
# direkt ins Job-Protokoll geschrieben.
DIAG="${DIAG_DIR:-app/build/reports/e2e-diagnose}"
mkdir -p "$DIAG"

adb logcat -c || true
adb logcat -G 32M || true
adb logcat -v time > "$DIAG/logcat.txt" 2>&1 &
LOGCAT_PID=$!

# Alle 3 Sekunden: RSS und Threadzahl des App-Prozesses, dazu der freie
# Speicher des Emulators. Wächst der Wert über die Testfolge monoton, ist der
# Absturz ein Leck und kein einzelner schuldiger Test.
speichertakt() {
  echo "zeit_s rss_kb threads memavailable_kb" > "$DIAG/speicher.txt"
  START=$(date +%s)
  while true; do
    ZEILE=$(adb shell "pid=\$(pidof de.roessing.app); \
      test -n \"\$pid\" && awk '/VmRSS/{r=\$2} /^Threads/{t=\$2} END{print r, t}' /proc/\$pid/status; \
      awk '/MemAvailable/{print \$2}' /proc/meminfo" 2>/dev/null | tr -d '\r' | tr '\n' ' ')
    echo "$(( $(date +%s) - START )) $ZEILE" >> "$DIAG/speicher.txt"
    sleep 3
  done
}
speichertakt &
TAKT_PID=$!

# Beim Verlassen: Takt und Mitschnitt beenden. Ist der Lauf gescheitert, die
# aussagekräftigen Zeilen sichtbar ins Job-Protokoll heben, statt sie im
# Artefakt zu verstecken.
aufraeumen() {
  CODE=$?
  kill "$TAKT_PID" "$LOGCAT_PID" 2>/dev/null || true
  sleep 1
  if [ "$CODE" != "0" ]; then
    echo "=============================================================="
    echo "E2E gescheitert (Code $CODE) — Diagnose aus dem Emulator:"
    echo "--- Speicherverlauf des App-Prozesses (letzte 40 Messpunkte) ---"
    tail -40 "$DIAG/speicher.txt" || true
    echo "--- Absturzspuren im logcat ---"
    grep -nE "FATAL|DEBUG *:|signal [0-9]+|lowmemorykiller|am_kill|Out of memory|OutOfMemoryError|Abort message|tombstone|GL_OUT_OF_MEMORY|eglCreateContext|Failed to create|too many|EMFILE|pthread_create" \
      "$DIAG/logcat.txt" | tail -80 || true
    echo "--- letzte 120 Zeilen logcat ---"
    tail -120 "$DIAG/logcat.txt" || true
    echo "--- Tombstones ---"
    adb shell ls -l /data/tombstones 2>/dev/null || true
    echo "=============================================================="
  fi
}
trap aufraeumen EXIT

# 0) Emulator-Standort auf Rössing setzen — davon lebt der Standort-Test.
#    (Ohne Fix liefert der Emulator gar keine Position, der Test überspringt dann.)
adb emu geo fix 9.8162 52.1843 || true

# 1) Instrumented- und E2E-Tests gegen das lokale Backend (Dev-Login).
#    websiteBaseUrl und mapStyleUrl zeigen ebenfalls auf den Runner: sonst
#    holte die App den Terminfeed von rössing.de und den Kartenstil von
#    OpenFreeMap — beides entfernte Server mitten im Testlauf.
./gradlew connectedDebugAndroidTest \
  -PapiBaseUrl="$API_BASE_URL" \
  -PwebsiteBaseUrl="$WEBSITE_BASE_URL" \
  -PmapStyleUrl="$MAP_STYLE_URL" \
  -PmietenBaseUrl="$MIETEN_BASE_URL" \
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

# 2) Echter OIDC-Login gegen das LOKALE Zitadel aus dem docker compose —
#    bewusst OHNE devAuth. Geprüft wird unverändert der komplette Weg, der
#    zuvor kaputt war: Browser-Login mit PKCE → Rücksprung über AppAuths
#    RedirectUriReceiverActivity → Token-Tausch → angemeldete Ansicht →
#    echter API-Aufruf mit dem erhaltenen Token. Nichts davon ist gemockt,
#    nur der Aussteller steht jetzt auf demselben Rechner.
#    Nur auf API 35, weil erst das google_apis-Image Chrome (und damit Custom
#    Tabs) mitbringt.
if [ "${API_LEVEL:-}" = "35" ] && [ -n "${E2E_OIDC_ISSUER:-}" ] && [ -n "${E2E_OIDC_CLIENT_ID:-}" ]; then
  # Chrome-Erststart-Dialoge unterdrücken, damit der Custom Tab direkt die
  # Zitadel-Anmeldung zeigt.
  adb shell 'echo "chrome --disable-fre --no-default-browser-check --no-first-run" > /data/local/tmp/chrome-command-line' || true

  # Der Ausgang wird gemerkt statt sofort abgebrochen: Die Diagnose unten ist
  # gerade dann wertvoll, wenn der Login schiefging.
  LOGIN_ERGEBNIS=0
  ./gradlew connectedDebugAndroidTest \
    -PapiBaseUrl="${E2E_OIDC_API_BASE_URL:?E2E_OIDC_API_BASE_URL fehlt}" \
    -PoidcIssuer="$E2E_OIDC_ISSUER" \
    -PoidcClientId="$E2E_OIDC_CLIENT_ID" \
    -PwebsiteBaseUrl="$WEBSITE_BASE_URL" \
    -PmapStyleUrl="$MAP_STYLE_URL" \
    -PmietenBaseUrl="$MIETEN_BASE_URL" \
    -Pandroid.testInstrumentationRunnerArguments.class=de.roessing.app.RealLoginE2eTest \
    -Pandroid.testInstrumentationRunnerArguments.realLoginUser="${E2E_LOGIN_USER:?E2E_LOGIN_USER fehlt}" \
    -Pandroid.testInstrumentationRunnerArguments.realLoginPassword="${E2E_LOGIN_PASSWORD:?E2E_LOGIN_PASSWORD fehlt}" \
    || LOGIN_ERGEBNIS=$?

  # Was im ECHTEN Token steht, gehört in die Ausgabe: Fehlt der Rollen-Claim,
  # ist in der ausgelieferten App niemand Verwaltung — und das sieht man sonst
  # nirgends, weil jede Rechteprüfung das 403 ja erwartet.
  echo "--- Claims des echten Tokens (aus dem Login oben)"
  adb logcat -d -s TOKENPROBE:I | sed -n 's/.*TOKENPROBE *: //p' || true

  # Und wenn der Login selbst hakte: der letzte Bildschirm im Klartext — plus
  # das, was AppAuth dazu zu sagen hat. Die Anzeige in der App nennt nur ein
  # Kürzel („0.9"); die eigentliche Erklärung steht ausschließlich hier.
  if [ "$LOGIN_ERGEBNIS" != "0" ]; then
    echo "--- Letzter Bildschirm der Anmeldung"
    adb logcat -d -s LOGINPROBE:I | sed -n 's/.*LOGINPROBE *: //p' || true
    echo "--- Meldungen des AuthManager"
    adb logcat -d -s AuthManager:V || true
    exit "$LOGIN_ERGEBNIS"
  fi
elif [ "${API_LEVEL:-}" = "35" ]; then
  # Früher wurde hier still übersprungen, wenn die Secrets fehlten — ein roter
  # Login-Weg wäre damit unbemerkt durchgerutscht. Das lokale Zitadel gehört
  # zur CI-Umgebung, sein Fehlen ist ein Fehler und keine Ausnahme.
  echo "E2E_OIDC_ISSUER/E2E_OIDC_CLIENT_ID fehlen — das lokale Zitadel wurde nicht eingerichtet." >&2
  exit 1
else
  echo "Login-Test übersprungen: API ${API_LEVEL:-unbekannt} bringt kein Chrome mit (Custom Tabs nötig)."
fi
