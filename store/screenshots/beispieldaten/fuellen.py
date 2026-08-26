#!/usr/bin/env python3
"""Füllt ein **lokales** Dorf-App-Backend mit Beispieldaten für die Store-Bilder.

Warum es das gibt: `SEED=1` legt genau zwei Blumenkästen „Unter den Eichen"
an — richtig für einen Testlauf, zu wenig für einen App-Store-Screenshot. Eine
Karte mit zwei Nadeln und eine leere Rangliste verkaufen nichts.

**Alle Namen und Kontaktdaten hier sind erfunden.** Auf den Bildern im App
Store darf kein echter Dorfbewohner stehen. Deshalb Vornamen mit
Anfangsbuchstaben (Anna B., Bernd K. …), Rufnummern aus dem von der
Bundesnetzagentur für Film und Funk reservierten Bereich (0171 39x xxxx) und
Adressen unter `example.org` (RFC 2606).

Läuft ausschließlich gegen `http://127.0.0.1:…` und ausschließlich gegen ein
Backend im `AUTH_MODE=insecure-dev`: Dort ist das Bearer-Token schlicht
`sub:name:rollen:mail` (siehe `backend/internal/auth`, `InsecureDevVerifier`).
Gegen die Produktion ist das wirkungslos — dort prüft Zitadel.

Aufruf:

    python3 store/screenshots/beispieldaten/fuellen.py --basis http://127.0.0.1:8080

Zweimal aufrufen schadet nichts: Orte werden am Namen wiedererkannt, ihre
Aufgaben und Erledigungen vorher weggeräumt.
"""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

# --- Die Leute -------------------------------------------------------------
#
# (Kennung, Name in der Rössing-ID, Anzeigename im Dorf). Die Kennung
# `e2e-user` mit dem Namen „E2E Tester" ist die, mit der sich der
# Entwickler-Login der App anmeldet (ios/Dorf/Anmeldung/Anmeldung.swift) —
# diese Person ist auf den Bildern „ich".
LEUTE = [
    ("e2e-user", "E2E Tester", "Lena M."),
    ("beispiel-anna", "Anna", "Anna B."),
    ("beispiel-bernd", "Bernd", "Bernd K."),
    ("beispiel-clara", "Clara", "Clara W."),
    ("beispiel-dieter", "Dieter", "Dieter H."),
    ("beispiel-emma", "Emma", "Emma S."),
    ("beispiel-frank", "Frank", "Frank T."),
    ("beispiel-greta", "Greta", "Greta L."),
]

# Das Profil der Person, die auf den Bildern angemeldet ist. Absichtlich
# gemischte Schalter: So zeigt „Mein Profil", dass jedes Feld einzeln
# freigegeben wird.
MEIN_PROFIL = {
    "displayName": "Lena M.",
    "nickname": "Lena",
    "phone": "0171 3900123",
    "email": "lena@example.org",
    "note": "Am besten abends erreichbar.",
    "visibility": {
        "displayName": "dorf",
        "nickname": "dorf",
        "phone": "verwaltung",
        "email": "dorf",
        "note": "dorf",
    },
}

# Kurze Notizen der anderen — sie stehen in der Dorfbewohner-Liste und in der
# Historie eines Ortes.
PROFILE_ANDERE = {
    "beispiel-anna": {"note": "Gießt gern die Runde am Dorfplatz."},
    "beispiel-bernd": {"note": "Werkzeug im Schuppen, einfach fragen."},
    "beispiel-clara": {"note": "Am Wochenende meist da."},
}

# Der Hitzefaktor: kleiner als 1 heißt heiß, dann werden Gieß-Aufgaben
# schneller fällig — und die Startseite sagt „Heiß — bitte großzügig gießen."
HITZEFAKTOR = 0.7

# --- Die Orte --------------------------------------------------------------
#
# Rössing liegt um 52,196 / 9,815. Die Punkte sind so gestreut, dass alle
# zusammen einen hübschen Kartenausschnitt ergeben.
#
# Je Aufgabe steht in `meldungen` eine Liste (Alter in Tagen, Person). Die
# **letzte** Meldung bestimmt die Ampel: Fällig wird eine Aufgabe nach
# `intervall` Tagen, rot nach `rot` Tagen — beim Gießen zusätzlich mit dem
# Hitzefaktor beschleunigt. Weiter als 14 Tage zurück nimmt das Backend keine
# Meldung an (model.MaxBackdate).
ORTE = [
    {
        "name": "Blumenkasten am Dorfplatz",
        "beschreibung": "Der große Kasten neben der Bank am Dorfplatz.",
        "art": "blumenkasten", "lat": 52.1962, "lon": 9.8152,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 10, "intervall": 3, "rot": 7,
             "meldungen": [(13, "beispiel-anna"), (11.5, "beispiel-bernd"),
                           (10, "beispiel-clara"), (8, "beispiel-anna")]},
            {"art": "jaeten", "titel": "Beikraut zupfen", "intervall": 7, "rot": 14,
             "meldungen": [(13, "beispiel-dieter"), (8, "beispiel-anna")]},
        ],
    },
    {
        "name": "Staudenbeet am Kirchplatz",
        "beschreibung": "Das lange Beet vor der Kirchenmauer.",
        "art": "beet", "lat": 52.1955, "lon": 9.8135,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 15, "intervall": 3, "rot": 7,
             "meldungen": [(13, "beispiel-anna"), (11, "beispiel-emma"),
                           (9, "beispiel-bernd"), (6, "e2e-user"), (3, "beispiel-clara")]},
            {"art": "jaeten", "titel": "", "intervall": 14, "rot": 28,
             "meldungen": [(10, "beispiel-clara"), (2, "beispiel-anna")]},
        ],
    },
    {
        "name": "Blumenkästen Bahnhofstraße",
        "beschreibung": "Die drei Kästen am Geländer zur Bahnhofstraße.",
        "art": "blumenkasten", "lat": 52.1978, "lon": 9.8168,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 10, "intervall": 3, "rot": 7,
             "meldungen": [(13, "beispiel-bernd"), (11, "beispiel-anna"),
                           (7, "beispiel-dieter"), (4, "e2e-user"), (1, "beispiel-anna")]},
        ],
    },
    {
        "name": "Rabatte am Sportplatz",
        "beschreibung": "Vor dem Vereinsheim, links vom Eingang.",
        "art": "beet", "lat": 52.1941, "lon": 9.8178,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 20, "intervall": 3, "rot": 7,
             "meldungen": [(12, "beispiel-emma"), (8, "e2e-user"), (3, "beispiel-bernd")]},
        ],
    },
    {
        "name": "Beet am Ehrenmal",
        "beschreibung": "Rund um den Gedenkstein.",
        "art": "beet", "lat": 52.1971, "lon": 9.8188,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 10, "intervall": 3, "rot": 7,
             "meldungen": [(13, "beispiel-frank"), (10, "beispiel-anna")]},
        ],
    },
    {
        "name": "Blumenkasten am Feuerwehrhaus",
        "beschreibung": "Am Tor, rechts neben der Einfahrt.",
        "art": "blumenkasten", "lat": 52.1985, "lon": 9.8142,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 10, "intervall": 3, "rot": 7,
             "meldungen": [(12, "beispiel-clara"), (9, "beispiel-bernd"),
                           (5, "beispiel-greta"), (1, "beispiel-anna")]},
        ],
    },
    {
        "name": "Hochbeet am Kindergarten",
        "beschreibung": "Das Hochbeet der Kindergruppe im Hof.",
        "art": "beet", "lat": 52.1950, "lon": 9.8162,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 8, "intervall": 3, "rot": 7,
             "meldungen": [(10, "beispiel-greta"), (6, "e2e-user"),
                           (2, "beispiel-clara"), (0.5, "beispiel-bernd")]},
        ],
    },
    {
        "name": "Beet am Mühlenweg",
        "beschreibung": "An der Ecke zum Feldweg.",
        "art": "beet", "lat": 52.1938, "lon": 9.8128,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 12, "intervall": 3, "rot": 7,
             "meldungen": [(11, "beispiel-dieter"), (6, "beispiel-emma"), (3, "e2e-user")]},
            {"art": "jaeten", "titel": "", "intervall": 4, "rot": 8,
             "meldungen": [(13, "beispiel-greta")]},
        ],
    },
    {
        "name": "Blumenkasten an der Grundschule",
        "beschreibung": "Unter dem Fenster der ersten Klasse.",
        "art": "blumenkasten", "lat": 52.1968, "lon": 9.8118,
        "aufgaben": [
            {"art": "giessen", "titel": "", "liter": 10, "intervall": 3, "rot": 7,
             "meldungen": [(12, "e2e-user"), (8, "beispiel-frank"),
                           (4, "beispiel-clara"), (1, "beispiel-emma")]},
        ],
    },
    {
        "name": "Grünstreifen am Bahnübergang",
        "beschreibung": "Der Streifen zwischen Weg und Gleis.",
        "art": "sonstiges", "lat": 52.1990, "lon": 9.8175,
        "aufgaben": [
            {"art": "sonstiges", "titel": "Laub harken", "intervall": 30, "rot": 45,
             "meldungen": [(5, "beispiel-frank")]},
        ],
    },
]

NAME_ZU_TOKEN = {sub: f"{sub}:{name}:admin" for sub, name, _ in LEUTE}
ANZEIGENAME = {sub: anzeige for sub, _, anzeige in LEUTE}


def ruf(basis: str, methode: str, pfad: str, token: str, daten=None, dulden=()):
    rumpf = json.dumps(daten).encode() if daten is not None else None
    req = urllib.request.Request(basis + pfad, data=rumpf, method=methode)
    req.add_header("Authorization", f"Bearer {token}")
    if rumpf is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=20) as antwort:
            roh = antwort.read()
            return json.loads(roh) if roh else {}
    except urllib.error.HTTPError as fehler:
        if fehler.code in dulden:
            return None
        text = fehler.read().decode(errors="replace")
        raise SystemExit(f"HTTP {fehler.code} bei {methode} {pfad}: {text}")


def hauptteil(basis: str) -> None:
    if not basis.startswith("http://127.0.0.1") and not basis.startswith("http://localhost"):
        raise SystemExit(
            f"„{basis}“ ist keine lokale Adresse. Dieses Skript füllt "
            "ausschließlich ein Backend auf dem eigenen Rechner — die Beispieldaten "
            "haben in keiner echten Datenbank etwas verloren."
        )

    verwaltung = "beispiel-verwaltung:Verwaltung:admin"
    jetzt = datetime.now(timezone.utc)

    # 1. Profile anlegen. GET /me legt beim ersten Mal eines an; erst danach
    #    lässt sich der Anzeigename setzen, unter dem die Person in Rangliste
    #    und Historie auftaucht.
    for sub, _, anzeige in LEUTE:
        token = NAME_ZU_TOKEN[sub]
        ruf(basis, "GET", "/api/v1/me", token)
        if sub == "e2e-user":
            eingabe = dict(MEIN_PROFIL)
        else:
            eingabe = {"displayName": anzeige, "nickname": "", "phone": "",
                       "email": "", "note": ""}
            eingabe.update(PROFILE_ANDERE.get(sub, {}))
        ruf(basis, "PUT", "/api/v1/me/profile", token, eingabe)
    print(f"{len(LEUTE)} Profile gesetzt.")

    # 2. Hitzefaktor — davon hängt der Hinweis auf der Startseite ab.
    ruf(basis, "PUT", "/api/v1/settings", verwaltung, {"wateringFactor": HITZEFAKTOR})
    print(f"Hitzefaktor auf {HITZEFAKTOR} gesetzt.")

    # 3. Orte. Was es schon gibt, wird am Namen erkannt und mitsamt Aufgaben
    #    neu aufgebaut — so ist ein zweiter Lauf harmlos.
    gewuenscht = {muster["name"] for muster in ORTE}
    vorhanden = {p["name"]: p for p in
                 ruf(basis, "GET", "/api/v1/places", verwaltung).get("places", [])}
    # Was nicht in dieser Datei steht, gehört nicht aufs Bild. Betrifft vor
    # allem die zwei Kästen „Unter den Eichen", die `SEED=1` anlegt: ohne
    # Aufgaben stünden sie mit „Nichts offen" in der Liste.
    for name in list(vorhanden):
        if name in gewuenscht:
            for aufgabe in vorhanden[name].get("tasks", []):
                ruf(basis, "DELETE", f"/api/v1/tasks/{aufgabe['id']}", verwaltung, dulden=(404,))
        else:
            ruf(basis, "DELETE", f"/api/v1/places/{vorhanden[name]['id']}",
                verwaltung, dulden=(404,))
            del vorhanden[name]

    meldungen = 0
    for muster in ORTE:
        eingabe = {"name": muster["name"], "description": muster["beschreibung"],
                   "kind": muster["art"], "lat": muster["lat"], "lon": muster["lon"],
                   "active": True}
        if muster["name"] in vorhanden:
            ort_id = vorhanden[muster["name"]]["id"]
            ruf(basis, "PUT", f"/api/v1/places/{ort_id}", verwaltung, eingabe)
        else:
            ort_id = ruf(basis, "POST", "/api/v1/places", verwaltung, eingabe)["id"]

        for a in muster["aufgaben"]:
            aufgabe = {"kind": a["art"], "title": a.get("titel", ""),
                       "intervalDays": a["intervall"], "redAfterDays": a["rot"],
                       "active": True}
            if a.get("liter"):
                aufgabe["liters"] = float(a["liter"])
            aufgabe_id = ruf(basis, "POST", f"/api/v1/places/{ort_id}/tasks",
                             verwaltung, aufgabe)["id"]

            # Ältestes zuerst — die letzte Meldung setzt die Ampel.
            for alter, wer in sorted(a["meldungen"], key=lambda m: -m[0]):
                wann = (jetzt - timedelta(days=alter)).isoformat().replace("+00:00", "Z")
                nutzlast = {"doneAt": wann, "force": True}
                if a.get("liter"):
                    nutzlast["liters"] = float(a["liter"])
                ruf(basis, "POST", f"/api/v1/tasks/{aufgabe_id}/completions",
                    NAME_ZU_TOKEN[wer], nutzlast)
                meldungen += 1

    print(f"{len(ORTE)} Orte mit Aufgaben und {meldungen} Erledigungen angelegt.")

    # 4. Nachschauen, ob die Bühne trägt — eine leere Rangliste oder eine
    #    einfarbige Karte wäre ein schlechtes Bild, und das soll auffallen,
    #    bevor jemand den Simulator anwirft.
    orte = ruf(basis, "GET", "/api/v1/places", NAME_ZU_TOKEN["e2e-user"])["places"]
    ampeln = {}
    for ort in orte:
        ampeln[ort["status"]] = ampeln.get(ort["status"], 0) + 1
    rang = ruf(basis, "GET", "/api/v1/stats/leaderboard?period=saison",
               NAME_ZU_TOKEN["e2e-user"])
    print("Ampeln:", ", ".join(f"{k}: {v}" for k, v in sorted(ampeln.items())))
    print("Rangliste:", len(rang["entries"]), "Zeilen,",
          rang["totals"]["completions"], "Erledigungen,",
          rang["totals"]["participants"], "Beteiligte")
    for zeile in rang["entries"][:5]:
        print(f"  {zeile['rank']}. {zeile['userName']} — {zeile['completions']}")

    fehlt = [farbe for farbe in ("red", "yellow", "green") if not ampeln.get(farbe)]
    if fehlt:
        print("WARNUNG: keine Orte mit Ampel " + ", ".join(fehlt), file=sys.stderr)
    if len(rang["entries"]) < 5:
        print("WARNUNG: die Rangliste ist zu kurz für ein gutes Bild", file=sys.stderr)


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--basis", default="http://127.0.0.1:8080",
                   help="Adresse des lokalen Backends (Vorgabe http://127.0.0.1:8080)")
    hauptteil(p.parse_args().basis.rstrip("/"))
