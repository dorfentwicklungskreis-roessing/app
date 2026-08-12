#!/usr/bin/env python3
"""Tests der Änderungshinweis-Erzeugung.

    python3 -m unittest discover -s scripts
"""

from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import aenderungsnotiz as an  # noqa: E402


class TestPunkte(unittest.TestCase):
    def test_praefix_faellt_weg(self):
        self.assertEqual(
            an.punkte(["feat(android): Karte zeigt den Standort"]),
            ["Karte zeigt den Standort"],
        )

    def test_rauschen_faellt_raus(self):
        betreffe = [
            "chore: Abhängigkeiten aktualisiert",
            "ci: Workflow angepasst",
            "test(backend): E2E ergänzt",
            "docs: README",
            "style: Formatierung",
            "refactor(backend): Aufräumen",
            "build: Gradle",
            "Merge pull request #12 from x/y",
            "fix(backend): Meldung wird gezählt",
        ]
        self.assertEqual(an.punkte(betreffe), ["Meldung wird gezählt"])

    def test_ci_bereich_faellt_raus(self):
        betreffe = [
            "fix(ci): Backend im Android-E2E lokal starten",
            "fix(e2e): Test stabilisiert",
            "fix(android): Karte lädt wieder",
        ]
        self.assertEqual(an.punkte(betreffe), ["Karte lädt wieder"])

    def test_doppelte_texte_nur_einmal(self):
        betreffe = ["fix(android): Karte zentriert", "fix(backend): Karte zentriert"]
        self.assertEqual(an.punkte(betreffe), ["Karte zentriert"])

    def test_betreff_ohne_praefix_zaehlt_nicht_als_typ(self):
        self.assertEqual(an.punkte(["Irgendwas ohne Präfix"]), [])

    def test_ausrufezeichen_bei_breaking_change(self):
        self.assertEqual(an.punkte(["feat!: Neue Anmeldung"]), ["Neue Anmeldung"])


class TestNotiz(unittest.TestCase):
    def test_aufzaehlung(self):
        text = an.notiz_de(
            ["feat(android): Rangliste", "fix(backend): Zählfehler", "ci: egal"]
        )
        self.assertEqual(text, "- Rangliste\n- Zählfehler")

    def test_fallback_wenn_nichts_uebrig_bleibt(self):
        self.assertEqual(an.notiz_de(["ci: nur CI", "chore: nur Kram"]), an.FALLBACK_DE)

    def test_fallback_ohne_commits(self):
        self.assertEqual(an.notiz_de([]), an.FALLBACK_DE)

    def test_kuerzung_auf_500_zeichen(self):
        betreffe = [f"feat(android): Neuerung Nummer {i} mit langem Text" for i in range(40)]
        text = an.notiz_de(betreffe)
        self.assertLessEqual(len(text), an.GRENZE)
        self.assertIn(an.MEHR_DE, text)
        # Keine halben Zeilen: jede Zeile ist ein vollständiger Punkt.
        for zeile in text.splitlines():
            self.assertTrue(zeile.startswith("- "))

    def test_einzelner_ueberlanger_betreff_wird_hart_gekuerzt(self):
        text = an.notiz_de(["fix(android): " + "a" * 900])
        self.assertLessEqual(len(text), an.GRENZE)
        self.assertTrue(text.endswith("…"))

    def test_englische_kurzfassung(self):
        self.assertIn("New features", an.notiz_en(["feat(android): X"]))
        self.assertEqual(an.notiz_en(["fix(android): X"]), an.FALLBACK_EN)


class TestSchreiben(unittest.TestCase):
    def test_schreibt_je_sprache_eine_datei(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            for locale in ("de-DE", "en-US"):
                (repo / "store" / "metadata" / "android" / locale).mkdir(parents=True)
            pfade = an.schreiben(repo, 1000103, "- Etwas", "Something")
            self.assertEqual(len(pfade), 2)
            basis = repo / "store" / "metadata" / "android"
            self.assertEqual(
                (basis / "de-DE" / "changelogs" / "1000103.txt").read_text(encoding="utf-8"),
                "- Etwas\n",
            )
            self.assertEqual(
                (basis / "en-US" / "changelogs" / "1000103.txt").read_text(encoding="utf-8"),
                "Something\n",
            )


if __name__ == "__main__":
    unittest.main()
