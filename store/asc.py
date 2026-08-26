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

Was hier bewusst NICHT geht: den App-Datensatz selbst anlegen. Dafür gibt es
keine API — das macht ein Mensch einmalig in App Store Connect
(`store/ios-veroeffentlichung.md`, Schritt 3).

Ohne Schlüssel bricht kein Unterbefehl mit einem Stacktrace ab, sondern sagt,
was fehlt und wie man es hinlegt.
"""

from __future__ import annotations

import argparse
import base64
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
            for zeile in json.dumps(daten, indent=2, ensure_ascii=False).splitlines():
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


BEFEHLE = {
    "team-id": lambda: print(team_id()),
    "bundle-id-anlegen": bundle_id_anlegen,
    "app-zeigen": app_zeigen,
    "testflight-gruppe": testflight_gruppe,
    "beta-info": beta_info,
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
    p.add_argument("--probe", action="store_true",
                   help="Trockenlauf: schreibende Anfragen nur zeigen, nicht schicken")
    a = p.parse_args()

    global PROBE
    PROBE = a.probe

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
