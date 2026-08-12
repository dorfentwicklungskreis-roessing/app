#!/usr/bin/env python3
"""Erzeugt die Play-Änderungshinweise (`changelogs/<versionCode>.txt`) aus den
Commit-Betreffen seit dem letzten Release.

Aufruf:

    # nur anzeigen
    python3 scripts/aenderungsnotiz.py --version 0.1.3
    # in store/metadata/android/<locale>/changelogs/<versionCode>.txt schreiben
    python3 scripts/aenderungsnotiz.py --version 0.1.3 --schreiben

Was drinsteht: die Betreffe der `feat:`- und `fix:`-Commits seit dem vorherigen
Tag, ohne Präfix, als Aufzählung, auf Googles 500-Zeichen-Grenze gekürzt.
`chore:`, `ci:`, `test:`, `docs:`, `refactor:`, `style:`, `build:` und
Merge-Commits fallen raus — für Nutzerinnen und Nutzer im Dorf sagen sie nichts.

Sprachen: `de-DE` bekommt die Aufzählung. Für `en-US` gibt es einen kurzen
generischen Text — die Commits sind auf Deutsch, und maschinell übersetzte
Store-Texte wären schlechter als eine ehrliche Kurzfassung. Play verlangt
lediglich, dass zu jeder gepflegten Sprache etwas dasteht.
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from naechste_version import (  # noqa: E402
    letzter_tag,
    tags_lesen,
    version_code,
    vorherige_version,
)

# Play kappt bei 500 Zeichen.
GRENZE = 500

# Conventional-Commit-Typen, die es in die Store-Notiz schaffen.
SICHTBARE_TYPEN = {"feat", "fix", "perf", "revert"}

# Bereiche, die zwar als fix/feat auftauchen, für Nutzende aber unsichtbar sind.
UNSICHTBARE_BEREICHE = {"ci", "e2e", "test", "tests", "deps", "build", "release"}

FALLBACK_DE = "Kleinere Verbesserungen und Fehlerbehebungen."
FALLBACK_EN = "Minor improvements and bug fixes."
MEHR_DE = "- und weitere Verbesserungen"


def _zerlege(betreff: str) -> tuple[str, str, str]:
    """('feat', 'android', 'Karte zeigt X') — Typ ist '' bei Betreffen ohne Präfix."""
    kopf, trenner, rest = betreff.partition(":")
    if not trenner:
        return "", "", betreff.strip()
    typ = kopf.split("(")[0].strip().lower().rstrip("!")
    bereich = ""
    if "(" in kopf and ")" in kopf:
        bereich = kopf[kopf.index("(") + 1 : kopf.rindex(")")].strip().lower()
    if not typ.isalpha():
        return "", "", betreff.strip()
    return typ, bereich, rest.strip()


def punkte(betreffe) -> list[str]:
    """Aufzählungstexte aus Commit-Betreffen — gefiltert, entdoppelt, in Reihenfolge."""
    ergebnis: list[str] = []
    for betreff in betreffe:
        betreff = betreff.strip()
        if not betreff or betreff.startswith("Merge "):
            continue
        typ, bereich, text = _zerlege(betreff)
        if typ not in SICHTBARE_TYPEN or bereich in UNSICHTBARE_BEREICHE:
            continue
        if not text:
            continue
        text = text[0].upper() + text[1:]
        if text not in ergebnis:
            ergebnis.append(text)
    return ergebnis


def notiz_de(betreffe, grenze: int = GRENZE) -> str:
    """Deutsche Änderungsnotiz, garantiert höchstens `grenze` Zeichen lang."""
    texte = punkte(betreffe)
    if not texte:
        return FALLBACK_DE

    zeilen: list[str] = []
    laenge = 0
    uebrig = 0
    for i, text in enumerate(texte):
        zeile = f"- {text}"
        # +1 für den Zeilenumbruch vor jeder Zeile außer der ersten.
        zusatz = len(zeile) + (1 if zeilen else 0)
        if laenge + zusatz > grenze:
            uebrig = len(texte) - i
            break
        zeilen.append(zeile)
        laenge += zusatz

    if not zeilen:
        # Ein einzelner sehr langer Betreff: hart kürzen statt aufgeben.
        return (f"- {texte[0]}")[: grenze - 1] + "…"

    if uebrig and laenge + len(MEHR_DE) + 1 <= grenze:
        zeilen.append(MEHR_DE)
    return "\n".join(zeilen)


def notiz_en(betreffe) -> str:
    """Kurzfassung auf Englisch — siehe Modulkopf, warum nicht übersetzt wird."""
    typen = {_zerlege(b)[0] for b in betreffe if punkte([b])}
    if "feat" in typen:
        return "New features, improvements and bug fixes."
    return FALLBACK_EN


def betreffe_aus_repo(repo: Path, von: str, bis: str) -> list[str]:
    bereich = f"{von}..{bis}" if von else bis
    roh = subprocess.run(
        ["git", "-C", str(repo), "log", "--first-parent", "--format=%s", bereich],
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    return [z for z in roh.splitlines() if z.strip()]


def schreiben(repo: Path, code: int, de: str, en: str) -> list[Path]:
    """Schreibt die Notizen in alle vorhandenen Sprachverzeichnisse."""
    basis = repo / "store" / "metadata" / "android"
    geschrieben: list[Path] = []
    for locale in sorted(p.name for p in basis.iterdir() if p.is_dir()):
        text = de if locale.startswith("de") else en
        ziel = basis / locale / "changelogs" / f"{code}.txt"
        ziel.parent.mkdir(parents=True, exist_ok=True)
        ziel.write_text(text.rstrip("\n") + "\n", encoding="utf-8")
        geschrieben.append(ziel)
    return geschrieben


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", default=".")
    ap.add_argument("--version", required=True, help="z.B. 0.1.3")
    ap.add_argument(
        "--von",
        default=None,
        help="Startpunkt (Vorgabe: der Tag der vorherigen Version)",
    )
    ap.add_argument(
        "--bis",
        default=None,
        help="Endpunkt (Vorgabe: der Tag der Version, sonst HEAD)",
    )
    ap.add_argument("--schreiben", action="store_true")
    args = ap.parse_args(argv)

    repo = Path(args.repo)
    tags = tags_lesen(repo)

    von = args.von
    if von is None:
        vorher = vorherige_version(tags, args.version)
        if vorher:
            # Schreibweise des vorhandenen Tags übernehmen ("v0.1.2" oder "0.1.2").
            von = next(
                (t for t in tags if t.lstrip("v") == vorher), letzter_tag(tags)
            )
        else:
            von = ""

    bis = args.bis
    if bis is None:
        bis = next(
            (t for t in tags if t.lstrip("v") == args.version.lstrip("v")), "HEAD"
        )

    betreffe = betreffe_aus_repo(repo, von, bis)
    code = version_code(args.version)
    de = notiz_de(betreffe)
    en = notiz_en(betreffe)

    print(f"# versionCode {code} ({von or 'Anfang'}..{bis}, {len(betreffe)} Commits)")
    print("--- de-DE ---")
    print(de)
    print("--- en-US ---")
    print(en)

    if args.schreiben:
        for pfad in schreiben(repo, code, de, en):
            print(f"geschrieben: {pfad}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
