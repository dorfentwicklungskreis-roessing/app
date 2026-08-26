#!/usr/bin/env python3
"""Baut den **lokalen** Kartenstil für die Store-Bilder.

Warum nicht einfach `android/e2e/fixtures/map-style.json`? Der Stil dort ist
für Tests gedacht und besteht aus einer einzigen Hintergrundfarbe — richtig,
solange nur geprüft wird, ob Nadeln erscheinen. Als erstes Suchergebnis im App
Store ist eine leere beige Fläche dagegen unbrauchbar: Sie sieht aus wie eine
kaputte Karte, und sie zeigt nicht, was die App wirklich tut.

Und warum nicht `tiles.openfreemap.org`? Weil kein Bild gegen einen fremden
Dienst aufgenommen wird — dieselbe Regel wie für Tests (CLAUDE.md).

Der Ausweg: Die Geometrie wird **einmal** hier geholt, in den Stil eingebettet
und als `map-style.json` neben dieses Skript gelegt. Beim Aufnehmen der Bilder
liefert der lokale `python3 -m http.server` nur noch diese Datei aus — kein
Kachelserver, keine Netzverbindung, jedes Mal dasselbe Bild.

Quelle ist die Overpass-API von OpenStreetMap. Es sind ausschließlich
öffentliche Geodaten (Straßen, Gebäudeumrisse, Wasser, Flächennutzung) —
nichts, was mit Dorfbewohnern zu tun hätte.

    python3 store/screenshots/beispieldaten/kartenstil_bauen.py

Lizenz der Daten: ODbL, © OpenStreetMap-Mitwirkende. Der Hinweis steht als
`attribution` in jeder Quelle des Stils; MapLibre zeigt ihn über das
(i)-Zeichen auf der Karte.
"""

from __future__ import annotations

import json
import urllib.parse
import urllib.request
from pathlib import Path

# Ausschnitt um Rössing. Etwas größer als die Beispielorte in `fuellen.py`,
# damit auch beim Verschieben der Karte nicht plötzlich das Nichts anfängt.
SUED, WEST, NORD, OST = 52.1870, 9.7980, 52.2070, 9.8330

OVERPASS = "https://overpass-api.de/api/interpreter"
ZIEL = Path(__file__).resolve().parent / "map-style.json"
HERKUNFT = "© OpenStreetMap-Mitwirkende"

ABFRAGE = f"""
[out:json][timeout:120];
(
  way["highway"]({SUED},{WEST},{NORD},{OST});
  way["building"]({SUED},{WEST},{NORD},{OST});
  way["waterway"~"^(river|stream|canal|ditch)$"]({SUED},{WEST},{NORD},{OST});
  way["natural"="water"]({SUED},{WEST},{NORD},{OST});
  way["landuse"]({SUED},{WEST},{NORD},{OST});
  way["leisure"~"^(park|pitch|garden|playground|sports_centre)$"]({SUED},{WEST},{NORD},{OST});
  way["railway"~"^(rail|light_rail)$"]({SUED},{WEST},{NORD},{OST});
);
out geom;
"""

# Straßenklassen, gröber zusammengefasst als in OSM: Für eine Dorfkarte
# reichen drei Breiten.
GROSS = {"motorway", "trunk", "primary", "secondary", "motorway_link", "trunk_link",
         "primary_link", "secondary_link"}
MITTEL = {"tertiary", "unclassified", "residential", "living_street", "tertiary_link"}


def holen() -> dict:
    daten = urllib.parse.urlencode({"data": ABFRAGE}).encode()
    req = urllib.request.Request(OVERPASS, data=daten,
                                 headers={"User-Agent": "dorf-app-store-screenshots/1.0"})
    with urllib.request.urlopen(req, timeout=180) as antwort:
        return json.loads(antwort.read())


def runden(punkte):
    """Fünf Nachkommastellen sind gut ein Meter — mehr braucht eine Dorfkarte
    nicht, und die Datei wird nur halb so groß."""
    return [[round(p["lon"], 5), round(p["lat"], 5)] for p in punkte]


def geschlossen(koordinaten) -> bool:
    return len(koordinaten) > 3 and koordinaten[0] == koordinaten[-1]


def sammeln(roh: dict) -> dict[str, list]:
    eimer: dict[str, list] = {"flaeche": [], "wasser": [], "gebaeude": [],
                              "weg_klein": [], "weg_mittel": [], "weg_gross": [],
                              "schiene": []}
    for element in roh.get("elements", []):
        if element.get("type") != "way" or not element.get("geometry"):
            continue
        marken = element.get("tags", {})
        koordinaten = runden(element["geometry"])
        if len(koordinaten) < 2:
            continue

        def flaeche(art: str, name: str):
            if geschlossen(koordinaten):
                eimer[art].append({
                    "type": "Feature", "properties": {"art": name},
                    "geometry": {"type": "Polygon", "coordinates": [koordinaten]},
                })

        def linie(art: str):
            eimer[art].append({
                "type": "Feature", "properties": {},
                "geometry": {"type": "LineString", "coordinates": koordinaten},
            })

        if "building" in marken:
            flaeche("gebaeude", "gebaeude")
        elif marken.get("natural") == "water":
            flaeche("wasser", "wasser")
        elif "waterway" in marken:
            # Bäche liegen als Linie in derselben Quelle wie die Seen; die
            # beiden Ebenen des Stils trennen sie über `geometry-type`.
            linie("wasser")
        elif "railway" in marken:
            linie("schiene")
        elif "highway" in marken:
            art = marken["highway"]
            if art in GROSS:
                linie("weg_gross")
            elif art in MITTEL:
                linie("weg_mittel")
            else:
                linie("weg_klein")
        elif "landuse" in marken or "leisure" in marken:
            wert = marken.get("landuse") or marken.get("leisure")
            flaeche("flaeche", wert)
    return eimer


def quelle(merkmale: list) -> dict:
    return {"type": "geojson", "attribution": HERKUNFT,
            "data": {"type": "FeatureCollection", "features": merkmale}}


def stil_bauen(eimer: dict[str, list]) -> dict:
    # Farben in Anlehnung an den Standardstil von OpenStreetMap: gedeckt,
    # damit die Ampel-Nadeln der App die kräftigsten Farben im Bild bleiben.
    gruen = ["farmland", "farmyard", "meadow", "grass", "orchard", "allotments",
             "village_green", "recreation_ground", "park", "garden", "pitch",
             "playground", "sports_centre", "cemetery", "forest"]
    return {
        "version": 8,
        "name": "Rössing (lokal, OpenStreetMap-Daten)",
        "metadata": {
            "hinweis": "Erzeugt von store/screenshots/beispieldaten/kartenstil_bauen.py. "
                       "Enthält die Geometrie fest eingebettet — beim Aufnehmen der "
                       "Store-Bilder wird kein Kachelserver angefragt.",
            "lizenz": "ODbL, " + HERKUNFT,
        },
        "sources": {
            "flaeche": quelle(eimer["flaeche"]),
            "wasser": quelle(eimer["wasser"]),
            "gebaeude": quelle(eimer["gebaeude"]),
            "weg_klein": quelle(eimer["weg_klein"]),
            "weg_mittel": quelle(eimer["weg_mittel"]),
            "weg_gross": quelle(eimer["weg_gross"]),
            "schiene": quelle(eimer["schiene"]),
        },
        "layers": [
            {"id": "hintergrund", "type": "background",
             "paint": {"background-color": "#f2efe9"}},
            {"id": "flaechen", "type": "fill", "source": "flaeche",
             "paint": {"fill-color": ["match", ["get", "art"], gruen, "#dcecc8",
                                      ["residential", "retail", "commercial"], "#eae7e2",
                                      ["industrial"], "#e6e0e6",
                                      "#eceae4"],
                       "fill-opacity": 0.9}},
            {"id": "wasserflaechen", "type": "fill", "source": "wasser",
             "filter": ["==", ["geometry-type"], "Polygon"],
             "paint": {"fill-color": "#aad3df"}},
            {"id": "wasserlaeufe", "type": "line", "source": "wasser",
             "filter": ["==", ["geometry-type"], "LineString"],
             "paint": {"line-color": "#aad3df", "line-width": 2.5}},
            {"id": "gebaeude", "type": "fill", "source": "gebaeude",
             "paint": {"fill-color": "#dfd8ce", "fill-outline-color": "#c9c0b4"}},
            # Straßen zweifach: erst der dunkle Rand, dann die helle Füllung.
            {"id": "wege-klein-rand", "type": "line", "source": "weg_klein",
             "layout": {"line-cap": "round", "line-join": "round"},
             "paint": {"line-color": "#d6d0c6", "line-width": 3}},
            {"id": "wege-klein", "type": "line", "source": "weg_klein",
             "layout": {"line-cap": "round", "line-join": "round"},
             "paint": {"line-color": "#ffffff", "line-width": 1.6}},
            {"id": "wege-mittel-rand", "type": "line", "source": "weg_mittel",
             "layout": {"line-cap": "round", "line-join": "round"},
             "paint": {"line-color": "#c8c2b6", "line-width": 7}},
            {"id": "wege-mittel", "type": "line", "source": "weg_mittel",
             "layout": {"line-cap": "round", "line-join": "round"},
             "paint": {"line-color": "#ffffff", "line-width": 5}},
            {"id": "wege-gross-rand", "type": "line", "source": "weg_gross",
             "layout": {"line-cap": "round", "line-join": "round"},
             "paint": {"line-color": "#e2b86a", "line-width": 10}},
            {"id": "wege-gross", "type": "line", "source": "weg_gross",
             "layout": {"line-cap": "round", "line-join": "round"},
             "paint": {"line-color": "#f7e06e", "line-width": 7.5}},
            {"id": "schienen", "type": "line", "source": "schiene",
             "paint": {"line-color": "#9a9a9a", "line-width": 2,
                       "line-dasharray": [3, 3]}},
        ],
    }


if __name__ == "__main__":
    print("Hole OpenStreetMap-Daten für den Ausschnitt um Rössing …")
    eimer = sammeln(holen())
    for name, merkmale in eimer.items():
        print(f"  {name}: {len(merkmale)}")
    ZIEL.write_text(json.dumps(stil_bauen(eimer), ensure_ascii=False,
                               separators=(",", ":")), encoding="utf-8")
    print(f"{ZIEL} geschrieben ({ZIEL.stat().st_size // 1024} KiB).")
