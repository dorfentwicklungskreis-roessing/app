#!/usr/bin/env python3
"""Tests für store/check_metadata.py — vor allem: fehlende Änderungshinweise
müssen weiterhin auffallen.

Das Skript prüft das echte Repository, deshalb wird es hier als Prozess
gestartet (kein Nachbau der Verzeichnisse):

    python3 -m unittest discover -s scripts
"""

from __future__ import annotations

import os
import subprocess
import sys
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
SKRIPT = REPO / "store" / "check_metadata.py"


def lauf(app_version: str | None) -> subprocess.CompletedProcess:
    umgebung = dict(os.environ)
    umgebung.pop("APP_VERSION", None)
    if app_version is not None:
        umgebung["APP_VERSION"] = app_version
    return subprocess.run(
        [sys.executable, str(SKRIPT)],
        cwd=REPO,
        capture_output=True,
        text=True,
        env=umgebung,
    )


class TestCheckMetadata(unittest.TestCase):
    def test_aktueller_stand_ist_in_ordnung(self):
        ergebnis = lauf(None)
        self.assertEqual(ergebnis.returncode, 0, ergebnis.stdout + ergebnis.stderr)

    def test_fehlender_aenderungshinweis_faellt_auf(self):
        # Zu dieser Version kann es keine Notiz geben.
        ergebnis = lauf("99.98.97")
        self.assertEqual(ergebnis.returncode, 1)
        self.assertIn("changelogs/1999897.txt fehlt", ergebnis.stderr)

    def test_vorhandener_aenderungshinweis_reicht(self):
        ergebnis = lauf("0.1.2")
        self.assertEqual(ergebnis.returncode, 0, ergebnis.stdout + ergebnis.stderr)
        self.assertIn("versionCode 1000102", ergebnis.stdout)


if __name__ == "__main__":
    unittest.main()
