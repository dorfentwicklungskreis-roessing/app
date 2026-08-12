#!/usr/bin/env python3
"""Prüft die Play-Store-Metadaten unter store/metadata/ auf Googles Vorgaben.

Läuft ohne Abhängigkeiten (nur Standardbibliothek) und wird von
.github/workflows/store.yml bei jeder Änderung an store/ ausgeführt.

Geprüft wird:
  * Zeichenlimits von Titel, Kurz- und Langbeschreibung sowie Änderungshinweisen
  * Vollständigkeit je Sprache
  * Bildformate (Maße, Farbtiefe, Transparenz) von icon.png und featureGraphic.png
  * dass zum versionCode aus android/app/build.gradle.kts ein Änderungshinweis
    in jeder Sprache existiert

Aufruf: python3 store/check_metadata.py
"""
from __future__ import annotations

import re
import struct
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
META = REPO / "store" / "metadata" / "android"
GRADLE = REPO / "android" / "app" / "build.gradle.kts"

# Googles Limits (Play Console, Stand 2026).
LIMITS = {
    "title.txt": 30,
    "short_description.txt": 80,
    "full_description.txt": 4000,
}
CHANGELOG_LIMIT = 500

# Sprache -> Pflichtdateien. Bilder pflegen wir nur in der Standardsprache;
# Play greift für weitere Sprachen automatisch darauf zurück.
LOCALES = ["de-DE", "en-US"]
DEFAULT_LOCALE = "de-DE"

# Bild -> (Breite, Höhe, Alphakanal erlaubt?)
IMAGES = {
    "icon.png": (512, 512, True),
    "featureGraphic.png": (1024, 500, False),
}

errors: list[str] = []


def fail(msg: str) -> None:
    errors.append(msg)


def read_text(path: Path) -> str:
    """Liest eine Metadatendatei. Ein einzelner Zeilenumbruch am Ende zählt nicht mit."""
    return path.read_text(encoding="utf-8").rstrip("\n")


def png_info(path: Path) -> tuple[int, int, int, int]:
    """(Breite, Höhe, Bittiefe, Farbtyp) aus dem IHDR-Chunk — ohne Fremdbibliothek."""
    raw = path.read_bytes()
    if raw[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("keine PNG-Datei")
    # 8 Byte Signatur, 4 Byte Länge, 4 Byte Typ ("IHDR"), dann die Daten.
    if raw[12:16] != b"IHDR":
        raise ValueError("IHDR fehlt")
    width, height, depth, color_type = struct.unpack(">IIBB", raw[16:26])
    return width, height, depth, color_type


def version_code() -> int | None:
    m = re.search(r"^\s*versionCode\s*=\s*(\d+)", GRADLE.read_text(encoding="utf-8"), re.M)
    return int(m.group(1)) if m else None


def check_locale(locale: str) -> None:
    base = META / locale
    if not base.is_dir():
        fail(f"{locale}: Verzeichnis fehlt ({base.relative_to(REPO)})")
        return

    for name, limit in LIMITS.items():
        path = base / name
        if not path.is_file():
            fail(f"{locale}/{name}: Datei fehlt")
            continue
        text = read_text(path)
        if not text.strip():
            fail(f"{locale}/{name}: ist leer")
        n = len(text)
        if n > limit:
            fail(f"{locale}/{name}: {n} Zeichen, erlaubt sind {limit}")
        if name != "full_description.txt" and "\n" in text:
            fail(f"{locale}/{name}: muss einzeilig sein")

    changelogs = base / "changelogs"
    if not changelogs.is_dir():
        fail(f"{locale}/changelogs: Verzeichnis fehlt")
        return
    found = False
    for path in sorted(changelogs.glob("*.txt")):
        found = True
        if not re.fullmatch(r"\d+", path.stem):
            fail(f"{locale}/changelogs/{path.name}: Dateiname muss der versionCode sein")
        n = len(read_text(path))
        if n > CHANGELOG_LIMIT:
            fail(f"{locale}/changelogs/{path.name}: {n} Zeichen, erlaubt sind {CHANGELOG_LIMIT}")
        if n == 0:
            fail(f"{locale}/changelogs/{path.name}: ist leer")
    if not found:
        fail(f"{locale}/changelogs: kein einziger Änderungshinweis")

    vc = version_code()
    if vc is None:
        fail("android/app/build.gradle.kts: versionCode nicht gefunden")
    elif not (changelogs / f"{vc}.txt").is_file():
        fail(
            f"{locale}/changelogs/{vc}.txt fehlt — zum versionCode {vc} aus "
            "build.gradle.kts gehört ein Änderungshinweis"
        )


def check_images() -> None:
    images = META / DEFAULT_LOCALE / "images"
    if not images.is_dir():
        fail(f"{DEFAULT_LOCALE}/images: Verzeichnis fehlt")
        return
    for name, (want_w, want_h, alpha_ok) in IMAGES.items():
        path = images / name
        if not path.is_file():
            fail(f"{DEFAULT_LOCALE}/images/{name}: Datei fehlt")
            continue
        try:
            w, h, depth, color_type = png_info(path)
        except ValueError as exc:
            fail(f"{DEFAULT_LOCALE}/images/{name}: {exc}")
            continue
        if (w, h) != (want_w, want_h):
            fail(f"{DEFAULT_LOCALE}/images/{name}: {w}x{h}, erwartet {want_w}x{want_h}")
        # Farbtyp 4 und 6 haben einen Alphakanal.
        if color_type in (4, 6) and not alpha_ok:
            fail(f"{DEFAULT_LOCALE}/images/{name}: hat einen Alphakanal, Play verlangt hier keinen")
        if depth != 8:
            fail(f"{DEFAULT_LOCALE}/images/{name}: Bittiefe {depth}, erwartet 8")
        size = path.stat().st_size
        if size > 1024 * 1024:
            fail(f"{DEFAULT_LOCALE}/images/{name}: {size} Bytes, Play erlaubt höchstens 1 MB")

    shots = images / "phoneScreenshots"
    pngs = sorted(p for p in shots.glob("*.png")) if shots.is_dir() else []
    if 0 < len(pngs) < 2:
        fail("phoneScreenshots: Play verlangt mindestens 2 Telefon-Screenshots")
    for path in pngs:
        try:
            w, h, _, _ = png_info(path)
        except ValueError as exc:
            fail(f"phoneScreenshots/{path.name}: {exc}")
            continue
        short, long_ = min(w, h), max(w, h)
        if short < 320 or long_ > 3840:
            fail(f"phoneScreenshots/{path.name}: {w}x{h} — kurze Kante >=320, lange <=3840")


def main() -> int:
    for locale in LOCALES:
        check_locale(locale)
    check_images()

    if errors:
        print("Metadaten fehlerhaft:", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    vc = version_code()
    print(f"Metadaten in Ordnung (versionCode {vc}, Sprachen: {', '.join(LOCALES)}).")
    for locale in LOCALES:
        for name in LIMITS:
            path = META / locale / name
            print(f"  {locale}/{name}: {len(read_text(path))}/{LIMITS[name]} Zeichen")
    return 0


if __name__ == "__main__":
    sys.exit(main())
