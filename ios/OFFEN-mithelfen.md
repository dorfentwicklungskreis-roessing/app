# Offen aus dem Bereich „Mithelfen"

Was der Bereich außerhalb von `Dorf/Bereiche/Mithelfen/` bräuchte. Hier steht
es, statt dass ein Bereich eine fremde Datei anfasst.

- `Navigation/StartView.swift`: `Bereichskachel` hat schon ein Feld `hinweis`
  („3 Orte warten auf dich"), benutzt es aber nirgends. Für „Mithelfen" wäre
  die Zahl der gelben und roten Orte dort der beste Platz — dafür müsste die
  Startseite die Orte kennen (etwa ein `OrteModell` in `AppUmgebung`, das
  Startseite und Bereich teilen).
- `Bereiche/Karte/KarteView.swift`: Die Karte meldet nur einen Tipp zurück.
  Ein „zeige auf Ort X" (etwa beim Umschalten von der Liste zur Karte) ginge
  nur mit einer Erweiterung der Schnittstelle — bewusst nicht angefasst.
- Der Wetterfaktor (`OrteAntwort.wateringFactor`) wird geladen und im Modell
  gehalten, aber noch nirgends gezeigt. Android macht daraus einen Hinweis auf
  der Startseite („Heiß — bitte großzügig gießen"); das gehört eher auf die
  Startseite als in die Ortsliste.
- Die Vergabe (`Aufgabe.assignment`, `signupCount`, `signedUp`) kommt schon in
  den DTOs an, hat aber noch keinen Weg im `DorfApi` (kein `claim`, `release`,
  `signup`). Der Bereich zeigt sie deshalb noch nicht — laut `README.md` ist
  das bewusst nach der ersten Fassung vorgesehen.
