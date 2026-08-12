#!/usr/bin/env python3
"""Tests der Release-Automatik.

Läuft ohne Fremdbibliotheken:  python3 -m unittest discover -s scripts

Die Repo-Tests arbeiten mit echten Git-Repositories in einem Temp-Verzeichnis —
kein nachgebautes Git, damit auch die Bereichsbildung (Tag..HEAD) mitgeprüft wird.
"""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import naechste_version as nv  # noqa: E402


class TestVersionen(unittest.TestCase):
    def test_naechste_patch_version(self):
        self.assertEqual(nv.naechste_patch_version(["v0.1.0", "v0.1.1", "v0.1.2"]), "0.1.3")

    def test_erster_tag_wenn_keine_tags(self):
        self.assertEqual(nv.naechste_patch_version([]), "0.1.0")

    def test_vorabversionen_werden_ignoriert(self):
        tags = ["v0.1.2", "v0.2.0-rc1", "v0.2.0-nightly", "v1.0.0-dev"]
        self.assertEqual(nv.naechste_patch_version(tags), "0.1.3")

    def test_fremde_tags_stoeren_nicht(self):
        tags = ["backend-2026-01-01", "v0.9.9", "irgendwas"]
        self.assertEqual(nv.naechste_patch_version(tags), "0.9.10")

    def test_sortierung_ist_numerisch_nicht_alphabetisch(self):
        self.assertEqual(nv.naechste_patch_version(["v0.1.9", "v0.1.10"]), "0.1.11")

    def test_vorherige_version(self):
        tags = ["v0.1.0", "v0.1.1", "v0.1.2"]
        self.assertEqual(nv.vorherige_version(tags, "0.1.3"), "0.1.2")
        self.assertEqual(nv.vorherige_version(tags, "0.1.1"), "0.1.0")
        self.assertEqual(nv.vorherige_version([], "0.1.0"), "")

    def test_version_code(self):
        self.assertEqual(nv.version_code("0.1.2"), 1000102)
        self.assertEqual(nv.version_code("0.1.3"), 1000103)
        self.assertEqual(nv.version_code("v1.0.0"), 1010000)

    def test_version_code_steigt_mit_der_version(self):
        folge = ["0.1.2", "0.1.3", "0.2.0", "0.9.99", "1.0.0", "1.0.1"]
        codes = [nv.version_code(v) for v in folge]
        self.assertEqual(codes, sorted(codes))
        self.assertEqual(len(set(codes)), len(codes))

    def test_version_code_ueber_den_alten_laufnummern(self):
        # Vor der Umstellung: 100 + GITHUB_RUN_NUMBER. Play nimmt jeden Code nur
        # einmal, der neue Bereich muss deshalb klar darüber liegen.
        self.assertGreater(nv.version_code("0.0.0"), 100_000)

    def test_version_code_lehnt_zu_grosse_stellen_ab(self):
        with self.assertRaises(ValueError):
            nv.version_code("0.1.100")


class TestAuswahlkriterien(unittest.TestCase):
    def test_app_und_backend_sind_relevant(self):
        for pfad in [
            "android/app/src/main/java/A.kt",
            "backend/internal/api/handler.go",
            "android/app/build.gradle.kts",
        ]:
            self.assertTrue(nv.ist_release_relevant(pfad), pfad)

    def test_doku_store_ci_und_deploy_sind_irrelevant(self):
        for pfad in [
            "README.md",
            "store/metadata/android/de-DE/title.txt",
            ".github/workflows/android.yml",
            "deploy/overlays/production/kustomization.yaml",
            "scripts/naechste_version.py",
            "backend/README.md",
            "android/README.md",
        ]:
            self.assertFalse(nv.ist_release_relevant(pfad), pfad)

    def test_sperrmarker_im_betreff(self):
        self.assertTrue(nv.betreff_stoppt_release("ci: deploy app-backend abc [skip ci]"))
        self.assertTrue(nv.betreff_stoppt_release("fix: irgendwas [skip release]"))
        self.assertFalse(nv.betreff_stoppt_release("fix(android): Karte zentrieren"))


class TestEntscheidung(unittest.TestCase):
    TAGS = ["v0.1.0", "v0.1.1", "v0.1.2"]

    def test_neue_app_commits_ergeben_neue_patch_version(self):
        commits = [nv.Commit("a", "fix(android): Karte", ("android/app/src/main/A.kt",))]
        e = nv.entscheide(commits, self.TAGS)
        self.assertTrue(e.release)
        self.assertEqual(e.version, "0.1.3")
        self.assertEqual(e.tag, "v0.1.3")
        self.assertEqual(e.vorherige_version, "0.1.2")
        self.assertEqual(e.version_code, 1000103)

    def test_keine_commits_kein_tag(self):
        e = nv.entscheide([], self.TAGS)
        self.assertFalse(e.release)
        self.assertIn("keine neuen Commits", e.grund)

    def test_nur_doku_kein_tag(self):
        commits = [
            nv.Commit("a", "docs: README", ("README.md",)),
            nv.Commit("b", "chore(store): Texte", ("store/metadata/android/de-DE/title.txt",)),
        ]
        e = nv.entscheide(commits, self.TAGS)
        self.assertFalse(e.release)
        self.assertIn("nicht", e.grund)

    def test_deploy_bump_kein_tag(self):
        commits = [
            nv.Commit(
                "a",
                "ci: deploy app-backend abc [skip ci]",
                ("deploy/overlays/production/kustomization.yaml",),
            )
        ]
        e = nv.entscheide(commits, self.TAGS)
        self.assertFalse(e.release)

    def test_sperrmarker_auf_dem_zu_taggenden_commit(self):
        commits = [
            nv.Commit("a", "fix(backend): Notfall [skip release]", ("backend/main.go",)),
            nv.Commit("b", "feat(android): Karte", ("android/app/src/main/A.kt",)),
        ]
        e = nv.entscheide(commits, self.TAGS)
        self.assertFalse(e.release)

    def test_doku_plus_code_ergibt_release(self):
        commits = [
            nv.Commit("a", "docs: README", ("README.md",)),
            nv.Commit("b", "feat(backend): X", ("backend/main.go",)),
        ]
        self.assertTrue(nv.entscheide(commits, self.TAGS).release)


class TestGitRepo(unittest.TestCase):
    """Gegen ein echtes Git-Repository, inklusive Bereichsbildung."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.repo = Path(self.tmp.name)
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.email", "test@example.invalid")
        self.git("config", "user.name", "Test")
        self.git("config", "commit.gpgsign", "false")
        self.addCleanup(self.tmp.cleanup)

    def git(self, *args):
        return subprocess.run(
            ["git", "-C", str(self.repo), *args],
            check=True,
            capture_output=True,
            text=True,
        ).stdout

    def commit(self, pfad: str, betreff: str, inhalt: str = "x"):
        ziel = self.repo / pfad
        ziel.parent.mkdir(parents=True, exist_ok=True)
        ziel.write_text(inhalt + betreff, encoding="utf-8")
        self.git("add", "-A")
        self.git("commit", "-q", "-m", betreff)

    def test_patch_bump_nach_app_aenderung(self):
        self.commit("android/app/A.kt", "feat(android): Start")
        self.git("tag", "v0.1.0")
        self.commit("android/app/A.kt", "fix(android): Karte zentriert")
        e = nv.entscheide_aus_repo(self.repo)
        self.assertTrue(e.release)
        self.assertEqual(e.tag, "v0.1.1")

    def test_ohne_tags_erste_version(self):
        self.commit("backend/main.go", "feat(backend): Start")
        e = nv.entscheide_aus_repo(self.repo)
        self.assertTrue(e.release)
        self.assertEqual(e.tag, "v0.1.0")

    def test_ohne_neue_commits_kein_release(self):
        self.commit("android/app/A.kt", "feat(android): Start")
        self.git("tag", "v0.1.0")
        e = nv.entscheide_aus_repo(self.repo)
        self.assertFalse(e.release)

    def test_nur_store_und_doku_kein_release(self):
        self.commit("backend/main.go", "feat(backend): Start")
        self.git("tag", "v0.1.0")
        self.commit("README.md", "docs: Hinweise")
        self.commit("store/metadata/android/de-DE/title.txt", "chore(store): Titel")
        e = nv.entscheide_aus_repo(self.repo)
        self.assertFalse(e.release)

    def test_bump_commit_loest_nichts_aus(self):
        self.commit("backend/main.go", "feat(backend): Start")
        self.git("tag", "v0.1.0")
        self.commit(
            "deploy/overlays/production/kustomization.yaml",
            "ci: deploy app-backend abcdef [skip ci]",
        )
        e = nv.entscheide_aus_repo(self.repo)
        self.assertFalse(e.release)

    def test_bis_zeigt_auf_einen_aelteren_stand(self):
        self.commit("android/app/A.kt", "feat(android): Start")
        self.git("tag", "v0.1.0")
        self.commit("android/app/A.kt", "fix(android): Eins")
        mitte = self.git("rev-parse", "HEAD").strip()
        self.commit("README.md", "docs: Zwei")
        e = nv.entscheide_aus_repo(self.repo, mitte)
        self.assertTrue(e.release)
        self.assertEqual(e.tag, "v0.1.1")
        self.assertEqual(e.notizen, ["fix(android): Eins"])

    def test_vorabtag_verschiebt_die_zaehlung_nicht(self):
        self.commit("android/app/A.kt", "feat(android): Start")
        self.git("tag", "v0.1.0")
        self.commit("android/app/A.kt", "fix(android): Eins")
        self.git("tag", "v0.2.0-rc1")
        e = nv.entscheide_aus_repo(self.repo)
        self.assertTrue(e.release)
        self.assertEqual(e.tag, "v0.1.1")


if __name__ == "__main__":
    unittest.main()
