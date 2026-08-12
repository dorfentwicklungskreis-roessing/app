// Karte der Verwaltung — die einzige Stelle, an der überhaupt JavaScript
// läuft. Alles andere (Navigation, Formulare, Löschen) funktioniert ohne.
// Deshalb gilt hier: niemals werfen, im Zweifel still abschalten.
(function () {
  "use strict";

  var behaelter = document.getElementById("karte");
  if (!behaelter) return;

  var mitte = [9.87, 52.211];
  var farben = ["match", ["get", "status"], "yellow", "#f9a825", "red", "#c62828", "#2e7d32"];

  function zustand(wert) {
    document.body.dataset.mapState = wert;
    behaelter.dataset.zustand = wert;
  }

  function abschalten(grund) {
    console.warn("Karte nicht verfügbar:", grund);
    zustand("error");
    var hinweis = document.getElementById("karte-hinweis");
    if (hinweis) hinweis.textContent = "Die Karte konnte nicht geladen werden. Die Verwaltung funktioniert trotzdem.";
  }

  function daten() {
    try {
      return JSON.parse(behaelter.dataset.karte || '{"type":"FeatureCollection","features":[]}');
    } catch (e) {
      return { type: "FeatureCollection", features: [] };
    }
  }

  if (typeof maplibregl === "undefined") {
    abschalten("MapLibre wurde nicht geladen");
    return;
  }

  var punkte = daten();
  behaelter.dataset.markers = String((punkte.features || []).length);
  if (punkte.features && punkte.features.length) {
    mitte = punkte.features[0].geometry.coordinates;
  }

  var karte;
  try {
    zustand("loading");
    karte = new maplibregl.Map({
      container: "karte",
      style: "https://tiles.openfreemap.org/styles/liberty",
      center: mitte,
      zoom: 15,
    });
  } catch (e) {
    abschalten(e);
    return;
  }

  karte.addControl(new maplibregl.NavigationControl());
  // MapLibre meldet fehlende Kacheln als Fehler-Event; das darf die Seite
  // nicht als Konsolenfehler verlassen.
  karte.on("error", function (e) {
    console.warn("MapLibre:", (e && e.error && e.error.message) || e);
  });

  var feldLat = document.getElementById("feld-lat");
  var feldLon = document.getElementById("feld-lon");
  var waehlbar = behaelter.dataset.waehlbar === "1" && feldLat && feldLon;

  function auswahlZeichnen(lng, lat) {
    var geojson = {
      type: "FeatureCollection",
      features: [{ type: "Feature", geometry: { type: "Point", coordinates: [lng, lat] }, properties: {} }],
    };
    var quelle = karte.getSource("auswahl");
    if (quelle) {
      quelle.setData(geojson);
      return;
    }
    karte.addSource("auswahl", { type: "geojson", data: geojson });
    karte.addLayer({
      id: "auswahl-punkt",
      type: "circle",
      source: "auswahl",
      paint: { "circle-radius": 9, "circle-color": "#1d4ed8", "circle-stroke-width": 3, "circle-stroke-color": "#ffffff" },
    });
  }

  karte.on("load", function () {
    try {
      karte.addSource("orte", { type: "geojson", data: punkte });
      karte.addLayer({
        id: "orte-punkte",
        type: "circle",
        source: "orte",
        paint: {
          "circle-radius": 10,
          "circle-color": farben,
          "circle-stroke-width": 2.5,
          "circle-stroke-color": "#ffffff",
        },
      });
      if (waehlbar) {
        var lat = parseFloat(String(feldLat.value).replace(",", "."));
        var lon = parseFloat(String(feldLon.value).replace(",", "."));
        if (isFinite(lat) && isFinite(lon)) auswahlZeichnen(lon, lat);
      }
      zustand("ready");
    } catch (e) {
      abschalten(e);
    }
  });

  // Klick auf einen Ort öffnet dessen Detailseite (echte Navigation).
  karte.on("click", function (e) {
    try {
      var treffer = karte.getLayer("orte-punkte")
        ? karte.queryRenderedFeatures(e.point, { layers: ["orte-punkte"] })
        : [];
      if (treffer.length && !waehlbar) {
        window.location.href = "/admin/dorfpflege/orte/" + treffer[0].properties.id;
        return;
      }
      if (waehlbar) {
        feldLat.value = e.lngLat.lat.toFixed(6);
        feldLon.value = e.lngLat.lng.toFixed(6);
        auswahlZeichnen(e.lngLat.lng, e.lngLat.lat);
      }
    } catch (err) {
      console.warn("Kartenklick:", err);
    }
  });
})();
