#!/usr/bin/env bash
# Erzeugt die Play-Store-Grafiken aus den SVG-Quellen in diesem Verzeichnis.
#
#   store/assets/icon.svg           -> metadata/android/de-DE/images/icon.png (512x512)
#   store/assets/featureGraphic.svg -> metadata/android/de-DE/images/featureGraphic.png (1024x500)
#
# Voraussetzung: ImageMagick (convert) und die DejaVu-Schriften.
# Aufruf: bash store/assets/render.sh   (aus dem Repo-Wurzelverzeichnis oder beliebig)
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Zielverzeichnis überschreibbar (die CI rendert nur zur Probe, in ein Temp-Verzeichnis).
out="${OUT_DIR:-$here/../metadata/android/de-DE/images}"
mkdir -p "$out"

font_bold=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf
font_regular=/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf
for f in "$font_bold" "$font_regular"; do
  [ -f "$f" ] || { echo "Schrift fehlt: $f (Paket fonts-dejavu-core)" >&2; exit 1; }
done

# --- App-Icon ---------------------------------------------------------------
# Play verlangt 512x512 PNG (32-Bit), ohne Transparenz und ohne eigene Ecken.
convert -background none "$here/icon.svg" \
  -resize 512x512! -background "#3B6939" -alpha remove -alpha off \
  PNG32:"$out/icon.png"

# --- Feature-Grafik ---------------------------------------------------------
# Hintergrund + Blume aus dem SVG, Schriftzug per ImageMagick.
convert -background none "$here/featureGraphic.svg" \
  -resize 1024x500! -background "#3B6939" -alpha remove -alpha off \
  -font "$font_bold" -pointsize 58 -fill "#FFFFFF" \
  -annotate +396+244 'Dorf-App Rössing' \
  -font "$font_regular" -pointsize 30 -fill "#DCEFD3" \
  -annotate +398+302 'Blumenkästen und Beete im Blick' \
  PNG24:"$out/featureGraphic.png"

# --- Kontrolle --------------------------------------------------------------
check() { # datei erwartete_geometrie
  got="$(identify -format '%wx%h' "$1")"
  [ "$got" = "$2" ] || { echo "FEHLER: $1 ist $got, erwartet $2" >&2; exit 1; }
  echo "OK: $1 ($got, $(stat -c%s "$1") Bytes)"
}
check "$out/icon.png" 512x512
check "$out/featureGraphic.png" 1024x500
