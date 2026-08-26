#!/bin/sh
# Nimmt die App-Store-Bilder für ein Gerät auf.
#
#   store/screenshots/aufnehmen.sh <simulator-udid> <ablageordner>
#
# Beispiel:
#   store/screenshots/aufnehmen.sh 59AC6B50-… iphone-6_9
#
# Alles läuft lokal: Backend, Terminfeed und Kartenstil kommen von diesem
# Rechner, angemeldet wird über den Entwickler-Login. Keine Zeile hiervon
# fasst app.xn--rssing-wxa.de, id.xn--rssing-wxa.de oder tiles.openfreemap.org
# an — auf Store-Bildern haben weder die Produktion noch echte Dorfbewohner
# etwas verloren.
#
# WICHTIG: Das Skript muss **in einem Stück** laufen. Server und Simulator
# müssen gleichzeitig leben; ein Aufteilen auf mehrere Aufrufe funktioniert
# in gekapselten Umgebungen nicht, weil der Simulator die Server sonst nicht
# mehr findet.
set -eu

UDID=${1:?Simulator-UDID fehlt}
ORDNER=${2:?Ablageordner fehlt (z.B. iphone-6_9)}

WURZEL=$(cd "$(dirname "$0")/../.." && pwd)
ARBEIT=${ARBEIT:-/tmp/dorf-shots-arbeit}
DB=${DB_PATH:-/tmp/dorf-shots.sqlite}
export PATH="$HOME/.local/share/mise/installs/go/1.23.12/bin:$PATH"

mkdir -p "$ARBEIT/web"
cp "$WURZEL/store/screenshots/beispieldaten/events.json"   "$ARBEIT/web/events.json"
cp "$WURZEL/store/screenshots/beispieldaten/map-style.json" "$ARBEIT/web/map-style.json"

# Belegte Ports sind der tückischste Fehler dieses Skripts: Ein vergessener
# Server von einem früheren Lauf liefert klaglos **alte** Daten aus, und die
# Bilder sehen fast richtig aus. Deshalb hier abbrechen statt weitermachen.
for port in 8080 8097; do
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
        echo "Port $port ist belegt — erst den alten Server beenden:" >&2
        lsof -nP -iTCP:"$port" -sTCP:LISTEN >&2
        exit 1
    fi
done

aufraeumen() {
    [ -n "${BACKEND:-}" ] && kill "$BACKEND" 2>/dev/null || true
    [ -n "${WEBSERVER:-}" ] && kill "$WEBSERVER" 2>/dev/null || true
}
trap aufraeumen EXIT INT TERM

# 1. Terminfeed und Kartenstil ausliefern — genau wie .github/workflows/ios.yml.
# Bewusst ohne Unterschale: `$!` muss der Server selbst sein, sonst
# erwischt das Aufräumen unten nur die Hülle und der Port bleibt belegt.
python3 -m http.server 8097 --bind 127.0.0.1 --directory "$ARBEIT/web" \
    >"$ARBEIT/web.log" 2>&1 &
WEBSERVER=$!

# 2. Backend mit Beispieldaten und Entwickler-Anmeldung.
#    Erst übersetzen, dann starten — nicht `go run`: Dessen Kindprozess
#    überlebt sonst das Aufräumen und belegt beim nächsten Lauf den Port.
(cd "$WURZEL/backend" && go build -o "$ARBEIT/dorfserver" ./cmd/server)
DB_PATH="$DB" AUTH_MODE=insecure-dev SEED=1 LISTEN_ADDR=127.0.0.1:8080 \
    "$ARBEIT/dorfserver" >"$ARBEIT/backend.log" 2>&1 &
BACKEND=$!

# Warten, bis beide antworten. Das Backend braucht beim ersten Mal am
# längsten — `go run` übersetzt erst.
warte_auf() {
    i=0
    while [ "$i" -lt 90 ]; do
        if curl -fsS --max-time 2 "$1" >/dev/null 2>&1; then return 0; fi
        i=$((i + 1)); sleep 2
    done
    echo "$1 antwortet nicht — siehe $ARBEIT/*.log" >&2
    return 1
}
warte_auf http://127.0.0.1:8080/healthz
warte_auf http://127.0.0.1:8097/events.json
warte_auf http://127.0.0.1:8097/map-style.json
echo "Backend und Webserver stehen."

python3 "$WURZEL/store/screenshots/beispieldaten/fuellen.py" --basis http://127.0.0.1:8080

# 3. Simulator. Der Neustart ist Absicht: Wurde vorher eine andere App
#    benutzt, klebt sonst ein „◀ Safari" in der Statusleiste — auf einem
#    Store-Bild sieht das nach einem Versehen aus.
xcrun simctl shutdown "$UDID" 2>/dev/null || true
xcrun simctl boot "$UDID"
xcrun simctl bootstatus "$UDID" -b
# Systemsprache Deutsch. Auf dem iPad steht das Datum in der Statusleiste;
# ein „Wed Aug 26" über einer durchgehend deutschen App sieht nach Versehen
# aus. Die Einstellung greift erst nach einem Neustart des Simulators.
if [ "$(xcrun simctl spawn "$UDID" defaults read -g AppleLocale 2>/dev/null)" != "de_DE" ]; then
    xcrun simctl spawn "$UDID" defaults write -g AppleLanguages -array de-DE
    xcrun simctl spawn "$UDID" defaults write -g AppleLocale -string de_DE
    xcrun simctl shutdown "$UDID"
    xcrun simctl boot "$UDID"
    xcrun simctl bootstatus "$UDID" -b
fi
# Volle Batterie und eine glatte Uhrzeit; an den Netz-Symbolen wird nichts
# gedreht — sie zeigen den echten Zustand des Simulators.
xcrun simctl status_bar "$UDID" override --time "9:15" \
    --batteryState charged --batteryLevel 100

# 4. Die App: normal aus ios/ gebaut, alle vier Adressen lokal übersteuert,
#    Entwickler-Login an (nur im Debug-Build vorhanden).
cd "$WURZEL/ios"
xcodegen generate
xcodebuild build -project Dorf.xcodeproj -scheme Dorf \
    -destination "platform=iOS Simulator,id=$UDID" \
    -clonedSourcePackagesDirPath .spm -derivedDataPath DerivedData \
    -configuration Debug \
    CODE_SIGNING_ALLOWED=YES CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY=- \
    API_BASE_URL=http://127.0.0.1:8080 \
    WEBSITE_BASE_URL=http://127.0.0.1:8097 \
    OIDC_ISSUER=http://127.0.0.1:8123 \
    MAP_STYLE_URL=http://127.0.0.1:8097/map-style.json \
    DEV_AUTH=1 >"$ARBEIT/bau.log" 2>&1
xcrun simctl uninstall "$UDID" de.roessing.app 2>/dev/null || true
xcrun simctl install "$UDID" "$WURZEL/ios/DerivedData/Build/Products/Debug-iphonesimulator/Dorf.app"

# 5. Der Prüfstand tippt sich durch die App und knipst.
cd "$WURZEL/store/screenshots/pruefstand"
xcodegen generate
rm -rf "$ARBEIT/Ergebnis.xcresult"
xcodebuild test -project DorfAufnahmen.xcodeproj -scheme DorfAufnahmen \
    -destination "platform=iOS Simulator,id=$UDID" \
    -derivedDataPath "$ARBEIT/DD-aufnahmen" \
    -resultBundlePath "$ARBEIT/Ergebnis.xcresult" \
    CODE_SIGNING_ALLOWED=YES CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY=- \
    >"$ARBEIT/test.log" 2>&1 || { tail -40 "$ARBEIT/test.log"; exit 1; }

# 6. Bilder aus dem Ergebnisbündel holen und ablegen.
python3 "$WURZEL/store/screenshots/bilder_ablegen.py" \
    --ergebnis "$ARBEIT/Ergebnis.xcresult" --geraet "$ORDNER"

xcrun simctl status_bar "$UDID" clear || true
echo "Fertig: store/screenshots/ios/*/$ORDNER/"
