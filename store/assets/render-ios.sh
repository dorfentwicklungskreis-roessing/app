#!/usr/bin/env bash
# Erzeugt das App-Icon der iOS-App aus den SVG-Quellen in diesem Verzeichnis.
#
#   store/assets/ios-icon.svg        -> AppIcon.appiconset/icon-1024.png         (hell)
#   store/assets/ios-icon-dark.svg   -> AppIcon.appiconset/icon-1024-dark.png    (dunkel)
#   store/assets/ios-icon-tinted.svg -> AppIcon.appiconset/icon-1024-tinted.png  (eingefärbt)
#
# Seit iOS 17 genügt **ein** Bild je Darstellung: 1024x1024. Die kleineren
# Größen rechnet Xcode beim Bauen selbst aus.
#
# Voraussetzung: ImageMagick 7 (`magick`, `brew install imagemagick`).
# Aufruf: bash store/assets/render-ios.sh   (von überall)
#
# Warum Alpha nur beim hellen Symbol entfernt wird: Das helle Bild ist
# zugleich das Marketing-Icon im App Store, und **App-Store-Icons dürfen
# keinen Alphakanal haben** — ein Upload mit Transparenz wird von App Store
# Connect abgewiesen (ITMS-90717). Die dunkle und die eingefärbte Variante
# sind reine Systemsymbole; dort ist der durchsichtige Grund gewollt, weil
# das System den Hintergrund selbst stellt.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
out="${OUT_DIR:-$here/../../ios/Dorf/Assets.xcassets/AppIcon.appiconset}"
mkdir -p "$out"

command -v magick >/dev/null || {
  echo "ImageMagick 7 fehlt (magick). macOS: brew install imagemagick" >&2
  exit 1
}

# --- helles Symbol: deckend, ohne Alphakanal --------------------------------
magick -background none "$here/ios-icon.svg" \
  -resize 1024x1024! -background "#3B6939" -alpha remove -alpha off \
  PNG24:"$out/icon-1024.png"

# --- dunkel und eingefärbt: mit durchsichtigem Grund ------------------------
magick -background none "$here/ios-icon-dark.svg" \
  -resize 1024x1024! PNG32:"$out/icon-1024-dark.png"
magick -background none "$here/ios-icon-tinted.svg" \
  -resize 1024x1024! PNG32:"$out/icon-1024-tinted.png"

# --- Kontrolle --------------------------------------------------------------
pruefe() { # datei erwartete_geometrie alpha_erlaubt(ja|nein)
  got="$(magick identify -format '%wx%h' "$1")"
  [ "$got" = "$2" ] || { echo "FEHLER: $1 ist $got, erwartet $2" >&2; exit 1; }
  alpha="$(magick identify -format '%A' "$1")"   # True/False/Blend/Undefined
  if [ "$3" = "nein" ] && [ "$alpha" != "False" ] && [ "$alpha" != "Undefined" ]; then
    echo "FEHLER: $1 hat einen Alphakanal — App-Store-Icons dürfen keinen haben" >&2
    exit 1
  fi
  echo "OK: $1 ($got, Alpha=$alpha, $(wc -c <"$1" | tr -d ' ') Bytes)"
}
pruefe "$out/icon-1024.png"        1024x1024 nein
pruefe "$out/icon-1024-dark.png"   1024x1024 ja
pruefe "$out/icon-1024-tinted.png" 1024x1024 ja

# sips ist der Gegencheck mit Apples eigenem Werkzeug — genau das, was auch
# `sips -g hasAlpha` von Hand meldet.
if command -v sips >/dev/null; then
  echo "sips: $(sips -g hasAlpha "$out/icon-1024.png" | tail -1 | tr -d ' ')"
fi
