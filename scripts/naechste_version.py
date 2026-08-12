#!/usr/bin/env python3
"""Entscheidet, ob ein Stand von `main` ein neues Release bekommt — und welches.

Diese Logik steckt bewusst NICHT im YAML des Workflows, sondern hier: so ist sie
mit `python3 -m unittest discover scripts` testbar (siehe test_naechste_version.py)
und lässt sich lokal trocken ausprobieren:

    python3 scripts/naechste_version.py entscheiden
    python3 scripts/naechste_version.py code --version 0.1.3

Regeln (Kurzfassung):
  * Grundlage ist der letzte stabile Tag `vX.Y.Z`; Vorabversionen (`v0.2.0-rc1`)
    zählen nicht mit.
  * Es wird die Patch-Stelle erhöht. Gibt es noch keinen Tag, ist die erste
    Version 0.1.0.
  * Kein Release, wenn seit dem letzten Tag nichts passiert ist, wenn der zu
    taggende Commit `[skip ci]`/`[skip release]` im Betreff trägt oder wenn seit
    dem letzten Tag ausschließlich Dinge geändert wurden, die die App und das
    Backend nicht anfassen (Dokumentation, Store-Texte, CI, Deploy-Overlay).

Der versionCode für Play wird ebenfalls hier berechnet, damit
android/app/build.gradle.kts, store/check_metadata.py und der Release-Workflow
denselben Wert benutzen.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

# Nur stabile Versionen taugen als Ausgangspunkt. "v0.2.0-rc1" wird ignoriert,
# damit eine Vorabversion nicht die Zählung der echten Releases verschiebt.
TAG_MUSTER = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)$")

# Betreffe mit diesen Markern lösen kein Release aus. `[skip ci]` benutzt der
# Bump-Job im Backend-Workflow — ohne diese Sperre würde jeder Deploy-Bump ein
# weiteres Release nach sich ziehen (und dessen Bump wieder eins).
STOPP_MARKER = ("[skip ci]", "[ci skip]", "[skip release]", "[release skip]")

# Änderungen an diesen Pfaden landen nicht im Auslieferungsartefakt und lösen
# deshalb kein Release aus. Alles andere (android/**, backend/**) schon.
IRRELEVANTE_MUSTER = (
    "*.md",
    "store/**",
    ".github/**",
    "deploy/**",
    "scripts/**",
    ".devcontainer/**",
    ".gitignore",
    "LICENSE",
)

# Abstand zu den versionCodes der ersten Handbuild-/Laufnummer-Ära (100–999).
VERSION_CODE_BASIS = 1_000_000


@dataclass(frozen=True)
class Commit:
    """Ein Commit, so weit er für die Entscheidung zählt."""

    sha: str
    betreff: str
    dateien: tuple[str, ...] = ()


@dataclass(frozen=True)
class Entscheidung:
    release: bool
    grund: str
    version: str = ""
    tag: str = ""
    vorherige_version: str = ""
    version_code: int = 0
    notizen: list[str] = field(default_factory=list)


# --------------------------------------------------------------------------
# Versionen
# --------------------------------------------------------------------------


def version_aus_tag(tag: str) -> tuple[int, int, int] | None:
    """(major, minor, patch) — oder None, wenn der Tag keine stabile Version ist."""
    treffer = TAG_MUSTER.match(tag.strip())
    if not treffer:
        return None
    return tuple(int(t) for t in treffer.groups())  # type: ignore[return-value]


def letzte_version(tags: list[str]) -> tuple[int, int, int] | None:
    """Höchste stabile Version aus einer Tagliste."""
    versionen = [v for v in (version_aus_tag(t) for t in tags) if v is not None]
    return max(versionen) if versionen else None


def als_text(version: tuple[int, int, int]) -> str:
    return "{}.{}.{}".format(*version)


def naechste_patch_version(tags: list[str]) -> str:
    """Nächste Version: Patch +1, oder 0.1.0, wenn es noch keinen Tag gibt."""
    letzte = letzte_version(tags)
    if letzte is None:
        return "0.1.0"
    major, minor, patch = letzte
    return als_text((major, minor, patch + 1))


def vorherige_version(tags: list[str], version: str) -> str:
    """Höchste stabile Version, die kleiner als `version` ist ("" wenn keine)."""
    ziel = version_aus_tag(version)
    if ziel is None:
        raise ValueError(f"keine gültige Version: {version}")
    kleinere = [
        v for v in (version_aus_tag(t) for t in tags) if v is not None and v < ziel
    ]
    return als_text(max(kleinere)) if kleinere else ""


def version_code(version: str) -> int:
    """versionCode für Play aus der Version.

    0.1.2 -> 1000102. Monoton steigend, solange minor und patch unter 100
    bleiben, und mit deutlichem Abstand über den alten Codes (100 + Laufnummer),
    die vor der Umstellung vergeben wurden.

    Achtung: dieselbe Formel steht in android/app/build.gradle.kts. Der
    Release-Workflow vergleicht beide Werte, damit sie nicht auseinanderlaufen.
    """
    zerlegt = version_aus_tag(version)
    if zerlegt is None:
        raise ValueError(f"keine gültige Version: {version}")
    major, minor, patch = zerlegt
    if minor > 99 or patch > 99:
        raise ValueError(f"minor/patch müssen unter 100 bleiben: {version}")
    return VERSION_CODE_BASIS + major * 10_000 + minor * 100 + patch


# --------------------------------------------------------------------------
# Auswahlkriterien
# --------------------------------------------------------------------------


def _passt(pfad: str, muster: str) -> bool:
    """fnmatch mit `**`-Semantik für Verzeichnispräfixe."""
    if muster.endswith("/**"):
        return pfad == muster[:-3] or pfad.startswith(muster[:-2])
    return fnmatch.fnmatch(pfad, muster)


def ist_release_relevant(pfad: str) -> bool:
    """True, wenn eine Änderung an diesem Pfad ein Release rechtfertigt."""
    return not any(_passt(pfad, muster) for muster in IRRELEVANTE_MUSTER)


def relevante_dateien(dateien) -> list[str]:
    """Alle Pfade, die ein Release rechtfertigen."""
    return [p for p in dateien if ist_release_relevant(p)]


def betreff_stoppt_release(betreff: str) -> bool:
    klein = betreff.lower()
    return any(marker in klein for marker in STOPP_MARKER)


def entscheide(commits: list[Commit], tags: list[str]) -> Entscheidung:
    """Kern der Automatik: aus Commits + vorhandenen Tags wird ein Ja/Nein."""
    if not commits:
        letzte = letzte_version(tags)
        stand = f" seit v{als_text(letzte)}" if letzte else ""
        return Entscheidung(False, f"keine neuen Commits{stand}")

    # commits[0] ist der Stand, der getaggt würde.
    if betreff_stoppt_release(commits[0].betreff):
        return Entscheidung(
            False, f"Commit trägt eine Release-Sperre im Betreff: {commits[0].betreff}"
        )

    dateien: list[str] = []
    for commit in commits:
        dateien.extend(commit.dateien)
    relevant = relevante_dateien(dateien)
    if not relevant:
        return Entscheidung(
            False,
            "seit dem letzten Tag wurden nur Dinge geändert, die nicht "
            "ausgeliefert werden (Dokumentation, Store, CI, Deploy)",
        )

    version = naechste_patch_version(tags)
    return Entscheidung(
        release=True,
        grund=(
            f"{len(commits)} Commit(s) mit {len(set(relevant))} ausgelieferten "
            "Dateien seit dem letzten Tag"
        ),
        version=version,
        tag=f"v{version}",
        vorherige_version=vorherige_version(tags, version),
        version_code=version_code(version),
        notizen=[c.betreff for c in commits],
    )


# --------------------------------------------------------------------------
# Git-Anbindung
# --------------------------------------------------------------------------


def _git(repo: Path, *args: str) -> str:
    return subprocess.run(
        ["git", "-C", str(repo), *args],
        check=True,
        capture_output=True,
        text=True,
    ).stdout


def tags_lesen(repo: Path) -> list[str]:
    return [z for z in _git(repo, "tag", "--list").splitlines() if z.strip()]


def commits_lesen(repo: Path, seit_tag: str, bis: str = "HEAD") -> list[Commit]:
    """Commits (neueste zuerst) samt geänderter Dateien im Bereich seit_tag..bis."""
    bereich = f"{seit_tag}..{bis}" if seit_tag else bis
    roh = _git(
        repo,
        "log",
        "--first-parent",
        "--name-only",
        "--format=%x1e%H%x1f%s",
        bereich,
    )
    commits: list[Commit] = []
    for block in roh.split("\x1e"):
        if not block.strip():
            continue
        kopf, _, rest = block.partition("\n")
        sha, _, betreff = kopf.partition("\x1f")
        dateien = tuple(z for z in rest.splitlines() if z.strip())
        commits.append(Commit(sha.strip(), betreff.strip(), dateien))
    return commits


def letzter_tag(tags: list[str]) -> str:
    """Der Tag (in seiner Original-Schreibweise) zur höchsten stabilen Version."""
    letzte = letzte_version(tags)
    if letzte is None:
        return ""
    for tag in tags:
        if version_aus_tag(tag) == letzte:
            return tag
    return ""


def entscheide_aus_repo(repo: Path, bis: str = "HEAD") -> Entscheidung:
    tags = tags_lesen(repo)
    return entscheide(commits_lesen(repo, letzter_tag(tags), bis), tags)


# --------------------------------------------------------------------------
# Kommandozeile
# --------------------------------------------------------------------------


def _ausgabe_schreiben(entscheidung: Entscheidung) -> None:
    """Ergebnis nach stdout und — falls vorhanden — in $GITHUB_OUTPUT."""
    daten = {
        "release": "true" if entscheidung.release else "false",
        "version": entscheidung.version,
        "tag": entscheidung.tag,
        "vorherige_version": entscheidung.vorherige_version,
        "version_code": str(entscheidung.version_code or ""),
        "grund": entscheidung.grund,
    }
    print(json.dumps(daten, ensure_ascii=False, indent=2))
    ziel = os.environ.get("GITHUB_OUTPUT")
    if ziel:
        with open(ziel, "a", encoding="utf-8") as f:
            for schluessel, wert in daten.items():
                f.write(f"{schluessel}={wert}\n")


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    unter = ap.add_subparsers(dest="befehl", required=True)

    e = unter.add_parser("entscheiden", help="Release ja/nein für einen Stand")
    e.add_argument("--repo", default=".", help="Pfad zum Repository")
    e.add_argument("--bis", default="HEAD", help="Commit, der getaggt würde")

    c = unter.add_parser("code", help="versionCode zu einer Version ausgeben")
    c.add_argument("--version", required=True)

    args = ap.parse_args(argv)

    if args.befehl == "code":
        print(version_code(args.version))
        return 0

    entscheidung = entscheide_aus_repo(Path(args.repo), args.bis)
    _ausgabe_schreiben(entscheidung)
    return 0


if __name__ == "__main__":
    sys.exit(main())
