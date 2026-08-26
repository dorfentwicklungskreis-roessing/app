#!/usr/bin/env python3
"""Holt die Bilder aus einem xcresult-Bündel und legt sie sprachweise ab.

Der Prüfstand hängt jedes Bild als `XCTAttachment` an das Testergebnis; die
Namen sind schon die Dateinamen (`01-mithelfen-karte` …). Hier werden sie
ausgepackt, auf ihre Maße geprüft und nach

    store/screenshots/ios/<sprache>/<geraet>/NN-name.png

kopiert.

**de-DE und en-US bekommen dieselben Bilder.** Die App ist durchgehend
deutsch — es gibt keine englische Oberfläche. Ein nachgestelltes englisches
Bild müsste erfunden werden, und genau das wäre falsch: Der Store zeigte dann
etwas, das die App nicht kann. Die englische Store-Beschreibung sagt es
ausdrücklich („The interface is in German only").

    python3 store/screenshots/bilder_ablegen.py \
        --ergebnis /tmp/dorf-shots-arbeit/Ergebnis.xcresult --geraet iphone-6_9
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import struct
import subprocess
import sys
import tempfile
from pathlib import Path

SPRACHEN = ["de-DE", "en-US"]
ZIEL = Path(__file__).resolve().parent / "ios"

# Was Apple je Anzeigetyp annimmt. Passt ein Bild nicht, bricht der Lauf ab —
# ein falsches Maß fällt sonst erst beim Hochladen auf.
MASSE = {
    "iphone-6_9": [(1320, 2868), (1290, 2796)],
    "ipad-13": [(2064, 2752), (2048, 2732)],
}

# Die Bildnamen aus dem Prüfstand. Fehlt einer, war der Lauf unvollständig.
ERWARTET = [
    "01-mithelfen-karte",
    "02-mithelfen-liste",
    "03-ortsdetail",
    "04-rangliste",
    "05-veranstaltungen",
    "06-profil",
    "07-startseite",
]


def png_masse(pfad: Path) -> tuple[int, int]:
    """Breite und Höhe aus dem IHDR-Kopf — ohne Fremdbibliothek."""
    with pfad.open("rb") as datei:
        kopf = datei.read(24)
    if kopf[:8] != b"\x89PNG\r\n\x1a\n":
        raise SystemExit(f"{pfad} ist kein PNG.")
    breite, hoehe = struct.unpack(">II", kopf[16:24])
    return breite, hoehe


def ausgepackt(ergebnis: Path, ordner: Path) -> dict[str, Path]:
    subprocess.run(
        ["xcrun", "xcresulttool", "export", "attachments",
         "--path", str(ergebnis), "--output-path", str(ordner)],
        check=True, capture_output=True,
    )
    manifest = json.loads((ordner / "manifest.json").read_text(encoding="utf-8"))
    gefunden: dict[str, Path] = {}
    for test in manifest:
        for anhang in test.get("attachments", []):
            name = anhang.get("suggestedHumanReadableName") or ""
            # XCTest hängt eine laufende Nummer und eine UUID an: aus
            # „01-mithelfen-karte_0_ABC….png" wird wieder „01-mithelfen-karte".
            treffer = re.match(r"^(\d\d-[a-z0-9-]+)_\d+_", name)
            if not treffer:
                continue
            gefunden[treffer.group(1)] = ordner / anhang["exportedFileName"]
    return gefunden


def hauptteil(ergebnis: Path, geraet: str) -> None:
    if geraet not in MASSE:
        raise SystemExit(f"Unbekanntes Gerät {geraet} — bekannt: {', '.join(MASSE)}")

    with tempfile.TemporaryDirectory() as tmp:
        bilder = ausgepackt(ergebnis, Path(tmp))
        fehlt = [name for name in ERWARTET if name not in bilder]
        if fehlt:
            raise SystemExit("Im Ergebnisbündel fehlen Bilder: " + ", ".join(fehlt))

        for sprache in SPRACHEN:
            ordner = ZIEL / sprache / geraet
            ordner.mkdir(parents=True, exist_ok=True)
            for alt in ordner.glob("*.png"):
                alt.unlink()
            for name in ERWARTET:
                quelle = bilder[name]
                masse = png_masse(quelle)
                if masse not in MASSE[geraet]:
                    raise SystemExit(
                        f"{name}: {masse[0]}×{masse[1]} passt nicht zu {geraet} "
                        f"(erlaubt: {MASSE[geraet]})"
                    )
                shutil.copyfile(quelle, ordner / f"{name}.png")
            print(f"{sprache}/{geraet}: {len(ERWARTET)} Bilder abgelegt "
                  f"({masse[0]}×{masse[1]}).")


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--ergebnis", required=True, type=Path)
    p.add_argument("--geraet", required=True, help="iphone-6_9 oder ipad-13")
    a = p.parse_args()
    if not a.ergebnis.exists():
        print(f"{a.ergebnis} gibt es nicht.", file=sys.stderr)
        raise SystemExit(1)
    hauptteil(a.ergebnis, a.geraet)
