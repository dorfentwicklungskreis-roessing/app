#!/usr/bin/env python3
"""Kleiner Zugang zur App-Store-Connect-API — ohne Fremdbibliotheken.

Apple verlangt für jede Anfrage ein kurzlebiges, mit ES256 signiertes JWT.
Dafür gäbe es fertige Pakete; hier steht es selbst, weil das Projekt sich
Abhängigkeiten grundsätzlich verkneift und die Signatur mit `openssl` und
vierzig Zeilen erledigt ist. Gleicher Grund wie beim Backend (`internal/push`
erzeugt sein Google-Token auch selbst).

Der private Schlüssel liegt **nie im Repo**, sondern dort, wo Xcode und
`xcrun altool` von selbst nachsehen:

    ~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8

Kennungen kommen aus der Umgebung oder aus den Parametern:

    export APP_STORE_CONNECT_KEY_ID=...      # z.B. 75C8P9JB9F
    export APP_STORE_CONNECT_ISSUER_ID=...   # UUID, für alle Schlüssel gleich

Aufruf:

    python3 store/asc.py GET /v1/bundleIds
    python3 store/asc.py GET '/v1/apps?limit=10'
    python3 store/asc.py POST /v1/bundleIds --daten '{"data": {...}}'

Dazu die Handgriffe, die man sonst in App Store Connect klicken müsste:

    python3 store/asc.py team-id             # Team-ID aus einem Zertifikat lesen
    python3 store/asc.py bundle-id-anlegen   # de.roessing.app registrieren
    python3 store/asc.py app-zeigen          # App-Datensatz suchen, id ausgeben
    python3 store/asc.py testflight-gruppe   # externe Gruppe „Dorf" + öffentlicher Link
    python3 store/asc.py beta-info           # Feedback-Adresse und Beta-Beschreibung
    python3 store/asc.py screenshots-hochladen  # Store-Bilder aus store/screenshots/ios/

Und die Handgriffe für den Store-Eintrag selbst:

    python3 store/asc.py kategorien          # Kategorien + Rechteangabe
    python3 store/asc.py alterseinstufung    # Fragebogen zur Altersfreigabe
    python3 store/asc.py pruefangaben        # Kontakt, Prüfkonto, Notiz an die Prüfung
    python3 store/asc.py version-angaben     # Copyright und Freigabeart
    python3 store/asc.py verfuegbarkeit      # kostenlos, weltweit
    python3 store/asc.py einreichstand       # was zur Einreichung noch fehlt

Jeder davon versteht `--probe`: dann wird gezeigt, was geschickt würde, und
nichts geschickt.

Was hier bewusst NICHT geht: den App-Datensatz selbst anlegen. Dafür gibt es
keine API — das macht ein Mensch einmalig in App Store Connect
(`store/ios-veroeffentlichung.md`, Schritt 3). Ebenso wenig geht hier die
Einreichung zur Prüfung: Der letzte Knopf gehört dem Menschen, der dafür
geradesteht. Und die Datenschutzangaben („App Privacy") führt Apple gar nicht
über die API — auch die bleiben Handarbeit.

Ohne Schlüssel bricht kein Unterbefehl mit einem Stacktrace ab, sondern sagt,
was fehlt und wie man es hinlegt.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

# Trockenlauf: `--probe` zeigt jede schreibende Anfrage, statt sie zu schicken.
# Die Objekte in App Store Connect gehören einem echten Verein — wer einen
# Unterbefehl zum ersten Mal ausprobiert, soll erst sehen, was er auslöst.
PROBE = False

# Ziel-Version für `screenshots-hochladen` (--store-version). Leer heißt:
# die neueste bearbeitbare Version der App nehmen.
STORE_VERSION = None

BASIS = "https://api.appstoreconnect.apple.com"
ZIELGRUPPE = "appstoreconnect-v1"
# Apple weist Tokens ab, die länger als 20 Minuten gelten.
GUELTIGKEIT = 20 * 60

# Dieselben Werte wie in ios/project.yml bzw. store/metadata/ios/ — hier stehen
# sie noch einmal, weil dieses Skript ohne Xcode und ohne XcodeGen laufen soll.
BUNDLE_ID = "de.roessing.app"
BUNDLE_NAME = "Roessing Dorf-App"   # nur intern; Apple verbietet hier Umlaute
TESTFLIGHT_GRUPPE = "Dorf"
FEEDBACK_ADRESSE = "post@levinkeller.de"
METADATEN = Path(__file__).resolve().parent / "metadata" / "ios"
SPRACHEN = ["de-DE", "en-US"]

# Store-Bilder: store/screenshots/ios/<sprache>/<geraet>/NN-name.png
BILDER = Path(__file__).resolve().parent / "screenshots" / "ios"

# Ordnername → Anzeigetyp in App Store Connect. Die gültigen Werte sind nicht
# geraten: Apple nennt sie in der Fehlermeldung, wenn man einen falschen
# schickt (ENTITY_ERROR.ATTRIBUTE.TYPE bei POST /v1/appScreenshotSets).
# Für 6,9″ (1320×2868) gibt es keinen eigenen Typ — er gehört zu
# APP_IPHONE_67, genau wie 6,5″ und 6,7″.
ANZEIGETYPEN = {
    "iphone-6_9": "APP_IPHONE_67",
    "ipad-13": "APP_IPAD_PRO_3GEN_129",
}

# In diesen Zuständen lässt sich eine Store-Version noch bearbeiten. Alles
# andere (IN_REVIEW, READY_FOR_SALE …) weist Apple ab — dann soll der
# Unterbefehl das sagen und nichts anfassen.
BEARBEITBAR = {
    "PREPARE_FOR_SUBMISSION", "DEVELOPER_REJECTED", "REJECTED",
    "METADATA_REJECTED", "INVALID_BINARY",
}


def _b64(roh: bytes) -> str:
    """base64url ohne Auffüllzeichen — so verlangt es die JWT-Spezifikation."""
    return base64.urlsafe_b64encode(roh).decode().rstrip("=")


def _der_zu_roh(der: bytes) -> bytes:
    """ECDSA-Signatur von DER (openssl) nach R||S (JWT).

    openssl liefert `SEQUENCE { INTEGER r, INTEGER s }`; ein JWT will die
    beiden Zahlen schlicht hintereinander, je auf 32 Byte aufgefüllt. Führende
    Nullbytes, die DER zur Vorzeichentrennung einfügt, müssen dabei weg.
    """
    if not der or der[0] != 0x30:
        raise ValueError("keine DER-Sequenz")
    laenge = der[1]
    rest = der[2:]
    if laenge & 0x80:  # lange Längenform
        anzahl = laenge & 0x7F
        rest = der[2 + anzahl:]

    zahlen = []
    for _ in range(2):
        if rest[0] != 0x02:
            raise ValueError("keine DER-Ganzzahl")
        n = rest[1]
        wert = rest[2:2 + n].lstrip(b"\x00")
        zahlen.append(wert.rjust(32, b"\x00"))
        rest = rest[2 + n:]
    return zahlen[0] + zahlen[1]


def schluesselpfad(key_id: str) -> Path:
    eigen = os.environ.get("APP_STORE_CONNECT_PRIVATE_KEY_PATH")
    if eigen:
        return Path(eigen).expanduser()
    return Path.home() / ".appstoreconnect" / "private_keys" / f"AuthKey_{key_id}.p8"


HINWEIS_OHNE_SCHLUESSEL = """Kein Zugang zur App-Store-Connect-API — es fehlt: {fehlt}.

Gebraucht werden drei Dinge:

  APP_STORE_CONNECT_KEY_ID      die zehnstellige Key ID, z.B. 2X9R4HXF34
  APP_STORE_CONNECT_ISSUER_ID   eine UUID, für das ganze Team dieselbe
  ~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8   (chmod 600)

Im Repo dorfentwicklungskreis-roessing/app liegen dieselben drei Werte als
GitHub-Secrets (APP_STORE_CONNECT_ISSUER_ID, APP_STORE_CONNECT_KEY_ID,
APP_STORE_CONNECT_PRIVATE_KEY); von dort kommt kein Klartext zurück. Auf dem
eigenen Rechner (die .p8 liegt im Passwortspeicher, nicht im Repo):

  export APP_STORE_CONNECT_KEY_ID=...
  export APP_STORE_CONNECT_ISSUER_ID=...
  mkdir -p ~/.appstoreconnect/private_keys
  cp AuthKey_$APP_STORE_CONNECT_KEY_ID.p8 ~/.appstoreconnect/private_keys/
  chmod 600 ~/.appstoreconnect/private_keys/AuthKey_$APP_STORE_CONNECT_KEY_ID.p8

Es wurde nichts geändert."""


def zugang(key_id: str | None = None, issuer_id: str | None = None) -> tuple[str, str]:
    """Kennungen und Schlüsseldatei einsammeln — oder verständlich abbrechen.

    Bewusst *vor* jedem Unterbefehl aufgerufen: Wer den Schlüssel nicht hat
    (auf dieser Maschine der Normalfall), soll eine Anleitung lesen und keinen
    Stacktrace.
    """
    key_id = key_id or os.environ.get("APP_STORE_CONNECT_KEY_ID", "")
    issuer_id = issuer_id or os.environ.get("APP_STORE_CONNECT_ISSUER_ID", "")

    fehlt = []
    if not key_id:
        fehlt.append("APP_STORE_CONNECT_KEY_ID")
    if not issuer_id:
        fehlt.append("APP_STORE_CONNECT_ISSUER_ID")
    if key_id and not schluesselpfad(key_id).exists():
        fehlt.append(f"die Schlüsseldatei {schluesselpfad(key_id)}")

    if fehlt:
        raise SystemExit(HINWEIS_OHNE_SCHLUESSEL.format(fehlt=", ".join(fehlt)))
    return key_id, issuer_id


def token(key_id: str, issuer_id: str) -> str:
    pfad = schluesselpfad(key_id)
    jetzt = int(time.time())
    kopf = {"alg": "ES256", "kid": key_id, "typ": "JWT"}
    rumpf = {
        "iss": issuer_id,
        "iat": jetzt,
        "exp": jetzt + GUELTIGKEIT,
        "aud": ZIELGRUPPE,
    }
    zusignieren = f"{_b64(json.dumps(kopf, separators=(',', ':')).encode())}." \
                  f"{_b64(json.dumps(rumpf, separators=(',', ':')).encode())}"

    lauf = subprocess.run(
        ["openssl", "dgst", "-sha256", "-sign", str(pfad)],
        input=zusignieren.encode(), capture_output=True,
    )
    if lauf.returncode != 0:
        raise SystemExit(f"Signieren fehlgeschlagen: {lauf.stderr.decode().strip()}")
    return f"{zusignieren}.{_b64(_der_zu_roh(lauf.stdout))}"


def _ohne_geheimnisse(wert):
    """Kopie eines Anfragerumpfs, in der Passwörter durch Sterne ersetzt sind.

    Nur für den Trockenlauf: Der zeigt den ganzen Rumpf, und das Passwort des
    Prüfkontos hat weder im Terminal noch in einem Workflow-Protokoll etwas
    verloren.
    """
    if isinstance(wert, dict):
        return {schluessel: ("********" if "assword" in schluessel
                             else _ohne_geheimnisse(inhalt))
                for schluessel, inhalt in wert.items()}
    if isinstance(wert, list):
        return [_ohne_geheimnisse(eintrag) for eintrag in wert]
    return wert


def anfrage(methode: str, pfad: str, daten: dict | None = None,
            key_id: str | None = None, issuer_id: str | None = None,
            dulden: tuple[int, ...] = ()) -> dict:
    """Eine API-Anfrage. `dulden` nennt HTTP-Codes, die kein Abbruch sind.

    Für die gibt es die Antwort mit zusätzlichem Schlüssel `__status` zurück —
    so kann ein Unterbefehl z.B. auf „gibt es schon" (409) freundlich
    antworten, statt den Lauf rot zu machen.
    """
    key_id, issuer_id = zugang(key_id, issuer_id)

    if PROBE and methode.upper() != "GET":
        print(f"[Trockenlauf] {methode.upper()} {pfad}")
        if daten is not None:
            for zeile in json.dumps(_ohne_geheimnisse(daten), indent=2,
                                    ensure_ascii=False).splitlines():
                print(f"    {zeile}")
        # Genug Gerüst, damit die aufrufende Funktion weiterläuft und ihre
        # restlichen Schritte ebenfalls zeigt.
        return {"data": {"id": "(Trockenlauf)", "type": "", "attributes": {}}}

    rumpf = json.dumps(daten).encode() if daten is not None else None
    req = urllib.request.Request(BASIS + pfad, data=rumpf, method=methode)
    req.add_header("Authorization", f"Bearer {token(key_id, issuer_id)}")
    if rumpf is not None:
        req.add_header("Content-Type", "application/json")

    try:
        with urllib.request.urlopen(req, timeout=30) as antwort:
            roh = antwort.read()
            return json.loads(roh) if roh else {}
    except urllib.error.HTTPError as fehler:
        roh = fehler.read().decode(errors="replace")
        if fehler.code in dulden:
            try:
                gelesen = json.loads(roh)
            except json.JSONDecodeError:
                gelesen = {"rohtext": roh}
            gelesen["__status"] = fehler.code
            return gelesen
        try:
            # Apple erklärt Ablehnungen ausführlich — den Text nicht verschlucken.
            gelesen = json.loads(roh)
            print(json.dumps(gelesen, indent=2, ensure_ascii=False), file=sys.stderr)
        except json.JSONDecodeError:
            print(roh, file=sys.stderr)
        raise SystemExit(f"HTTP {fehler.code} bei {methode} {pfad}")


def team_id() -> str:
    """Die zehnstellige Team-ID — sie steht in jedem Zertifikat als OU.

    Ein eigenes Feld dafür gibt es in der API nicht; wer sie sucht, findet sie
    entweder auf der Mitgliedschaftsseite oder eben hier.
    """
    antwort = anfrage("GET", "/v1/certificates?limit=200")
    for eintrag in antwort.get("data", []):
        inhalt = eintrag.get("attributes", {}).get("certificateContent")
        if not inhalt:
            continue
        pem = "-----BEGIN CERTIFICATE-----\n" + inhalt + "\n-----END CERTIFICATE-----\n"
        lauf = subprocess.run(
            ["openssl", "x509", "-noout", "-subject"],
            input=pem.encode(), capture_output=True,
        )
        for teil in lauf.stdout.decode().replace("/", ",").split(","):
            teil = teil.strip()
            if teil.startswith("OU="):
                return teil[3:].strip()
    raise SystemExit(
        "Keine Team-ID gefunden — es gibt noch kein Zertifikat. "
        "Sie steht auf developer.apple.com unter Account → Membership details."
    )


# ---------------------------------------------------------------------------
# Unterbefehle: die Handgriffe, die sonst in App Store Connect geklickt werden
# ---------------------------------------------------------------------------


def _lies(sprache: str, *dateien: str) -> str:
    """Erste vorhandene, nicht leere Metadatendatei einer Sprache.

    Reihenfolge ist Absicht: `beta_description.txt` ist die kurze Fassung für
    TestFlight; fehlt sie, tut es die Store-Beschreibung.
    """
    for name in dateien:
        pfad = METADATEN / sprache / name
        if pfad.is_file():
            text = pfad.read_text(encoding="utf-8").strip()
            if text:
                return text
    raise SystemExit(
        f"Keine der Dateien {', '.join(dateien)} unter "
        f"{(METADATEN / sprache).relative_to(Path(__file__).resolve().parent.parent)} "
        "gefunden oder alle leer."
    )


def bundle_id_anlegen() -> None:
    """`de.roessing.app` als App ID registrieren (POST /v1/bundleIds).

    Eine Bundle-ID lässt sich weder umbenennen noch löschen — deshalb wird
    vorher gesucht statt blind angelegt.
    """
    zugang()
    vorhanden = anfrage("GET", f"/v1/bundleIds?filter[identifier]={BUNDLE_ID}&limit=200")
    for eintrag in vorhanden.get("data", []):
        if eintrag.get("attributes", {}).get("identifier") == BUNDLE_ID:
            print(f"{BUNDLE_ID} gibt es schon (id {eintrag['id']}) — nichts zu tun.")
            return

    antwort = anfrage("POST", "/v1/bundleIds", {
        "data": {
            "type": "bundleIds",
            "attributes": {
                "identifier": BUNDLE_ID,
                "name": BUNDLE_NAME,
                "platform": "IOS",
            },
        },
    }, dulden=(409,))

    if antwort.get("__status") == 409:
        # Kommt vor, wenn die ID einem anderen Team gehört oder gerade eben
        # von Hand angelegt wurde. Apples Text ist aussagekräftig — durchreichen.
        for fehler in antwort.get("errors", [{}]):
            print(f"{BUNDLE_ID} konnte nicht angelegt werden: "
                  f"{fehler.get('detail') or fehler.get('title') or antwort}")
        return

    print(f"{BUNDLE_ID} angelegt (id {antwort['data']['id']}).")
    print("Capabilities bleiben bewusst leer — siehe store/ios-veroeffentlichung.md, Schritt 2.")


def app_datensatz() -> dict:
    """Den App-Datensatz zur Bundle-ID holen. Fehlt er, wird das erklärt."""
    zugang()
    antwort = anfrage("GET", f"/v1/apps?filter[bundleId]={BUNDLE_ID}&limit=10")
    daten = antwort.get("data", [])
    if not daten:
        raise SystemExit(
            f"Zu {BUNDLE_ID} gibt es noch keinen App-Datensatz.\n"
            "Den legt ein Mensch einmalig an — dafür hat Apple keine API:\n"
            "  appstoreconnect.apple.com → Apps → ＋ → New App\n"
            "Die Felder stehen in store/ios-veroeffentlichung.md, Schritt 3.\n"
            "Es wurde nichts geändert."
        )
    return daten[0]


def app_zeigen() -> None:
    """Nachschauen, ob und wie der App-Datensatz da ist.

    Der einzige Weg, das von außen zu prüfen: Der private Schlüssel liegt nur
    als GitHub-Secret vor, also läuft die Nachfrage über einen Workflow-Lauf.
    Deshalb gibt dieser Unterbefehl gleich alles aus, was man dabei wissen
    will — Kennung, Zustand der Store-Version und die letzten Builds.
    """
    app = app_datensatz()
    a = app.get("attributes", {})
    print(f"App-Id:     {app['id']}")
    print(f"Name:       {a.get('name')}")
    print(f"Bundle-ID:  {a.get('bundleId')}")
    print(f"SKU:        {a.get('sku')}")
    print(f"Sprache:    {a.get('primaryLocale')}")

    versionen = anfrage(
        "GET", f"/v1/apps/{app['id']}/appStoreVersions?limit=5"
    ).get("data", [])
    if versionen:
        print("Store-Versionen (neueste zuerst):")
        for eintrag in versionen:
            v = eintrag.get("attributes", {})
            # appStoreState heißt in neueren Fassungen der API appVersionState;
            # Apple liefert je nach Zeitpunkt das eine oder das andere.
            zustand = v.get("appVersionState") or v.get("appStoreState") or "?"
            print(f"  {v.get('versionString', '?')}  {zustand}")
    else:
        print("Store-Versionen: noch keine — die App wurde nie eingereicht.")

    # Bewusst /v1/builds mit Filter statt /v1/apps/<id>/builds: Die
    # Beziehungs-Adresse lehnt `sort` ab („The parameter 'sort' can not be used
    # with this request"), und ohne Sortierung ist „die letzten fünf" sinnlos.
    builds = anfrage(
        "GET", f"/v1/builds?filter[app]={app['id']}&limit=5&sort=-uploadedDate"
    ).get("data", [])
    if builds:
        print("Builds (neueste zuerst):")
        for eintrag in builds:
            b = eintrag.get("attributes", {})
            abgelaufen = " (abgelaufen)" if b.get("expired") else ""
            print(f"  Nr. {b.get('version', '?')}  {b.get('processingState', '?')}"
                  f"  {b.get('uploadedDate', '?')}{abgelaufen}")
    else:
        print("Builds: noch keiner hochgeladen — TestFlight ist damit leer.")

    gruppen = anfrage("GET", f"/v1/apps/{app['id']}/betaGroups?limit=200").get("data", [])
    if gruppen:
        print("TestFlight-Gruppen:")
        for eintrag in gruppen:
            g = eintrag.get("attributes", {})
            art = "intern" if g.get("isInternalGroup") else "extern"
            link = g.get("publicLink") or "kein öffentlicher Link"
            print(f"  {g.get('name')} ({art}) — {link}")
    else:
        print("TestFlight-Gruppen: noch keine.")


def testflight_gruppe() -> None:
    """Externe Tester-Gruppe „Dorf" anlegen und den öffentlichen Link freischalten.

    Extern heißt: ohne Team-Mitgliedschaft, bis 10.000 Plätze, dafür einmal
    Beta App Review. Der öffentliche Link ist der Grund für „extern" — damit
    reicht ein Link ins Dorf statt einer Einladung je Adresse.
    """
    app = app_datensatz()
    gruppen = anfrage("GET", f"/v1/apps/{app['id']}/betaGroups?limit=200")

    gefunden = None
    for eintrag in gruppen.get("data", []):
        if eintrag.get("attributes", {}).get("name") == TESTFLIGHT_GRUPPE:
            gefunden = eintrag
            break

    if gefunden is None:
        antwort = anfrage("POST", "/v1/betaGroups", {
            "data": {
                "type": "betaGroups",
                "attributes": {
                    "name": TESTFLIGHT_GRUPPE,
                    "isInternalGroup": False,
                    "publicLinkEnabled": True,
                    # Kein Deckel: Rössing hat rund 1.500 Einwohner, die
                    # 10.000 Plätze von TestFlight reichen also mit Abstand.
                    "publicLinkLimitEnabled": False,
                    "feedbackEnabled": True,
                },
                "relationships": {
                    "app": {"data": {"type": "apps", "id": app["id"]}},
                },
            },
        })
        gefunden = antwort["data"]
        print(f"Gruppe „{TESTFLIGHT_GRUPPE}“ angelegt (id {gefunden['id']}).")
    elif not gefunden.get("attributes", {}).get("publicLinkEnabled"):
        antwort = anfrage("PATCH", f"/v1/betaGroups/{gefunden['id']}", {
            "data": {
                "id": gefunden["id"],
                "type": "betaGroups",
                "attributes": {"publicLinkEnabled": True, "publicLinkLimitEnabled": False},
            },
        })
        gefunden = antwort["data"]
        print(f"Gruppe „{TESTFLIGHT_GRUPPE}“ gab es schon — öffentlicher Link eingeschaltet.")
    else:
        print(f"Gruppe „{TESTFLIGHT_GRUPPE}“ gibt es schon, öffentlicher Link ist an.")

    link = gefunden.get("attributes", {}).get("publicLink")
    if not link and not PROBE:
        # Apple erzeugt den Link asynchron; direkt nach dem Anlegen ist er oft
        # noch leer. Einmal nachfragen genügt in aller Regel.
        link = anfrage("GET", f"/v1/betaGroups/{gefunden['id']}") \
            .get("data", {}).get("attributes", {}).get("publicLink")

    if link:
        print(f"Öffentlicher Link: {link}")
    elif PROBE:
        print("Öffentlicher Link: entsteht erst beim echten Lauf.")
    else:
        print("Öffentlicher Link noch nicht erzeugt — in ein paar Minuten erneut aufrufen.")
    print("Der Link trägt erst, wenn ein Build die Beta App Review bestanden hat.")


def beta_info() -> None:
    """Beta-Angaben je Sprache setzen (betaAppLocalizations).

    Das ist der Inhalt von *TestFlight → Test Information*: Beschreibung,
    Feedback-Adresse, Marketing- und Datenschutz-Adresse. Ohne sie nimmt Apple
    keinen externen Test an.
    """
    app = app_datensatz()
    vorhanden = anfrage("GET", f"/v1/apps/{app['id']}/betaAppLocalizations?limit=50")
    nach_sprache = {e["attributes"]["locale"]: e["id"] for e in vorhanden.get("data", [])}

    for sprache in SPRACHEN:
        eigenschaften = {
            "description": _lies(sprache, "beta_description.txt", "description.txt"),
            "feedbackEmail": FEEDBACK_ADRESSE,
            "marketingUrl": _lies(sprache, "marketing_url.txt"),
            "privacyPolicyUrl": _lies(sprache, "privacy_url.txt"),
        }
        if sprache in nach_sprache:
            kennung = nach_sprache[sprache]
            anfrage("PATCH", f"/v1/betaAppLocalizations/{kennung}", {
                "data": {"id": kennung, "type": "betaAppLocalizations",
                         "attributes": eigenschaften},
            })
            print(f"{sprache}: Beta-Angaben aktualisiert.")
        else:
            anfrage("POST", "/v1/betaAppLocalizations", {
                "data": {
                    "type": "betaAppLocalizations",
                    "attributes": dict(eigenschaften, locale=sprache),
                    "relationships": {"app": {"data": {"type": "apps", "id": app["id"]}}},
                },
            })
            print(f"{sprache}: Beta-Angaben angelegt.")

    print(f"Feedback-Adresse: {FEEDBACK_ADRESSE}")
    print("Was hier NICHT gesetzt wird: das Prüfkonto für die Beta App Review "
          "(apple.review + Passwort). Das Passwort gehört nicht ins Repo und "
          "wird von Hand eingetragen — siehe store/ios-veroeffentlichung.md.")


# ---------------------------------------------------------------------------
# Der Store-Eintrag: Kategorien, Alterseinstufung, Prüfangaben, Preis
# ---------------------------------------------------------------------------

# Kategorien (2026 über /v1/appCategories geprüft — die Liste ändert sich).
#
# Primär **Dienstprogramme**: Die App ist ein Werkzeug für eine Arbeit —
# welcher Blumenkasten ist fällig, wer hat gegossen, wann ist der nächste
# Termin. Sekundär **Lifestyle**, die Schublade für Haus, Garten und
# Nachbarschaft.
#
# Bewusst **nicht** „Soziale Netze" (SOCIAL_NETWORKING): Es gibt keinen Chat,
# keine Kommentare, keinen Beitragsstrom und kein Teilen — nur ein Verzeichnis
# der angemeldeten Dorfbewohner. Die Kategorie würde etwas versprechen, was
# die App nicht kann, und die Prüfung auf Richtlinie 1.2 (Moderation von
# Nutzerinhalten) unnötig scharf stellen.
KATEGORIE_PRIMAER = "UTILITIES"
KATEGORIE_SEKUNDAER = "LIFESTYLE"

# Rechteangabe (App Information → Content Rights). **Ja, Inhalte Dritter**:
# Die Karte zeigt OpenStreetMap-Daten über OpenFreeMap
# (`Konfiguration.kartenstil`), und der Attributionsknopf von MapLibre steht
# fest in der Kartenansicht (`MapLibreKarte.swift`). Die Termine können auf
# Seiten fremder Veranstalter verweisen (`external` in `events.json`).
# Die Rechte sind vorhanden: ODbL mit sichtbarer Namensnennung.
INHALTSRECHTE = "USES_THIRD_PARTY_CONTENT"

# Version 1.0: Urheberrechtsvermerk und Freigabeart.
#
# **Manuelle Freigabe** ist Absicht. Apple gibt eine bestandene Prüfung sonst
# sofort frei — auch nachts um drei. Die App nützt aber erst, wenn im Dorf
# bekannt ist, dass es sie gibt, und wenn die Rössing-IDs verteilt sind. Der
# Dorfentwicklungskreis drückt den Knopf, wenn der Aushang hängt.
COPYRIGHT = "2026 Dorfentwicklungskreis Rössing"
FREIGABE = "MANUAL"

# Prüfangaben (App Review Information).
KONTAKT_VORNAME = "Levin"
KONTAKT_NACHNAME = "Keller"
KONTAKT_TELEFON = "+4915156041082"
PRUEFKONTO = "apple.review"
# Das Passwort steht **nicht** im Repo — gleiche Regel wie bei der Beta-Review.
PRUEFKONTO_UMGEBUNG = "PRUEFKONTO_PASSWORT"

# Verfügbarkeit: **weltweit**, und neue Länder kommen von selbst dazu.
#
# Die Absperrung ist die Rössing-ID, nicht der Ländershop: Ohne Konto kommt
# niemand über den Anmeldebildschirm, egal aus welchem Store die App kommt.
# Eine Beschränkung auf Deutschland schützt also nichts, kostet aber die
# Dorfbewohner mit einer nicht-deutschen Apple-ID (Ausland, Zugezogene) den
# Zugang zur eigenen Dorf-App. Der Basisshop bleibt Deutschland.
BASIS_LAND = "DEU"

# Der Fragebogen zur Alterseinstufung. Jede Antwort ist am Quelltext belegt;
# was sich nicht belegen ließ, steht nicht drin.
#
# Die Inhaltsfragen (NONE / INFREQUENT_OR_MILD / FREQUENT_OR_INTENSE) sind
# durchweg NONE: Die App zeigt Blumenkästen, Beete, Termine und Namen.
#
# Die Ja/Nein-Fragen im Einzelnen:
#
#   userGeneratedContent = **ja**. Die Profilnotiz, der Anzeigename und der
#     Spitzname sind freie Texte (`ProfilView.swift`), und wer sie auf
#     „öffentlich" stellt, zeigt sie allen angemeldeten Dorfbewohnern
#     (`DorfbewohnerView.swift`). Auch die Verwaltung legt Orts- und
#     Aufgabennamen als Text an. Das ist wenig, aber es ist Nutzerinhalt.
#     — Nicht mitgezählt: die Erledigungsnotiz. `DorfApi.melden` kennt zwar
#     ein Feld `notiz`, die Oberfläche schickt aber immer den leeren String
#     (`OrteModell.melden`, Zeile mit `quelle.melden(aufgabe.id, …, "")`).
#     Und die Idee aus „Idee vorschlagen" geht nur an den
#     Dorfentwicklungskreis, kein anderes Konto bekommt sie zu sehen.
#
#   unrestrictedWebAccess = **nein**. Die App hat keinen eingebauten Browser:
#     kein WKWebView, kein SFSafariViewController, nichts dergleichen im
#     ganzen Quelltext. Was sie öffnet, sind `Link`-Ziele, und die übergibt
#     iOS an Safari, wo Bildschirmzeit und Beschränkungen greifen. Geöffnet
#     werden: Impressum und Datenschutz auf rössing.de
#     (`RechtlichesLeiste.swift`), die Rössing-ID
#     (`EinstellungenView.swift`), `tel:` und `mailto:` aus dem
#     Bewohnerverzeichnis (`Profilstand.swift`), die Systemeinstellungen
#     (`KarteView.swift`) — und die Adresse eines Termins
#     (`VeranstaltungenView.swift`). Letztere kann auf eine fremde Seite
#     führen, wenn der Termin als `external` in der `events.json` der
#     Dorf-Website steht. Eine feste Liste ist das also nicht; ein
#     Surfbrett aber auch nicht: Der Sprung endet in Safari, nicht in der App.
#
#   messagingAndChat = **nein**. Es gibt keinen Chat, keine Kommentare, keine
#     Nachricht von Nutzer zu Nutzer. Die Texte der Vergabe formuliert der
#     Server aus festen Bausteinen; auf eine Anfrage antwortet man mit einem
#     Knopf, nicht mit Worten.
#
#   advertising = **nein**. Einzige Fremdbibliothek ist MapLibre
#     (`ios/project.yml`) — kein Werbe- oder Analysebaustein im Projekt.
#
#   socialMedia / socialMediaAgeRestricted = **nein**. Kein Beitragsstrom,
#     kein Folgen, kein Teilen nach außen.
#
#   healthOrWellnessTopics = **nein**. Gießen und Jäten ist Gartenarbeit,
#     keine Gesundheitsauskunft.
#
#   gambling / lootBox = **nein**. Die Rangliste zählt Erledigungen; es gibt
#     keinen Zufall, keinen Einsatz und keinen Preis. Aus demselben Grund
#     steht `contests` auf NONE.
#
#   parentalControls / ageAssurance = **nein**. Die App hat weder eine
#     Kindersicherung noch eine Altersprüfung.
#
#   kidsAgeBand bleibt leer: Die App ist nicht für die Kinderkategorie
#     gedacht (`isOrEverWasMadeForKids` steht bei Apple auf false).
ALTERSEINSTUFUNG = {
    "violenceCartoonOrFantasy": "NONE",
    "violenceRealistic": "NONE",
    "violenceRealisticProlongedGraphicOrSadistic": "NONE",
    "gunsOrOtherWeapons": "NONE",
    "profanityOrCrudeHumor": "NONE",
    "matureOrSuggestiveThemes": "NONE",
    "horrorOrFearThemes": "NONE",
    "sexualContentOrNudity": "NONE",
    "sexualContentGraphicAndNudity": "NONE",
    "alcoholTobaccoOrDrugUseOrReferences": "NONE",
    "medicalOrTreatmentInformation": "NONE",
    "gamblingSimulated": "NONE",
    "contests": "NONE",
    "gambling": False,
    "lootBox": False,
    "advertising": False,
    "messagingAndChat": False,
    "socialMedia": False,
    "socialMediaAgeRestricted": False,
    "healthOrWellnessTopics": False,
    "parentalControls": False,
    "ageAssurance": False,
    "userGeneratedContent": True,
    "unrestrictedWebAccess": False,
    "ageRatingOverride": "NONE",
    "koreaAgeRatingOverride": "NONE",
}

# Die Notiz für die App-Prüfung. Zweisprachig, weil die Prüfung nicht
# zwingend in Deutschland sitzt und der Text sonst raten lässt.
PRUEFHINWEISE = """\
Die App ist die Dorf-App des Dorfes Rössing (Gemeinde Nordstemmen, \
Niedersachsen). Betrieben wird sie vom Dorfentwicklungskreis Rössing, einer \
ehrenamtlichen Gruppe im Dorf. Keine Werbung, keine Käufe in der App, kein \
Tracking.

1. Ohne Anmeldung ist nichts zu sehen. Bitte das beigefügte Prüfkonto \
benutzen (Benutzername apple.review). Der Anmeldeknopf öffnet dafür den \
Browser.

2. Die Anmeldung läuft über den eigenen OIDC-Dienst des Dorfes, die \
„Rössing-ID“ auf id.xn--rssing-wxa.de (id.rössing.de). Diesen Dienst \
betreibt der Dorfentwicklungskreis selbst. Es gibt KEINE Anmeldung über \
einen Drittanbieter — kein Google, kein Facebook, kein anderer sozialer \
Dienst. Richtlinie 4.8 ist deshalb nicht einschlägig.

3. Der Bereich „Verwaltung“ erscheint nur bei Konten mit der Rolle „admin“. \
Das Prüfkonto hat diese Rolle nicht, der Bereich bleibt dort also \
unsichtbar. Das ist kein Fehler und nichts Verstecktes, sondern die \
Rechteverwaltung des Dorfes.

4. Das Konto lässt sich in der App löschen: Einstellungen → Konto löschen. \
Richtlinie 5.1.1 (v) ist damit erfüllt.

--- English ---

This is the village app of Rössing, a village in Lower Saxony, Germany. It \
is run by the Dorfentwicklungskreis Rössing, a volunteer group in the \
village. No ads, no in-app purchases, no tracking.

1. Nothing is visible without signing in. Please use the review account \
provided (user name apple.review). The sign-in button opens the browser.

2. Sign-in uses the village's own OIDC provider, the "Rössing-ID" at \
id.xn--rssing-wxa.de, operated by the Dorfentwicklungskreis itself. There is \
NO third-party login — no Google, no Facebook, no other social login. \
Guideline 4.8 therefore does not apply.

3. The 'Verwaltung' (administration) area only appears for accounts holding \
the role 'admin'. The review account does not hold that role, so the area \
stays hidden for you. Nothing is broken and nothing is concealed — it is the \
village's own role management.

4. The account can be deleted inside the app: Einstellungen (Settings) → \
Konto löschen (Delete account). Guideline 5.1.1 (v) is satisfied."""


def app_info() -> dict:
    """Der `appInfo`-Datensatz, an dem Kategorien und Alterseinstufung hängen.

    Apple führt für jede App mehrere davon (einen je Zustand); bearbeitbar
    ist der, der nicht schon im Store steht.
    """
    app = app_datensatz()
    infos = anfrage("GET", f"/v1/apps/{app['id']}/appInfos?limit=10").get("data", [])
    if not infos:
        raise SystemExit("Kein appInfo zur App gefunden — das darf nicht sein.")
    for eintrag in infos:
        zustand = eintrag.get("attributes", {}).get("state") \
            or eintrag.get("attributes", {}).get("appStoreState")
        if zustand != "READY_FOR_DISTRIBUTION":
            return eintrag
    return infos[0]


def bearbeitbare_version() -> dict:
    """Die Version, an der gerade gearbeitet wird (die nicht im Store steht)."""
    app = app_datensatz()
    versionen = anfrage(
        "GET", f"/v1/apps/{app['id']}/appStoreVersions?limit=10"
    ).get("data", [])
    if not versionen:
        raise SystemExit(
            "Zur App gibt es noch keine Store-Version. Eine Version legt ein "
            "Mensch in App Store Connect an — siehe store/ios-veroeffentlichung.md."
        )
    for eintrag in versionen:
        zustand = eintrag.get("attributes", {}).get("appVersionState") \
            or eintrag.get("attributes", {}).get("appStoreState")
        if zustand != "READY_FOR_DISTRIBUTION":
            return eintrag
    return versionen[0]


def kategorien() -> None:
    """Primäre und sekundäre Kategorie am `appInfo` setzen.

    Beides ist jederzeit änderbar und kostet keine neue Prüfung — die
    Begründung für genau diese zwei steht oben bei den Konstanten.
    """
    app = app_datensatz()
    info = app_info()
    anfrage("PATCH", f"/v1/appInfos/{info['id']}", {
        "data": {
            "id": info["id"],
            "type": "appInfos",
            "relationships": {
                "primaryCategory": {
                    "data": {"type": "appCategories", "id": KATEGORIE_PRIMAER}},
                "secondaryCategory": {
                    "data": {"type": "appCategories", "id": KATEGORIE_SEKUNDAER}},
            },
        },
    })
    print(f"Kategorien gesetzt: primär {KATEGORIE_PRIMAER}, "
          f"sekundär {KATEGORIE_SEKUNDAER}.")

    anfrage("PATCH", f"/v1/apps/{app['id']}", {
        "data": {
            "id": app["id"],
            "type": "apps",
            "attributes": {"contentRightsDeclaration": INHALTSRECHTE},
        },
    })
    print(f"Rechteangabe gesetzt: {INHALTSRECHTE} — wegen der "
          "OpenStreetMap-Karte und der Seiten fremder Veranstalter.")


def alterseinstufung() -> None:
    """Den Fragebogen zur Alterseinstufung beantworten.

    Die Antworten stehen oben in `ALTERSEINSTUFUNG`, jede mit der Stelle im
    Quelltext, die sie trägt. Wer hier etwas ändert, ändert eine Erklärung
    gegenüber Apple — nicht eine Einstellung.
    """
    info = app_info()
    antwort = anfrage("PATCH", f"/v1/ageRatingDeclarations/{info['id']}", {
        "data": {
            "id": info["id"],
            "type": "ageRatingDeclarations",
            "attributes": ALTERSEINSTUFUNG,
        },
    })
    print("Alterseinstufung beantwortet.")
    print("  Nutzergenerierte Inhalte: ja (Profilnotiz, Anzeigename, Spitzname)")
    print("  Unbeschränkter Web-Zugriff: nein (kein eingebauter Browser)")
    print("  Alles Übrige: keine Gewalt, kein Alkohol, kein Glücksspiel, "
          "keine Werbung, kein Chat")
    if not PROBE:
        eingestuft = anfrage("GET", f"/v1/appInfos/{info['id']}") \
            .get("data", {}).get("attributes", {})
        print(f"  Ergebnis laut Apple: {eingestuft.get('appStoreAgeRating') or 'noch offen'}")


def pruefangaben() -> None:
    """Prüfkontakt, Prüfkonto und Notiz für die App-Prüfung setzen.

    Das Passwort kommt aus der Umgebung, nicht aus dem Repo:

        export PRUEFKONTO_PASSWORT='…'
        python3 store/asc.py pruefangaben
    """
    passwort = os.environ.get(PRUEFKONTO_UMGEBUNG, "")
    if not passwort:
        raise SystemExit(
            f"Das Passwort des Prüfkontos fehlt. Es steht nicht im Repo und "
            f"gehört auch nicht hinein:\n\n"
            f"  export {PRUEFKONTO_UMGEBUNG}='…'\n"
            f"  python3 store/asc.py pruefangaben\n\n"
            f"Das Konto heißt {PRUEFKONTO}; das Passwort liegt im "
            f"Passwortspeicher.\nEs wurde nichts geändert."
        )

    version = bearbeitbare_version()
    eigenschaften = {
        "contactFirstName": KONTAKT_VORNAME,
        "contactLastName": KONTAKT_NACHNAME,
        "contactPhone": KONTAKT_TELEFON,
        "contactEmail": FEEDBACK_ADRESSE,
        "demoAccountName": PRUEFKONTO,
        "demoAccountPassword": passwort,
        "demoAccountRequired": True,
        "notes": PRUEFHINWEISE,
    }

    vorhanden = anfrage(
        "GET", f"/v1/appStoreVersions/{version['id']}/appStoreReviewDetail"
    ).get("data")

    if vorhanden:
        anfrage("PATCH", f"/v1/appStoreReviewDetails/{vorhanden['id']}", {
            "data": {"id": vorhanden["id"], "type": "appStoreReviewDetails",
                     "attributes": eigenschaften},
        })
        print(f"Prüfangaben zu Version "
              f"{version['attributes'].get('versionString')} aktualisiert.")
    else:
        anfrage("POST", "/v1/appStoreReviewDetails", {
            "data": {
                "type": "appStoreReviewDetails",
                "attributes": eigenschaften,
                "relationships": {"appStoreVersion": {
                    "data": {"type": "appStoreVersions", "id": version["id"]}}},
            },
        })
        print(f"Prüfangaben zu Version "
              f"{version['attributes'].get('versionString')} angelegt.")

    print(f"  Kontakt: {KONTAKT_VORNAME} {KONTAKT_NACHNAME}, "
          f"{FEEDBACK_ADRESSE}, {KONTAKT_TELEFON}")
    print(f"  Prüfkonto: {PRUEFKONTO} (Passwort aus {PRUEFKONTO_UMGEBUNG}), "
          f"wird verlangt: ja")
    print(f"  Notiz: {len(PRUEFHINWEISE)} Zeichen, deutsch und englisch")


def version_angaben() -> None:
    """Urheberrechtsvermerk und Freigabeart der Version setzen."""
    version = bearbeitbare_version()
    anfrage("PATCH", f"/v1/appStoreVersions/{version['id']}", {
        "data": {
            "id": version["id"],
            "type": "appStoreVersions",
            "attributes": {
                "copyright": COPYRIGHT,
                "releaseType": FREIGABE,
                # Kein Werbebaustein im Projekt (einzige Fremdbibliothek ist
                # MapLibre), also auch keine Werbekennung.
                "usesIdfa": False,
            },
        },
    })
    print(f"Version {version['attributes'].get('versionString')}: "
          f"Copyright „{COPYRIGHT}“, Freigabe {FREIGABE}, keine Werbekennung.")
    if FREIGABE == "MANUAL":
        print("  Manuell heißt: Nach bestandener Prüfung passiert erst einmal "
              "nichts.\n  Der Dorfentwicklungskreis gibt frei, wenn das Dorf "
              "Bescheid weiß.")


def verfuegbarkeit() -> None:
    """Kostenlos und weltweit — Preisplan und Länderliste anlegen.

    Beides fehlt bei einer frisch angelegten App und beides blockiert die
    Einreichung. Der Preisplan braucht einen „Preispunkt" für 0,00 €; die
    Länderliste bekommt alles, was Apple gerade führt.
    """
    app = app_datensatz()

    # --- Preis: kostenlos ---------------------------------------------------
    # Apple legt zu jeder App von sich aus einen leeren Preisplan an (Basisland
    # USA, keine Preise). Ein POST ersetzt ihn vollständig — deshalb wird hier
    # nicht erst gefragt, sondern geschrieben.
    punkte = anfrage(
        "GET",
        f"/v1/apps/{app['id']}/appPricePoints?filter[territory]={BASIS_LAND}&limit=200",
    ).get("data", [])
    kostenlos = next(
        (p for p in punkte
         if float(p.get("attributes", {}).get("customerPrice", "1")) == 0.0),
        None,
    )
    if kostenlos is None:
        raise SystemExit(
            f"Kein Preispunkt für 0,00 in {BASIS_LAND} gefunden — das darf "
            "nicht sein. Es wurde nichts geändert."
        )

    anfrage("POST", "/v1/appPriceSchedules", {
        "data": {
            "type": "appPriceSchedules",
            "relationships": {
                "app": {"data": {"type": "apps", "id": app["id"]}},
                "baseTerritory": {
                    "data": {"type": "territories", "id": BASIS_LAND}},
                # `${…}` ist Apples Schreibweise für „der Datensatz, der
                # weiter unten unter `included` steht" — ein Preis und sein
                # Preispunkt entstehen in einer einzigen Anfrage.
                "manualPrices": {"data": [
                    {"type": "appPrices", "id": "${kostenlos}"}]},
            },
        },
        "included": [{
            "type": "appPrices",
            "id": "${kostenlos}",
            # Ohne Start- und Enddatum: gilt ab sofort und ohne Ablauf.
            "relationships": {"appPricePoint": {
                "data": {"type": "appPricePoints", "id": kostenlos["id"]}}},
        }],
    })
    print(f"Preis: kostenlos, Basisland {BASIS_LAND}.")

    # --- Länder: weltweit ---------------------------------------------------
    laender = [e["id"] for e in
               anfrage("GET", "/v1/territories?limit=200").get("data", [])]
    if not laender:
        raise SystemExit("Apple gibt keine Länderliste zurück — Abbruch.")

    # Jedes Land steht zweimal in der Anfrage: einmal als Verweis unter
    # `relationships` und einmal als eigener Datensatz unter `included`. So
    # will es Apple — die Beziehung allein reicht nicht.
    anfrage("POST", "/v2/appAvailabilities", {
        "data": {
            "type": "appAvailabilities",
            "attributes": {"availableInNewTerritories": True},
            "relationships": {
                "app": {"data": {"type": "apps", "id": app["id"]}},
                "territoryAvailabilities": {"data": [
                    {"type": "territoryAvailabilities", "id": "${%s}" % land}
                    for land in laender
                ]},
            },
        },
        "included": [
            {"type": "territoryAvailabilities", "id": "${%s}" % land,
             "relationships": {"territory": {
                 "data": {"type": "territories", "id": land}}}}
            for land in laender
        ],
    })
    print(f"Verfügbarkeit: {len(laender)} Länder — alles, was Apple führt; "
          "neue Länder kommen von selbst dazu.")
    print("  Grund: Die Absperrung ist die Rössing-ID, nicht der Ländershop. "
          "Wer\n  im Ausland lebt und trotzdem zum Dorf gehört, soll die App "
          "laden können.")


def einreichstand() -> None:
    """Ehrliche Bestandsaufnahme: Was fehlt zur Einreichung noch?

    Fragt nur — ändert nichts. Reihenfolge wie in App Store Connect, damit
    man die Liste von oben nach unten abarbeiten kann.
    """
    app = app_datensatz()
    info = app_info()
    version = bearbeitbare_version()
    offen: list[str] = []

    def pruefe(bedingung: bool, fehlt: str) -> None:
        if not bedingung:
            offen.append(fehlt)

    merkmale = app.get("attributes", {})
    print(f"App {merkmale.get('name')} ({merkmale.get('bundleId')}), "
          f"Version {version['attributes'].get('versionString')} — "
          f"{version['attributes'].get('appVersionState')}")
    print()

    # Kategorien und Rechteangabe. Ohne `include` liefert Apple zu den
    # Kategorien nur Verweise, keine Kennungen — dann sähe alles leer aus.
    voll = anfrage("GET", f"/v1/appInfos/{info['id']}"
                          "?include=primaryCategory,secondaryCategory")
    beziehungen = voll.get("data", {}).get("relationships", {})
    primaer = (beziehungen.get("primaryCategory", {}).get("data") or {}).get("id")
    sekundaer = (beziehungen.get("secondaryCategory", {}).get("data") or {}).get("id")
    print(f"Kategorie primär:      {primaer or '— fehlt'}")
    print(f"Kategorie sekundär:    {sekundaer or '— fehlt (freiwillig)'}")
    pruefe(bool(primaer), "primäre Kategorie (`asc.py kategorien`)")
    print(f"Rechteangabe:          {merkmale.get('contentRightsDeclaration') or '— fehlt'}")
    pruefe(bool(merkmale.get("contentRightsDeclaration")),
           "Rechteangabe (`asc.py kategorien`)")

    # Alterseinstufung
    einstufung = anfrage("GET", f"/v1/appInfos/{info['id']}/ageRatingDeclaration") \
        .get("data", {}).get("attributes", {}) or {}
    unbeantwortet = [name for name, wert in einstufung.items()
                     if wert is None and name not in
                     ("kidsAgeBand", "developerAgeRatingInfoUrl")]
    ergebnis = voll.get("data", {}).get("attributes", {}).get("appStoreAgeRating")
    print(f"Alterseinstufung:      {ergebnis or '— offen'}"
          + (f" ({len(unbeantwortet)} Fragen offen)" if unbeantwortet else ""))
    pruefe(not unbeantwortet, "Alterseinstufung (`asc.py alterseinstufung`)")

    # Texte je Sprache
    sprachen = anfrage(
        "GET", f"/v1/appStoreVersions/{version['id']}/appStoreVersionLocalizations?limit=50"
    ).get("data", [])
    print(f"Store-Texte:           {len(sprachen)} Sprache(n): "
          + ", ".join(sorted(s["attributes"]["locale"] for s in sprachen)))
    pruefe(bool(sprachen), "Store-Texte (`asc.py store-texte`, falls vorhanden)")

    # Screenshots — je Sprache mindestens ein Satz
    ohne_bilder = []
    for sprache in sprachen:
        saetze = anfrage(
            "GET",
            f"/v1/appStoreVersionLocalizations/{sprache['id']}/appScreenshotSets?limit=50",
        ).get("data", [])
        wieviele = 0
        for satz in saetze:
            wieviele += len(anfrage(
                "GET", f"/v1/appScreenshotSets/{satz['id']}/appScreenshots?limit=10"
            ).get("data", []))
        print(f"  Screenshots {sprache['attributes']['locale']}: "
              f"{wieviele} in {len(saetze)} Satz/Sätzen")
        if wieviele == 0:
            ohne_bilder.append(sprache["attributes"]["locale"])
    pruefe(not ohne_bilder,
           "Screenshots für " + ", ".join(ohne_bilder) + " (macht ein anderer)")

    # Prüfangaben
    detail = anfrage(
        "GET", f"/v1/appStoreVersions/{version['id']}/appStoreReviewDetail"
    ).get("data")
    d = (detail or {}).get("attributes", {})
    print(f"Prüfangaben:           {'da' if detail else '— fehlen'}"
          + (f", Prüfkonto {d.get('demoAccountName')}" if detail else ""))
    pruefe(bool(detail), "Prüfangaben (`asc.py pruefangaben`)")

    # Version selbst
    v = version.get("attributes", {})
    print(f"Copyright:             {v.get('copyright') or '— fehlt'}")
    pruefe(bool(v.get("copyright")), "Copyright (`asc.py version-angaben`)")
    print(f"Freigabe:              {v.get('releaseType')}")

    # Build
    build = anfrage("GET", f"/v1/appStoreVersions/{version['id']}/build").get("data")
    if build:
        voller_build = anfrage(
            "GET", f"/v1/builds/{build['id']}?include=preReleaseVersion")
        b = voller_build.get("data", {}).get("attributes", {})
        vorab = next((i["attributes"].get("version")
                      for i in voller_build.get("included", [])
                      if i["type"] == "preReleaseVersions"), "?")
        print(f"Build:                 Nr. {b.get('version')} "
              f"({b.get('processingState')}), Marketing-Version {vorab}")
        # Die Zahl im Binärpaket und die Zahl im Store-Eintrag sollten
        # dieselbe sein — sonst steht im Store etwas anderes als in der App.
        if vorab != v.get("versionString"):
            print(f"  ⚠ Der Build sagt {vorab}, der Store-Eintrag sagt "
                  f"{v.get('versionString')}.")
            offen.append(
                f"Versionsnummern zusammenbringen: Build {vorab} gegen "
                f"Store-Eintrag {v.get('versionString')}")
    else:
        print("Build:                 — der Version ist keiner zugeordnet")
    pruefe(bool(build), "Build der Version zuordnen")

    # Preis und Länder
    preis = anfrage("GET", f"/v1/apps/{app['id']}/appPriceSchedule", dulden=(404,))
    hat_preis = bool(preis.get("data"))
    print(f"Preisplan:             {'da' if hat_preis else '— fehlt'}")
    pruefe(hat_preis, "Preis (`asc.py verfuegbarkeit`)")

    verfuegbar = anfrage("GET", f"/v2/appAvailabilities/{app['id']}", dulden=(404,))
    hat_laender = bool(verfuegbar.get("data"))
    print(f"Verfügbarkeit:         {'da' if hat_laender else '— fehlt'}")
    pruefe(hat_laender, "Länder (`asc.py verfuegbarkeit`)")

    # Zwei Pflichtangaben kann dieses Skript nicht nachsehen, weil Apple sie
    # gar nicht über die API führt. Sie fehlen zu behaupten wäre geraten —
    # sie zu verschweigen wäre schlimmer. Also stehen sie hier als Auftrag.
    offen.append(
        "App Privacy von Hand ausfüllen (keine API: `appDataUsages` & Co. "
        "gibt es nicht) — Vorlage: store/ios-datenschutz.md")
    offen.append(
        "Händlerstatus (EU) in App Store Connect nachsehen — Business → "
        "App Information; über die API nicht abfragbar")

    print()
    if offen:
        print("Es fehlt noch:")
        for nummer, satz in enumerate(offen, 1):
            print(f"  {nummer}. {satz}")
    else:
        print("Nichts Offenes gefunden — was Apple beim Einreichen zusätzlich "
              "verlangt,\nsagt erst die Einreichung selbst.")
    print("\nEingereicht wird hier bewusst nicht: Das ist der einzige Schritt, "
          "den ein\nMensch tun muss — App Store Connect → „Add for Review“.")
# Store-Bilder hochladen
# ---------------------------------------------------------------------------


def store_version(vorgabe: str | None = None) -> str:
    """Die Store-Version, an der die Bilder hängen sollen.

    Ohne Angabe wird die neueste iOS-Version genommen, sofern sie sich noch
    bearbeiten lässt. Das ist Absicht: Bilder an eine Version zu hängen, die
    gerade in der Prüfung liegt, geht nicht — und ein stiller Fehlschlag wäre
    hier besonders ärgerlich, weil man ihn erst in App Store Connect merkt.
    """
    if vorgabe:
        return vorgabe
    app = app_datensatz()
    versionen = anfrage(
        "GET", f"/v1/apps/{app['id']}/appStoreVersions?limit=10&filter[platform]=IOS"
    ).get("data", [])
    if not versionen:
        raise SystemExit("Zu dieser App gibt es noch keine Store-Version.")
    eintrag = versionen[0]
    a = eintrag.get("attributes", {})
    zustand = a.get("appVersionState") or a.get("appStoreState") or "?"
    if zustand not in BEARBEITBAR:
        raise SystemExit(
            f"Version {a.get('versionString', '?')} steht auf {zustand} — in diesem "
            "Zustand nimmt Apple keine neuen Bilder an.\n"
            "Es wurde nichts geändert."
        )
    return eintrag["id"]


def _lokalisierungen(version: str) -> dict[str, str]:
    antwort = anfrage("GET", f"/v1/appStoreVersions/{version}/appStoreVersionLocalizations?limit=50")
    return {e["attributes"]["locale"]: e["id"] for e in antwort.get("data", [])}


def _bildsatz(lokalisierung: str, anzeigetyp: str) -> str:
    """Den Bildsatz je Sprache und Anzeigetyp holen — oder anlegen.

    Ein Satz ist der Kasten, in dem die Bilder einer Gerätegröße liegen. Es
    gibt genau einen je Kombination; ein zweiter wäre ein Fehler, deshalb wird
    zuerst gesucht.
    """
    vorhandene = anfrage(
        "GET",
        f"/v1/appStoreVersionLocalizations/{lokalisierung}/appScreenshotSets?limit=50",
    ).get("data", [])
    for satz in vorhandene:
        if satz.get("attributes", {}).get("screenshotDisplayType") == anzeigetyp:
            return satz["id"]

    antwort = anfrage("POST", "/v1/appScreenshotSets", {
        "data": {
            "type": "appScreenshotSets",
            "attributes": {"screenshotDisplayType": anzeigetyp},
            "relationships": {
                "appStoreVersionLocalization": {
                    "data": {"type": "appStoreVersionLocalizations", "id": lokalisierung}
                }
            },
        },
    })
    return antwort["data"]["id"]


def _satz_leeren(satz: str) -> int:
    """Alle Bilder eines Satzes löschen.

    Hochladen ersetzt nicht, es hängt an. Ohne dieses Aufräumen stünden nach
    dem zweiten Lauf vierzehn Bilder im Store — und Apple nimmt höchstens
    zehn je Satz.
    """
    if PROBE and satz.startswith("("):
        # Der Satz wurde im Trockenlauf nur *gedacht* — es gibt nichts zu leeren.
        return 0
    vorhandene = anfrage("GET", f"/v1/appScreenshotSets/{satz}/appScreenshots?limit=50")
    if vorhandene.get("data") is None:
        return 0
    for bild in vorhandene["data"]:
        anfrage("DELETE", f"/v1/appScreenshots/{bild['id']}")
    return len(vorhandene["data"])


def _hochschieben(bild: dict, inhalt: bytes) -> None:
    """Die Teile einer Datei an die von Apple genannten Adressen schicken.

    Apple zerlegt große Dateien in mehrere Abschnitte und nennt für jeden
    Adresse, Methode, Offset, Länge und die zu setzenden Kopfzeilen. Genau die
    sind zu nehmen — die Adressen sind kurzlebig signiert.
    """
    for teil in bild["attributes"].get("uploadOperations", []):
        stueck = inhalt[teil["offset"]:teil["offset"] + teil["length"]]
        req = urllib.request.Request(teil["url"], data=stueck,
                                     method=teil.get("method", "PUT"))
        for kopf in teil.get("requestHeaders", []):
            req.add_header(kopf["name"], kopf["value"])
        try:
            with urllib.request.urlopen(req, timeout=120) as antwort:
                antwort.read()
        except urllib.error.HTTPError as fehler:
            text = fehler.read().decode(errors="replace")
            raise SystemExit(
                f"Teil-Upload fehlgeschlagen (HTTP {fehler.code}): {text}\n"
                f"Das angelegte Bild {bild['id']} bleibt unfertig zurück und sollte "
                "gelöscht werden — ein erneuter Lauf räumt den Satz ohnehin leer."
            )


def _bild_hochladen(satz: str, pfad: Path) -> str:
    inhalt = pfad.read_bytes()
    reservierung = anfrage("POST", "/v1/appScreenshots", {
        "data": {
            "type": "appScreenshots",
            "attributes": {"fileSize": len(inhalt), "fileName": pfad.name},
            "relationships": {
                "appScreenshotSet": {"data": {"type": "appScreenshotSets", "id": satz}}
            },
        },
    })
    bild = reservierung["data"]
    if PROBE:
        print(f"    [Trockenlauf] {pfad.name} ({len(inhalt) // 1024} KiB) würde in "
              f"{len(bild['attributes'].get('uploadOperations', []))} Teilen hochgehen.")
        return bild["id"]

    _hochschieben(bild, inhalt)
    # Erst mit `uploaded` gilt die Datei als vollständig. Die Prüfsumme ist
    # der Beleg, dass angekommen ist, was losgeschickt wurde.
    anfrage("PATCH", f"/v1/appScreenshots/{bild['id']}", {
        "data": {
            "id": bild["id"], "type": "appScreenshots",
            "attributes": {"uploaded": True,
                           "sourceFileChecksum": hashlib.md5(inhalt).hexdigest()},
        },
    })
    return bild["id"]


def _reihenfolge_setzen(satz: str, kennungen: list[str]) -> None:
    """Die Reihenfolge im Satz festlegen.

    Apple zeigt die ersten drei Bilder in den Suchergebnissen — die
    Reihenfolge ist deshalb kein Schönheitsfehler, sondern der Verkaufstext.
    """
    anfrage("PATCH", f"/v1/appScreenshotSets/{satz}/relationships/appScreenshots", {
        "data": [{"type": "appScreenshots", "id": k} for k in kennungen],
    })


def screenshots_hochladen(version: str | None = None) -> None:
    """Die Bilder aus `store/screenshots/ios/` an die Store-Version hängen.

    Je Sprache und Gerätegröße wird der Bildsatz gesucht (oder angelegt),
    geleert und neu gefüllt. Neu füllen statt ergänzen: Sonst wächst der Satz
    bei jedem Lauf, und welche zehn Apple dann zeigt, bestimmt der Zufall.

    Aufgenommen und abgelegt werden die Bilder mit
    `store/screenshots/aufnehmen.sh` — siehe `store/screenshots/README.md`.
    """
    zugang()
    if not BILDER.is_dir():
        raise SystemExit(
            f"{BILDER} gibt es nicht. Die Bilder entstehen mit\n"
            "  store/screenshots/aufnehmen.sh <simulator-udid> <geraet>\n"
            "Es wurde nichts geändert."
        )

    version = store_version(version or STORE_VERSION)
    lokalisierungen = _lokalisierungen(version)
    print(f"Store-Version {version}")

    for sprache in SPRACHEN:
        if sprache not in lokalisierungen:
            raise SystemExit(
                f"Für {sprache} gibt es keine Lokalisierung an dieser Version — "
                "erst die Texte anlegen, dann die Bilder."
            )
        for ordner, anzeigetyp in ANZEIGETYPEN.items():
            quelle = BILDER / sprache / ordner
            dateien = sorted(quelle.glob("*.png"))
            if not dateien:
                print(f"{sprache}/{ordner}: keine Bilder — übersprungen.")
                continue
            if len(dateien) > 10:
                raise SystemExit(
                    f"{sprache}/{ordner}: {len(dateien)} Bilder — Apple nimmt "
                    "höchstens zehn je Anzeigetyp."
                )

            satz = _bildsatz(lokalisierungen[sprache], anzeigetyp)
            geloescht = _satz_leeren(satz)
            print(f"{sprache}/{ordner} ({anzeigetyp}): Satz {satz}"
                  + (f", {geloescht} alte Bilder entfernt" if geloescht else ""))

            kennungen = []
            for datei in dateien:
                kennungen.append(_bild_hochladen(satz, datei))
                print(f"  {datei.name}")
            if not PROBE:
                _reihenfolge_setzen(satz, kennungen)
            print(f"  {len(dateien)} Bilder, Reihenfolge gesetzt.")

    print("\nWas hier NICHT passiert: die Version zur Prüfung einreichen. "
          "Das bleibt ein bewusster Klick in App Store Connect.")


BEFEHLE = {
    "team-id": lambda: print(team_id()),
    "bundle-id-anlegen": bundle_id_anlegen,
    "app-zeigen": app_zeigen,
    "testflight-gruppe": testflight_gruppe,
    "beta-info": beta_info,
    "kategorien": kategorien,
    "alterseinstufung": alterseinstufung,
    "pruefangaben": pruefangaben,
    "version-angaben": version_angaben,
    "verfuegbarkeit": verfuegbarkeit,
    "einreichstand": einreichstand,
    "screenshots-hochladen": screenshots_hochladen,
}


def main() -> None:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("methode",
                   help="GET/POST/PATCH/DELETE — oder einer der Unterbefehle: "
                        + ", ".join(BEFEHLE))
    p.add_argument("pfad", nargs="?", default="", help="z.B. /v1/bundleIds")
    p.add_argument("--daten", help="JSON-Rumpf für POST/PATCH")
    p.add_argument("--key-id", dest="key_id")
    p.add_argument("--issuer-id", dest="issuer_id")
    p.add_argument("--store-version", dest="store_version",
                   help="Kennung der Store-Version für screenshots-hochladen "
                        "(ohne Angabe: die neueste bearbeitbare)")
    p.add_argument("--probe", action="store_true",
                   help="Trockenlauf: schreibende Anfragen nur zeigen, nicht schicken")
    a = p.parse_args()

    global PROBE, STORE_VERSION
    PROBE = a.probe
    STORE_VERSION = a.store_version

    # Kennungen von der Kommandozeile gelten auch für die Unterbefehle.
    if a.key_id:
        os.environ["APP_STORE_CONNECT_KEY_ID"] = a.key_id
    if a.issuer_id:
        os.environ["APP_STORE_CONNECT_ISSUER_ID"] = a.issuer_id

    if a.methode in BEFEHLE:
        BEFEHLE[a.methode]()
        if PROBE:
            print("\nTrockenlauf — es wurde nichts geändert.")
        return
    if not a.pfad:
        p.error("Pfad fehlt")

    antwort = anfrage(a.methode.upper(), a.pfad,
                      json.loads(a.daten) if a.daten else None,
                      a.key_id, a.issuer_id)
    print(json.dumps(antwort, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
