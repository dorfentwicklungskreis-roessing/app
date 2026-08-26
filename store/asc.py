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
    python3 store/asc.py team-id          # liest die Team-ID aus einem Zertifikat
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

BASIS = "https://api.appstoreconnect.apple.com"
ZIELGRUPPE = "appstoreconnect-v1"
# Apple weist Tokens ab, die länger als 20 Minuten gelten.
GUELTIGKEIT = 20 * 60


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


def token(key_id: str, issuer_id: str) -> str:
    pfad = schluesselpfad(key_id)
    if not pfad.exists():
        raise SystemExit(
            f"Privater Schlüssel nicht gefunden: {pfad}\n"
            "Die .p8-Datei aus App Store Connect dorthin legen "
            "(chmod 600) — sie gehört nicht ins Repo."
        )

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
            key_id: str | None = None, issuer_id: str | None = None) -> dict:
    key_id = key_id or os.environ.get("APP_STORE_CONNECT_KEY_ID", "")
    issuer_id = issuer_id or os.environ.get("APP_STORE_CONNECT_ISSUER_ID", "")
    if not key_id or not issuer_id:
        raise SystemExit(
            "APP_STORE_CONNECT_KEY_ID und APP_STORE_CONNECT_ISSUER_ID setzen "
            "(oder --key-id/--issuer-id angeben)."
        )

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


def main() -> None:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("methode", help="GET/POST/PATCH/DELETE — oder 'team-id'")
    p.add_argument("pfad", nargs="?", default="", help="z.B. /v1/bundleIds")
    p.add_argument("--daten", help="JSON-Rumpf für POST/PATCH")
    p.add_argument("--key-id", dest="key_id")
    p.add_argument("--issuer-id", dest="issuer_id")
    a = p.parse_args()

    if a.methode == "team-id":
        print(team_id())
        return
    if not a.pfad:
        p.error("Pfad fehlt")

    antwort = anfrage(a.methode.upper(), a.pfad,
                      json.loads(a.daten) if a.daten else None,
                      a.key_id, a.issuer_id)
    print(json.dumps(antwort, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
