#!/usr/bin/env python3
"""Prüft die App-Store-Metadaten unter store/metadata/ios/ auf Apples Vorgaben.

Gegenstück zu `store/check_metadata.py` (Play Store). Läuft ohne
Abhängigkeiten — nur Standardbibliothek —, damit die CI nichts installieren
muss und das Skript auch auf einem frischen Rechner sofort läuft.

Geprüft wird:
  * Vollständigkeit der Felddateien je Sprache (Fastlane-Deliver-Format:
    eine Datei je Feld)
  * Zeichengrenzen des App Store (Name 30, Untertitel 30, Schlüsselwörter 100,
    Werbetext 170, Beschreibung 4000, Neuerungen 4000)
  * dass einzeilige Felder wirklich einzeilig sind
  * Form der Schlüsselwörter (kommagetrennt, keine leeren Einträge, kein
    Leerzeichen hinter dem Komma — Apple zählt es mit)
  * dass die URL-Felder auf https zeigen und die Datenschutz-URL die
    veröffentlichte Erklärung ist
  * das App-Icon: 1024x1024, Bittiefe 8 und **ohne Alphakanal** — ein
    Marketing-Icon mit Transparenz weist App Store Connect ab (ITMS-90717)
  * die Asset-Kataloge: gültiges JSON, und jede dort benannte Bilddatei
    existiert
  * die Store-Bilder unter store/screenshots/ios/: je Sprache und Gerät
    vollständig, in einem von Apple angenommenen Maß, hochkant, und keins
    größer als 8 MB

Aufruf: python3 store/check_ios_metadata.py
"""
from __future__ import annotations

import json
import struct
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
META = REPO / "store" / "metadata" / "ios"
XCASSETS = REPO / "ios" / "Dorf" / "Assets.xcassets"
APPICON = XCASSETS / "AppIcon.appiconset"
SCREENSHOTS = REPO / "store" / "screenshots" / "ios"

# Apples Grenzen (App Store Connect / Fastlane deliver, Stand 2026).
LIMITS = {
    "name.txt": 30,
    "subtitle.txt": 30,
    "keywords.txt": 100,
    "promotional_text.txt": 170,
    "description.txt": 4000,
    "release_notes.txt": 4000,
}

# Felder, die auf genau einer Zeile stehen müssen.
EINZEILIG = {"name.txt", "subtitle.txt", "keywords.txt", "promotional_text.txt",
             "support_url.txt", "marketing_url.txt", "privacy_url.txt"}

URLS = {
    "support_url.txt": "https://xn--rssing-wxa.de/impressum/",
    "marketing_url.txt": None,          # frei, muss nur https sein
    "privacy_url.txt": "https://xn--rssing-wxa.de/app/datenschutz/",
}

PFLICHTDATEIEN = sorted(set(LIMITS) | set(URLS))

LOCALES = ["de-DE", "en-US"]

# Icon -> (Breite, Höhe, Alphakanal erlaubt?)
# Ab iOS 17 genügt je Darstellung ein 1024er Bild; Xcode rechnet die kleineren
# Größen selbst. Nur das helle Symbol wandert als Marketing-Icon in den Store,
# nur dort ist Transparenz verboten.
ICONS = {
    "icon-1024.png": (1024, 1024, False),
    "icon-1024-dark.png": (1024, 1024, True),
    "icon-1024-tinted.png": (1024, 1024, True),
}

# Store-Bilder: Ordner je Gerät -> die Maße, die Apple für den zugehörigen
# Anzeigetyp annimmt (siehe ANZEIGETYPEN in store/asc.py). Nur Hochformat —
# Querformat wäre erlaubt, aber die App wird stehend benutzt, und ein
# gemischter Satz sieht im Store nach Zufall aus.
SCREENSHOT_MASSE = {
    "iphone-6_9": {(1320, 2868), (1290, 2796)},
    "ipad-13": {(2064, 2752), (2048, 2732)},
}

# Die sieben Bilder je Satz, in der Reihenfolge, in der sie im Store stehen.
SCREENSHOT_NAMEN = [
    "01-mithelfen-karte.png",
    "02-mithelfen-liste.png",
    "03-ortsdetail.png",
    "04-rangliste.png",
    "05-veranstaltungen.png",
    "06-profil.png",
    "07-startseite.png",
]

# Apple weist größere Dateien ab.
SCREENSHOT_MAX_BYTES = 8 * 1024 * 1024

errors: list[str] = []


def fail(msg: str) -> None:
    errors.append(msg)


def read_text(path: Path) -> str:
    """Liest eine Metadatendatei. Ein einzelner Zeilenumbruch am Ende zählt nicht mit."""
    return path.read_text(encoding="utf-8").rstrip("\n")


def png_info(path: Path) -> tuple[int, int, int, int, bool]:
    """(Breite, Höhe, Bittiefe, Farbtyp, Transparenz) — ohne Fremdbibliothek.

    Transparenz heißt: Farbtyp mit Alphakanal (4/6) **oder** ein tRNS-Chunk,
    der auch einem Palettenbild durchsichtige Stellen gibt.
    """
    raw = path.read_bytes()
    if raw[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("keine PNG-Datei")
    if raw[12:16] != b"IHDR":
        raise ValueError("IHDR fehlt")
    width, height, depth, color_type = struct.unpack(">IIBB", raw[16:26])
    return width, height, depth, color_type, color_type in (4, 6) or b"tRNS" in raw


def check_locale(locale: str) -> None:
    base = META / locale
    if not base.is_dir():
        fail(f"{locale}: Verzeichnis fehlt ({base.relative_to(REPO)})")
        return

    for name in PFLICHTDATEIEN:
        path = base / name
        if not path.is_file():
            fail(f"{locale}/{name}: Datei fehlt")
            continue
        text = read_text(path)
        if not text.strip():
            fail(f"{locale}/{name}: ist leer")
            continue
        if name in LIMITS and len(text) > LIMITS[name]:
            fail(f"{locale}/{name}: {len(text)} Zeichen, erlaubt sind {LIMITS[name]}")
        if name in EINZEILIG and "\n" in text:
            fail(f"{locale}/{name}: muss einzeilig sein")

    check_keywords(locale, base / "keywords.txt")
    check_urls(locale, base)


def check_keywords(locale: str, path: Path) -> None:
    if not path.is_file():
        return
    text = read_text(path)
    if ", " in text:
        fail(f"{locale}/keywords.txt: Leerzeichen hinter dem Komma — Apple zählt es "
             "als Zeichen mit, es gehört weg")
    teile = [t for t in text.split(",")]
    if any(not t.strip() for t in teile):
        fail(f"{locale}/keywords.txt: leeres Schlüsselwort (doppeltes Komma oder Komma am Rand)")
    klein = [t.strip().lower() for t in teile if t.strip()]
    doppelt = {w for w in klein if klein.count(w) > 1}
    if doppelt:
        fail(f"{locale}/keywords.txt: doppelte Schlüsselwörter: {', '.join(sorted(doppelt))}")


def check_urls(locale: str, base: Path) -> None:
    for name, erwartet in URLS.items():
        path = base / name
        if not path.is_file():
            continue
        wert = read_text(path).strip()
        if not wert.startswith("https://"):
            fail(f"{locale}/{name}: muss mit https:// beginnen (steht: {wert!r})")
        if erwartet and wert != erwartet:
            fail(f"{locale}/{name}: {wert!r}, erwartet {erwartet!r}")


def check_icons() -> None:
    if not APPICON.is_dir():
        fail(f"ios/Dorf/Assets.xcassets/AppIcon.appiconset fehlt — "
             "erzeugen mit: bash store/assets/render-ios.sh")
        return

    for name, (want_w, want_h, alpha_ok) in ICONS.items():
        path = APPICON / name
        if not path.is_file():
            fail(f"AppIcon.appiconset/{name}: Datei fehlt — "
                 "erzeugen mit: bash store/assets/render-ios.sh")
            continue
        try:
            w, h, depth, _, transparent = png_info(path)
        except ValueError as exc:
            fail(f"AppIcon.appiconset/{name}: {exc}")
            continue
        if (w, h) != (want_w, want_h):
            fail(f"AppIcon.appiconset/{name}: {w}x{h}, erwartet {want_w}x{want_h}")
        if depth != 8:
            fail(f"AppIcon.appiconset/{name}: Bittiefe {depth}, erwartet 8")
        if transparent and not alpha_ok:
            fail(f"AppIcon.appiconset/{name}: hat einen Alphakanal. App-Store-Icons "
                 "dürfen keinen haben (ITMS-90717) — "
                 "'sips -g hasAlpha' muss 'no' melden, siehe store/assets/render-ios.sh")


def check_kataloge() -> None:
    """Contents.json muss gültiges JSON sein und darf nur vorhandene Dateien nennen."""
    for pfad in (XCASSETS / "Contents.json",
                 APPICON / "Contents.json",
                 XCASSETS / "AccentColor.colorset" / "Contents.json"):
        if not pfad.is_file():
            fail(f"{pfad.relative_to(REPO)}: fehlt")
            continue
        try:
            inhalt = json.loads(pfad.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            fail(f"{pfad.relative_to(REPO)}: kein gültiges JSON — {exc}")
            continue
        for bild in inhalt.get("images", []):
            datei = bild.get("filename")
            if datei and not (pfad.parent / datei).is_file():
                fail(f"{pfad.relative_to(REPO)}: nennt {datei}, die Datei fehlt aber")


def check_screenshots() -> None:
    """Die Store-Bilder je Sprache und Gerät.

    Fehlende Bilder sind hier ein Fehler und keine Warnung: Ohne sie lässt
    sich die Version gar nicht einreichen, und das soll auffallen, bevor
    jemand in App Store Connect danach sucht.
    """
    if not SCREENSHOTS.is_dir():
        fail("store/screenshots/ios/ fehlt — Bilder aufnehmen mit "
             "store/screenshots/aufnehmen.sh (siehe store/screenshots/README.md)")
        return

    for locale in LOCALES:
        for geraet, erlaubt in SCREENSHOT_MASSE.items():
            ordner = SCREENSHOTS / locale / geraet
            if not ordner.is_dir():
                fail(f"screenshots/{locale}/{geraet}: Verzeichnis fehlt")
                continue
            vorhanden = sorted(d.name for d in ordner.glob("*.png"))
            if vorhanden != SCREENSHOT_NAMEN:
                fehlt = [n for n in SCREENSHOT_NAMEN if n not in vorhanden]
                zuviel = [n for n in vorhanden if n not in SCREENSHOT_NAMEN]
                if fehlt:
                    fail(f"screenshots/{locale}/{geraet}: es fehlen {', '.join(fehlt)}")
                if zuviel:
                    fail(f"screenshots/{locale}/{geraet}: unerwartet {', '.join(zuviel)}")
            for name in vorhanden:
                pfad = ordner / name
                if pfad.stat().st_size > SCREENSHOT_MAX_BYTES:
                    fail(f"screenshots/{locale}/{geraet}/{name}: "
                         f"{pfad.stat().st_size // 1024} KiB — Apple nimmt höchstens 8 MB")
                try:
                    w, h, _, _, _ = png_info(pfad)
                except ValueError as exc:
                    fail(f"screenshots/{locale}/{geraet}/{name}: {exc}")
                    continue
                if (w, h) not in erlaubt:
                    masse = " oder ".join(f"{a}x{b}" for a, b in sorted(erlaubt))
                    fail(f"screenshots/{locale}/{geraet}/{name}: {w}x{h}, "
                         f"erwartet {masse}")


def main() -> int:
    for locale in LOCALES:
        check_locale(locale)
    check_icons()
    check_kataloge()
    check_screenshots()

    if errors:
        print("iOS-Metadaten fehlerhaft:", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        print(f"\n{len(errors)} Beanstandung(en). Nichts hochladen, bis das leer ist.",
              file=sys.stderr)
        return 1

    print(f"iOS-Metadaten in Ordnung (Sprachen: {', '.join(LOCALES)}).")
    for locale in LOCALES:
        for name, limit in LIMITS.items():
            n = len(read_text(META / locale / name))
            print(f"  {locale}/{name}: {n}/{limit} Zeichen")
    for name in ICONS:
        w, h, _, _, transparent = png_info(APPICON / name)
        print(f"  AppIcon/{name}: {w}x{h}, Alphakanal: {'ja' if transparent else 'nein'}")
    for locale in LOCALES:
        for geraet in SCREENSHOT_MASSE:
            ordner = SCREENSHOTS / locale / geraet
            bilder = sorted(ordner.glob("*.png"))
            w, h, _, _, _ = png_info(bilder[0])
            print(f"  screenshots/{locale}/{geraet}: {len(bilder)} Bilder, {w}x{h}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
