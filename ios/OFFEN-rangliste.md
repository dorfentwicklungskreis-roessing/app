# Offen aus dem Bereich „Rangliste"

Nichts davon liegt unter `Dorf/Bereiche/Rangliste/`, deshalb steht es hier
statt im Quelltext. Nichts davon hat Eile.

- `Navigation/Ziel.swift`: Die Rangliste hängt unverändert unter `.rangliste`.
  Ein Tiefenlink auf einen bestimmten Zeitraum („Rangliste der Woche" aus
  einer Benachrichtigung) bräuchte einen Wert am Ziel — sinnvoll erst, wenn es
  Push gibt.
- `Daten/DorfApi.swift`: Die Rangliste ruft `api.rangliste(zeitraum:)` bei
  jedem Umschalten neu ab. Ein kurzer Zwischenspeicher je Zeitraum würde beim
  Hin- und Herschalten das Netz schonen; ohne ihn geht nichts kaputt, weil der
  zuletzt geladene Stand bei Netzausfall ohnehin stehen bleibt.
