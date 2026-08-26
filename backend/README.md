# Backend der Dorf-App

Der Überblick über Aufbau, Betrieb und Entwicklung steht im
[README des Repos](../README.md); die Sicherheits- und Datenschutzfragen in
[SICHERHEIT.md](SICHERHEIT.md). Diese Datei beschreibt Endpunkte, deren
Verhalten sich nicht von selbst versteht.

## `DELETE /api/v1/me` — das eigene Konto löschen

Angemeldet wie alle `/api/v1`-Routen (JWT, `internal/api/api.go`). Der Rumpf
ist **optional**; ein `DELETE` ohne Inhalt ist der Normalfall. Wird einer
mitgeschickt, darf er nur `{"userSub": "<eigene Kennung>"}` enthalten — eine
fremde Kennung ergibt **403**, auch für Admins. Gelöscht wird ausschließlich
das eigene Konto; das Löschen von außen gehört in die Verwaltung, nicht in
die Selbstbedienung (dieselbe Regel wie bei `PUT /api/v1/me/profile`).

Es gibt diesen Weg, weil es ihn geben muss: Apples Richtlinie 5.1.1 (v)
verlangt von jeder App mit Konto einen Weg zum Löschen **in der App**, die
DSGVO (Art. 17) verlangt ihn ohnehin.

### Was passiert (`internal/db/konto.go`, alles in einer Transaktion)

| Daten | was damit geschieht |
| --- | --- |
| Profil (`profiles`) | gelöscht |
| Gerätekennungen (`push_devices`) | gelöscht — der Push hört sofort auf |
| Helfer-Eintragungen (`care_signups`) | gelöscht |
| Zustellungen (`care_notifications`) | gelöscht |
| Befähigungsanträge (`befaehigungs_antraege`) | gelöscht, samt Antrag, Begründung und interner Notiz |
| laufende Zusagen (`care_assignments`, `ended_at` leer) | **freigegeben**: wieder `offen`, Zusagender entfernt, neues Angebot fällig |
| beendete Zusagen | Kennung entfernt, Name durch den Ersatznamen ersetzt |
| Erledigungen (`completions`) | **bleiben**, aber ohne Kennung und unter dem Ersatznamen |
| eingereichte Ideen (`ideen`) | bleiben (die Verwaltung arbeitet sie ab), verlieren aber Kennung, Name und E-Mail |

Der Ersatzname ist „Ehemaliges Mitglied" (`api.LoeschErsatzname`). Warum die
Erledigungen bleiben, steht im Kopf von `internal/db/konto.go` und in
[SICHERHEIT.md](SICHERHEIT.md), Abschnitt „Kontolöschung“.

Ein zweiter Aufruf ist unschädlich — dann ist schlicht nichts mehr da.

### Was der Endpunkt **nicht** tut

Er löscht **nicht** das Konto in der **Rössing-ID**. Das gehört Zitadel, und
dieselbe Anmeldung dient auch anderen Anwendungen des Dorfes; die Dorf-App
darf sie nicht mit wegräumen. Die Antwort sagt das im Klartext, damit niemand
glaubt, mit dem einen Knopf sei beides erledigt.

### Antwort

**200** mit Erklärung statt 204 — hier gibt es etwas zu sagen, und die App
zeigt es an, bevor sie sich abmeldet:

```json
{
  "geloescht": true,
  "bilanz": {
    "profil": true, "geraete": 1, "anmeldungen": 3, "benachrichtigungen": 12,
    "erledigungen": 47, "zusagen": 2, "befaehigungen": 1, "ideen": 0
  },
  "erledigungen": "Deine Meldungen bleiben anonym stehen …",
  "roessingId": "Deine Rössing-ID bleibt bestehen: …",
  "roessingIdUrl": "https://id.xn--rssing-wxa.de"
}
```

Die `bilanz` sagt nur, wie viele Zeilen betroffen waren — sie enthält keine
personenbezogenen Daten und ist vor allem für die Tests und für die
Rückmeldung in der App da.
