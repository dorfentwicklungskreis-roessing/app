#!/usr/bin/env python3
"""Wacht darüber, dass kein Test gegen einen entfernten Server läuft.

Hintergrund: Der Android-E2E meldete sich für `RealLoginE2eTest` an der ECHTEN
Rössing-ID an. Als der Produktionsserver ausfiel, wurde die CI rot, obwohl am
Code nichts falsch war — und ein Test mit gültiger Anmeldung hätte dort auch
Daten verändern können. Solche Zugriffe schleichen sich leise wieder ein
(eine vergessene `-P`-Übersteuerung genügt), deshalb diese Prüfung.

Geprüft wird viererlei:

1. **Dienst-Adressen** (Produktions-Zitadel, Produktions-API, fremde Kachel-
   und Push-Dienste) dürfen in Testquellen und Test-CI gar nicht vorkommen.
2. Die **Produktions-Website** darf als Datenstring vorkommen (Termine tragen
   nun einmal solche Links), aber niemals als *konfigurierter Endpunkt* —
   also nicht als baseUrl, Issuer oder Ähnliches.
3. Jeder Testlauf muss **alle** Adressen der App lokal übersteuern — auf
   Android in `android/ci-e2e.sh` (`-PapiBaseUrl` …), auf iOS in
   `.github/workflows/ios.yml` (`xcodebuild … API_BASE_URL=…`). Fehlt eine,
   greift die Produktions-Vorbelegung aus `build.gradle.kts` bzw. aus
   `ios/project.yml` — genau so ist der Fehler ursprünglich entstanden.
4. Der E2E-Job bekommt **keine Zugangsdaten-Secrets** mehr: Die Testkonten
   entstehen im lokalen Zitadel.
5. Für iOS-Tests (`ios/DorfTests/`) zusätzlich **strukturell**, nicht nur nach
   Zeichenketten: Ein Test erreicht die Produktion auch ganz ohne Adresse im
   Quelltext — indem er die Vorbelegungen der App benutzt. Beanstandet wird
   deshalb jede Verwendung von `Konfiguration.` (die Werte stammen aus
   `ios/project.yml` und zeigen auf die Produktion), jedes `DorfApi(` ohne
   ausdrückliches `basis:` (sonst greift dieselbe Vorbelegung) sowie
   `URLSession.shared` und `URLSession.dorfSitzung` (echte Sitzungen gehen
   wirklich ins Netz; ein Test baut sich seine eigene und fängt sie über
   `protocolClasses` ab). Das ist derselbe Fehler wie oben — eine vergessene
   Übersteuerung —, nur dass hier keine Adresse dasteht, an der er auffiele.

Auslieferungs-Workflows (Play-Upload, Firebase-Verteilung, GHCR-Push) sind
ausdrücklich nicht betroffen — die dürfen und müssen nach außen.

Einzelfall-Ausnahme: ein Kommentar `ci-extern-ok: <Begründung>` in derselben
Zeile. Bewusst unbequem, damit es eine Entscheidung bleibt.

Aufruf:
    python3 .github/scripts/pruefe_lokale_tests.py
    python3 .github/scripts/pruefe_lokale_tests.py --selbsttest
"""

from __future__ import annotations

import re
import sys
import tempfile
from pathlib import Path

WURZEL = Path(__file__).resolve().parents[2]

# Alles, was Test ist oder Tests aufsetzt. Bewusst NICHT dabei:
# .github/workflows/release.yml (Play/Firebase), .github/scripts/upload_play.py,
# deploy/ und der Produktivcode der App — die dürfen nach außen.
GEPRUEFTE_PFADE = [
    "android/app/src/androidTest",
    "android/app/src/test",
    "android/ci-e2e.sh",
    "android/e2e",
    "backend/e2e",
    "ios/DorfTests",
    ".github/workflows/android.yml",
    ".github/workflows/backend.yml",
    ".github/workflows/ios.yml",
]

ENDUNGEN = {".kt", ".java", ".swift", ".go", ".mjs", ".js", ".ts", ".sh", ".yml", ".yaml", ".json", ".py"}

# 1) Dienste, die ein Test niemals anfassen darf.
VERBOTENE_DIENSTE = [
    (r"id\.xn--rssing-wxa\.de", "Produktions-Zitadel (Rössing-ID)"),
    (r"app\.xn--rssing-wxa\.de", "Produktions-Backend der Dorf-App"),
    (r"tiles\.openfreemap\.org", "fremder Kachelserver"),
    (r"fcm\.googleapis\.com", "Firebase Cloud Messaging"),
    # Beide Apple-Adressen einzeln: „api.sandbox.push.apple.com" enthält
    # „api.push.apple.com" gerade nicht.
    (r"api\.push\.apple\.com", "Apple Push Notification service (Produktion)"),
    (r"api\.sandbox\.push\.apple\.com", "Apple Push Notification service (Sandbox)"),
    (r"firebase\.tools", "Firebase-CLI-Installer"),
    (r"firebaseinstallations\.googleapis\.com", "Firebase Installations"),
]

# 2) Die Produktions-Website darf Datenstring sein, aber kein Endpunkt.
WEBSITE = r"(?:https?://)?xn--rssing-wxa\.de"
ENDPUNKT_WOERTER = r"(?:baseUrl|base_url|BASE_URL|BASEURL|issuer|ISSUER|apiBaseUrl|websiteBaseUrl|mapStyleUrl|oidcIssuer|AUTH_ISSUER|E2E_[A-Z_]*URL)"
ENDPUNKT_MUSTER = re.compile(
    rf"{ENDPUNKT_WOERTER}[^\n]{{0,40}}?{WEBSITE}|{WEBSITE}[^\n]{{0,10}}?[\"']\s*(?:as\s+)?{ENDPUNKT_WOERTER}"
)

FREIGABE = re.compile(r"ci-extern-ok:")

# 3) Jeder Testlauf muss diese Adressen lokal setzen.
PFLICHT_UEBERSTEUERUNGEN = ["-PapiBaseUrl", "-PwebsiteBaseUrl", "-PmapStyleUrl"]

# Dasselbe für iOS: Die Build-Einstellungen aus `ios/project.yml` zeigen auf die
# Produktion, jeder `xcodebuild`-Aufruf der CI muss sie übersteuern.
PFLICHT_UEBERSTEUERUNGEN_IOS = ["API_BASE_URL", "WEBSITE_BASE_URL", "OIDC_ISSUER", "MAP_STYLE_URL"]

# Als lokal gilt der Runner selbst bzw. der Host aus Sicht des Emulators.
LOKALE_ADRESSE = re.compile(r"^(?:http://)?(?:127\.0\.0\.1|localhost|10\.0\.2\.2)(?::\d+)?(?:/|$)")

# 5) iOS-Tests: Struktur statt Zeichenkette.
#
# Ein Test unter `ios/DorfTests/` kann die Produktion erreichen, ohne dass
# eine einzige Adresse in der Datei steht — er muss nur die Vorbelegungen der
# App benutzen. Genau diese Lücke sieht die Sperrliste oben nicht.
IOS_TEST_PFAD = "ios/DorfTests/"

# Zeilenweise beanstandet: Zugriffe, die es in einem Test nicht geben darf.
IOS_VERBOTENE_ZUGRIFFE = [
    (
        re.compile(r"\bKonfiguration\."),
        "liest die Vorbelegungen der App (Konfiguration.…) — die stehen in "
        "ios/project.yml und zeigen auf die Produktion; ein Test setzt seine "
        "Adressen selbst",
    ),
    (
        re.compile(r"\bURLSession\.shared\b"),
        "benutzt URLSession.shared — die geteilte Sitzung geht wirklich ins Netz; "
        "ein Test baut sich eine eigene (URLSessionConfiguration.ephemeral) und "
        "fängt sie über protocolClasses ab",
    ),
    (
        re.compile(r"\bURLSession\.dorfSitzung\b"),
        "benutzt URLSession.dorfSitzung — die Sitzung der App geht wirklich ins "
        "Netz; ein Test baut sich eine eigene und fängt sie über protocolClasses ab",
    ),
]

# Über mehrere Zeilen geprüft: `DorfApi(` ohne ausdrückliche Basis. Der
# Vorgabewert des Initialisierers ist `Konfiguration.apiBasis` — also die
# Produktion. Im Repo wird ein solcher Aufruf umgebrochen, deshalb reicht eine
# Zeilenprüfung hier nicht.
DORFAPI_AUFRUF = re.compile(r"\bDorfApi\s*\(")


def _aufruf_ende(text: str, klammer: int) -> int:
    """Index hinter der schließenden Klammer zur öffnenden an `klammer`.

    Klammern in Zeichenketten werden nicht ausgenommen — in Swift-Quellen
    stehen sie dort paarweise (`URL(string: "…")!`), und ein Fehlgriff hieße
    hier bloß, dass ein Stück mehr oder weniger nach `basis:` durchsucht wird.
    """
    tiefe = 0
    for i in range(klammer, len(text)):
        if text[i] == "(":
            tiefe += 1
        elif text[i] == ")":
            tiefe -= 1
            if tiefe == 0:
                return i + 1
    return len(text)


def pruefe_ios_testquelle(relativ: str, text: str) -> list[str]:
    """Die strukturellen Regeln für `ios/DorfTests/`."""
    funde = []
    for nummer, zeile in enumerate(text.splitlines(), start=1):
        if FREIGABE.search(zeile):
            continue
        for muster, was in IOS_VERBOTENE_ZUGRIFFE:
            if muster.search(zeile):
                funde.append(f"{relativ}:{nummer}: Der Test {was}.\n    {zeile.strip()}")

    for treffer in DORFAPI_AUFRUF.finditer(text):
        klammer = treffer.end() - 1
        ende = _aufruf_ende(text, klammer)
        aufruf = text[klammer:ende]
        if "basis:" in aufruf:
            continue
        # Die Freigabe darf irgendwo in den Zeilen des Aufrufs stehen.
        zeilenanfang = text.rfind("\n", 0, treffer.start()) + 1
        zeilenende = text.find("\n", ende)
        block = text[zeilenanfang : len(text) if zeilenende == -1 else zeilenende]
        if FREIGABE.search(block):
            continue
        nummer = text.count("\n", 0, treffer.start()) + 1
        funde.append(
            f"{relativ}:{nummer}: DorfApi ohne ausdrückliches basis: — dann greift "
            "die Vorbelegung Konfiguration.apiBasis aus ios/project.yml, und der "
            "Test spricht mit dem Produktions-Backend, ohne dass hier eine Adresse "
            "stünde.\n    " + " ".join(block.split())[:160]
        )
    return funde


def _dateien():
    for eintrag in GEPRUEFTE_PFADE:
        pfad = WURZEL / eintrag
        if pfad.is_file():
            yield pfad
        elif pfad.is_dir():
            for datei in sorted(pfad.rglob("*")):
                if datei.is_file() and datei.suffix in ENDUNGEN and "node_modules" not in datei.parts:
                    yield datei


def pruefe_text(relativ: str, text: str) -> list[str]:
    """Prüft einen einzelnen Dateiinhalt. Getrennt, damit der Selbsttest greift."""
    funde = []
    # Die Sperrliste dieser Prüfung selbst nennt die Adressen naturgemäß.
    if relativ == ".github/scripts/pruefe_lokale_tests.py":
        return funde
    for nummer, zeile in enumerate(text.splitlines(), start=1):
        if FREIGABE.search(zeile):
            continue
        for muster, was in VERBOTENE_DIENSTE:
            if re.search(muster, zeile):
                funde.append(f"{relativ}:{nummer}: {was} — Tests laufen ausschließlich lokal.\n    {zeile.strip()}")
        if ENDPUNKT_MUSTER.search(zeile):
            funde.append(
                f"{relativ}:{nummer}: Produktions-Website als Endpunkt konfiguriert — "
                f"im Test muss eine lokale Ablage stehen.\n    {zeile.strip()}"
            )
    if relativ.startswith(IOS_TEST_PFAD):
        funde += pruefe_ios_testquelle(relativ, text)
    return funde


def pruefe_uebersteuerungen() -> list[str]:
    """`android/ci-e2e.sh`: kein Testlauf ohne lokale Adressen."""
    funde = []
    quelle = (WURZEL / "android/ci-e2e.sh").read_text(encoding="utf-8")
    # Fortsetzungszeilen zusammenziehen, der Aufruf ist mehrzeilig.
    zusammen = quelle.replace("\\\n", " ")
    for zeile in zusammen.splitlines():
        if "connectedDebugAndroidTest" not in zeile:
            continue
        fehlend = [p for p in PFLICHT_UEBERSTEUERUNGEN if p not in zeile]
        if fehlend:
            funde.append(
                "android/ci-e2e.sh: Ein Testlauf übersteuert "
                f"{', '.join(fehlend)} nicht — dann greifen die Produktions-Vorbelegungen "
                f"aus build.gradle.kts.\n    {zeile.strip()[:160]}"
            )
    return funde


def pruefe_ios_text(text: str) -> list[str]:
    """`.github/workflows/ios.yml`: kein xcodebuild ohne lokale Adressen.

    Getrennt vom Dateizugriff, damit der Selbsttest greift.
    """
    funde = []
    # Fortsetzungszeilen zusammenziehen, die Aufrufe sind mehrzeilig.
    for zeile in text.replace("\\\n", " ").splitlines():
        if "xcodebuild" not in zeile or not re.search(r"\b(build|test)\b", zeile):
            continue
        for name in PFLICHT_UEBERSTEUERUNGEN_IOS:
            treffer = re.search(rf"\b{name}=(\S+)", zeile)
            if treffer is None:
                funde.append(
                    f".github/workflows/ios.yml: Ein xcodebuild-Aufruf setzt {name} nicht — "
                    "dann greift die Produktions-Vorbelegung aus ios/project.yml.\n    "
                    + zeile.strip()[:160]
                )
            elif not LOKALE_ADRESSE.match(treffer.group(1)):
                funde.append(
                    f".github/workflows/ios.yml: {name} zeigt nicht auf einen lokalen Dienst "
                    f"({treffer.group(1)}).\n    " + zeile.strip()[:160]
                )
    return funde


def pruefe_ios_uebersteuerungen() -> list[str]:
    workflow = WURZEL / ".github/workflows/ios.yml"
    if not workflow.is_file():
        return []
    return pruefe_ios_text(workflow.read_text(encoding="utf-8"))


def pruefe_push() -> list[str]:
    """`PushEchtTest` spricht über Google mit echtem FCM — nie in der CI.

    Der Test schützt sich selbst mit `assumeTrue(push == "true")`. Genau dieses
    Argument darf im CI-Skript nicht auftauchen.
    """
    quelle = (WURZEL / "android/ci-e2e.sh").read_text(encoding="utf-8")
    if re.search(r"\bpush\s+true\b|push=true|PushEchtTest", quelle):
        return [
            "android/ci-e2e.sh: schaltet den echten Push-Test scharf — der läuft "
            "über Google (FCM) und gehört nicht in die CI."
        ]
    return []


def pruefe_secrets() -> list[str]:
    """Der E2E-Job darf keine Zugangsdaten mehr bekommen."""
    funde = []
    workflow = (WURZEL / ".github/workflows/android.yml").read_text(encoding="utf-8")
    for nummer, zeile in enumerate(workflow.splitlines(), start=1):
        if re.search(r"secrets\.TEST_USER", zeile):
            funde.append(
                f".github/workflows/android.yml:{nummer}: Zugangsdaten aus GitHub-Secrets — "
                "das Testkonto entsteht im lokalen Zitadel.\n    " + zeile.strip()
            )
    return funde


def selbsttest() -> int:
    """Beweist, dass die Prüfung anschlägt — sonst wäre sie ein Placebo."""
    faelle = [
        ("android/app/src/androidTest/Boese.kt", 'val issuer = "https://id.xn--rssing-wxa.de"', True),
        ("backend/e2e/boese.mjs", "const s = 'https://tiles.openfreemap.org/styles/liberty'", True),
        ("backend/e2e/boese.go", 'ziel := "https://api.push.apple.com/3/device/" + token', True),
        ("backend/e2e/boese.go", 'ziel := "https://api.sandbox.push.apple.com/3/device/" + token', True),
        ("backend/e2e/gut.go", 'ziel := srv.URL + "/3/device/" + token', False),
        ("android/app/src/test/Boese.kt", 'val websiteBaseUrl = "https://xn--rssing-wxa.de"', True),
        # Datenstrings bleiben erlaubt — Termine tragen nun einmal solche Links.
        ("android/app/src/test/Gut.kt", 'url = "https://xn--rssing-wxa.de/events/grillen"', False),
        ("backend/e2e/gut.go", '"IDEEN_ZIELE=https://xn--rssing-wxa.de"', False),
        # Ausdrückliche Einzelfreigabe.
        ("android/app/src/test/Frei.kt", 'val issuer = "https://id.xn--rssing-wxa.de" // ci-extern-ok: Beispiel', False),
        # iOS: Swift-Testquellen werden genauso geprüft.
        ("ios/DorfTests/BoeseTests.swift", 'let aussteller = URL(string: "https://id.xn--rssing-wxa.de")!', True),
        ("ios/DorfTests/GutTests.swift", 'let link = "https://xn--rssing-wxa.de/termine/grillen"', False),
        # iOS, strukturell: der Weg in die Produktion ganz ohne Adresse.
        ("ios/DorfTests/BoeseTests.swift", "let basis = Konfiguration.apiBasis", True),
        ("ios/DorfTests/BoeseTests.swift", "let id = Konfiguration.oidcClientId", True),
        ("ios/DorfTests/BoeseTests.swift", "let api = DorfApi(tokenGeber: { nil })", True),
        # So wird im Repo formatiert — mehrzeilig, die Basis fehlt trotzdem.
        (
            "ios/DorfTests/BoeseTests.swift",
            "let api = DorfApi(\n"
            "    sitzung: URLSession(configuration: k),\n"
            "    tokenGeber: { nil }\n"
            ")",
            True,
        ),
        ("ios/DorfTests/BoeseTests.swift", "let (d, _) = try await URLSession.shared.data(from: u)", True),
        ("ios/DorfTests/BoeseTests.swift", "let (d, _) = try await URLSession.dorfSitzung.data(from: u)", True),
        # Erlaubt: ausdrückliche lokale Basis, einzeilig wie mehrzeilig.
        (
            "ios/DorfTests/GutTests.swift",
            'let api = DorfApi(basis: URL(string: "http://127.0.0.1:8099")!, tokenGeber: { nil })',
            False,
        ),
        (
            "ios/DorfTests/GutTests.swift",
            "let api = DorfApi(\n"
            '    basis: URL(string: "http://127.0.0.1:8099")!,\n'
            "    sitzung: URLSession(configuration: k),\n"
            "    tokenGeber: { nil }\n"
            ")",
            False,
        ),
        # Eine selbst gebaute, abgefangene Sitzung ist genau der richtige Weg.
        ("ios/DorfTests/GutTests.swift", "let sitzung = URLSession(configuration: k)", False),
        # Die Einzelfreigabe gilt auch hier — und auch über mehrere Zeilen.
        ("ios/DorfTests/FreiTests.swift", "let basis = Konfiguration.apiBasis // ci-extern-ok: Beispiel", False),
        ("ios/DorfTests/FreiTests.swift", "let api = DorfApi(tokenGeber: { nil }) // ci-extern-ok: Beispiel", False),
        # Der Produktivcode der App darf all das — geprüft wird nur ios/DorfTests/.
        ("ios/Dorf/Umgebung.swift", "let api = DorfApi(tokenGeber: { nil })", False),
    ]
    fehler = 0
    for name, zeile, erwartet_treffer in faelle:
        getroffen = bool(pruefe_text(name, zeile))
        if getroffen != erwartet_treffer:
            print(f'SELBSTTEST FEHLGESCHLAGEN: „{zeile}“ → Treffer={getroffen}, erwartet={erwartet_treffer}')
            fehler += 1

    # Und die Übersteuerungs-Prüfung: ein Aufruf ohne -PwebsiteBaseUrl muss auffallen.
    with tempfile.TemporaryDirectory() as tmp:
        probe = Path(tmp) / "ci-e2e.sh"
        probe.write_text("./gradlew connectedDebugAndroidTest -PapiBaseUrl=http://10.0.2.2:8099\n", encoding="utf-8")
        zusammen = probe.read_text(encoding="utf-8")
        fehlend = [p for p in PFLICHT_UEBERSTEUERUNGEN if p not in zusammen]
        if not fehlend:
            print("SELBSTTEST FEHLGESCHLAGEN: fehlende Übersteuerung wurde nicht erkannt")
            fehler += 1

    # Und für iOS: ein xcodebuild-Aufruf, dem eine Adresse fehlt oder der eine
    # entfernte trägt, muss auffallen — ein vollständig lokaler nicht.
    vollstaendig = (
        "xcodebuild test -project Dorf.xcodeproj -scheme Dorf "
        "API_BASE_URL=http://127.0.0.1:8099 WEBSITE_BASE_URL=http://127.0.0.1:8097 "
        "OIDC_ISSUER=http://127.0.0.1:8123 MAP_STYLE_URL=http://127.0.0.1:8097/map-style.json"
    )
    ios_faelle = [
        (vollstaendig, False),
        # Mehrzeilig mit Fortsetzungszeichen — so steht es im Workflow.
        (vollstaendig.replace(" API_BASE_URL", " \\\n  API_BASE_URL"), False),
        (vollstaendig.replace(" MAP_STYLE_URL=http://127.0.0.1:8097/map-style.json", ""), True),
        (vollstaendig.replace("http://127.0.0.1:8123", "https://id.xn--rssing-wxa.de"), True),
        # Kein Testlauf, kein Anspruch.
        ("xcodebuild -showBuildSettings -project Dorf.xcodeproj", False),
    ]
    for aufruf, erwartet_treffer in ios_faelle:
        getroffen = bool(pruefe_ios_text(aufruf))
        if getroffen != erwartet_treffer:
            print(f'SELBSTTEST FEHLGESCHLAGEN (iOS): „{aufruf[:80]}…“ → Treffer={getroffen}, erwartet={erwartet_treffer}')
            fehler += 1

    if fehler:
        return 1
    print(
        f"Selbsttest bestanden ({len(faelle)} Fälle + Übersteuerungs-Prüfung "
        f"+ {len(ios_faelle)} iOS-Fälle)."
    )
    return 0


def main() -> int:
    if "--selbsttest" in sys.argv:
        return selbsttest()

    funde: list[str] = []
    geprueft = 0
    for datei in _dateien():
        geprueft += 1
        relativ = datei.relative_to(WURZEL).as_posix()
        funde += pruefe_text(relativ, datei.read_text(encoding="utf-8", errors="replace"))
    funde += pruefe_uebersteuerungen()
    funde += pruefe_ios_uebersteuerungen()
    funde += pruefe_push()
    funde += pruefe_secrets()

    if funde:
        print("Tests dürfen ausschließlich gegen lokale Dienste laufen. Gefunden:\n")
        for fund in funde:
            print(f"  {fund}")
        print(
            "\nAbhilfe: Adresse auf einen Dienst umstellen, den die CI selbst startet "
            "(docker compose in backend/e2e, statische Ablage in android/e2e/fixtures). "
            "Bei den iOS-Treffern ohne Adresse: dem DorfApi-Aufruf eine eigene "
            "'basis:' mitgeben, die Sitzung selbst bauen und über 'protocolClasses' "
            "abfangen, statt Konfiguration.* zu lesen. "
            "Ist der Treffer wirklich nur ein Datenstring, hilft ein Kommentar "
            "'ci-extern-ok: <Begruendung>' in derselben Zeile."
        )
        return 1

    print(f"{geprueft} Test- und CI-Dateien geprüft: kein Zugriff auf entfernte Server.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
