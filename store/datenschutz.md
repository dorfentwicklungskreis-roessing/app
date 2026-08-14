# Datenschutz — Kurzfassung für den Play Store

> **Achtung:** Dieses Dokument ist die Arbeitsgrundlage, **nicht** die verbindliche
> Erklärung. Verbindlich ist die veröffentlichte Fassung:
>
> **URL: <https://xn--rssing-wxa.de/app/datenschutz/>**
> (in der Play Console unter *App-Inhalte → Datenschutzerklärung* eingetragen)
>
> Bei Abweichungen gilt die Website. Wer hier etwas ändert, das den Umgang mit
> Daten betrifft, muss die Website nachziehen — nicht umgekehrt.

## Verantwortlich

Dorfentwicklungskreis Rössing. Ladungsfähige Anschrift und E-Mail-Adresse stehen
in der veröffentlichten Fassung (Play prüft das stichprobenartig, und die DSGVO
verlangt es ohnehin).

## Welche Daten verarbeitet die App

**Anmeldedaten (Rössing-ID).** Die Anmeldung läuft über die Rössing-ID, eine vom
Dorf selbst betriebene Zitadel-Instanz unter `id.xn--rssing-wxa.de`. Die App
öffnet dafür den System-Browser (OAuth 2.0 Authorization Code mit PKCE). Aus dem
Konto erhält die App die Nutzerkennung (`sub`), den angezeigten Namen, die
E-Mail-Adresse und die Rollen im Projekt. Anmeldedaten (Passwort) sieht die App
nie — sie werden nur im Browser gegenüber der Rössing-ID eingegeben.

**Zugangs- und Erneuerungstoken.** Sie liegen ausschließlich auf dem Gerät, in
einem privaten Bereich der App (DataStore). Beim Abmelden werden sie gelöscht.

**Erledigungsmeldungen.** Meldet jemand „gegossen" oder „gejätet", speichert der
Dorfserver: Nutzerkennung, angezeigter Name, betroffene Aufgabe, Zeitpunkt,
gegebenenfalls die Litermenge und eine optionale Notiz. Diese Meldungen sind für
alle angemeldeten Dorfbewohner sichtbar — in der Historie eines Ortes und in der
Rangliste. Genau das ist der Zweck der App.

**Gerätestandort — nur auf dem Gerät.** Wer die Standortfreigabe erteilt, sieht
die eigene Position auf der Karte, die Entfernung zu jedem Pflege-Ort und kann
die Liste nach Nähe sortieren. Die App fragt dafür einen einzelnen Standort ab
(zuletzt bekannte Position, sonst ein einmaliger Fix mit Zeitlimit) — kein
Dauer-Tracking, kein Hintergrundstandort. **Die Position wird nicht gespeichert
und nicht übertragen**, weder an den Dorfserver noch an Dritte; die API der App
kennt kein Feld dafür. Ohne Freigabe funktioniert die App vollständig, nur ohne
Entfernungsangaben.

**Mithelfen und Anfragen.** Wer sich an einem Ort als Helfer:in einträgt,
hinterlegt damit: Kennung, Ort, ggf. Aufgabenart und Zeitpunkt. Steht dort
etwas an, entsteht daraus eine Anfrage (gespeichert: Kennung, Anlass,
Zeitpunkt, ob gelesen). Gefragt wird **nacheinander** im Abstand einer Stunde,
damit nicht alle gleichzeitig losziehen. Sagt niemand zu, geht am Ende der
Liste ein **Rundruf** an alle Eingetragenen gleichzeitig — ausgenommen, wessen
Zusage in diesem Vorgang schon verfallen ist. Auch danach passiert nichts
weiter: Der Ort bleibt fällig. Wer zusagt, ist mit Namen und Frist (24
Stunden) für die übrigen Eingetragenen sichtbar — sonst gießen zwei Leute
denselben Kasten. Das Eintragen ist freiwillig und jederzeit widerrufbar;
Anfragen und Vorgänge hängen an der Pflegeaufgabe und verschwinden mit ihr.

**Benachrichtigungen aufs Handy (freiwillig).** Wer zustimmt, bekommt Anfragen
als Push-Nachricht. Dafür vergibt **Google (Firebase Cloud Messaging)** eine
Kennung dieser App-Installation; die App meldet sie dem Dorfserver, damit er
Nachrichten an dieses Gerät schicken kann. **Diese Kennung entsteht erst, wenn
Benachrichtigungen erlaubt sind** — wer sie ablehnt oder in den
Android-Einstellungen abschaltet, für dessen Gerät wird gar keine vergeben —
auch das Firebase-SDK selbst bleibt bis dahin stumm und meldet sich nicht von
sich aus bei Google an.
An Google gehen dabei:

- die Kennung der App-Installation,
- Titel und Text der Meldung (also Ortsname, Aufgabe und ggf. die Frist),
- dazu ein technischer Datenteil, den die App braucht, um beim Antippen die
  richtige Stelle zu öffnen: Ortsname, Aufgabenname, die internen Nummern von
  Ort, Aufgabe, Vorgang und Benachrichtigung, die Art der Nachricht sowie der
  Ablaufzeitpunkt einer Anfrage.

**Namen anderer Personen stehen nie in einer solchen Nachricht.** Verschickt
wird bei einer Anfrage, beim Rundruf und bei Hinweisen (Zusage abgelaufen oder
aufgehoben, Aufgabe schon erledigt oder nicht mehr nötig). Die Anfragen stehen
unabhängig davon beim Öffnen in der App — Push ist die Abkürzung, nicht der
einzige Weg. Wird die Erlaubnis in den Android-Einstellungen wieder entzogen,
löscht die App die Kennung beim nächsten Öffnen vom Dorfserver und wirft sie
danach auch bei Google weg; beim Abmelden aus der App ebenso, und ebenso, wenn
Google sie als ungültig meldet. Zwischen 21 und 7 Uhr wird nichts zugestellt.

**Ideen an den Dorfentwicklungskreis (freiwillig).** Über „Idee vorschlagen"
lässt sich ein frei getippter Wunsch einreichen, dazu wahlweise Name und
E-Mail-Adresse für Rückfragen (aus dem Profil vorbelegt, änder- und leerbar).
Die Einreichung ist nur für die Verwaltung sichtbar, wird nicht veröffentlicht
und erscheint anderen Nutzern nirgends in der App.

**Kein Tracking.** Keine Werbung, keine Analyse- oder Absturz-SDKs, keine
Werbe-ID, kein Profiling. Das Firebase-SDK ist ausschließlich für die
Benachrichtigungen eingebunden.

## Empfänger

- **Dorfserver** `app.xn--rssing-wxa.de` — betrieben vom Dorfentwicklungskreis auf
  eigener Infrastruktur (K3S-Cluster, SQLite-Datenbank).
- **Rössing-ID** `id.xn--rssing-wxa.de` — ebenfalls selbst betrieben.
- **OpenFreeMap** (`tiles.openfreemap.org`) — liefert die Kartenkacheln auf Basis
  von OpenStreetMap-Daten. Dabei erfährt dieser Dienst die IP-Adresse des Geräts
  und die abgerufenen Kachelbereiche, wie bei jedem Abruf einer Internetseite.
  Kein Vertragsverhältnis, kein Konto, kein API-Schlüssel.

- **Google Ireland Ltd. / Google LLC (Firebase Cloud Messaging)** — nur, wenn
  Benachrichtigungen erlaubt sind: Google stellt die Nachrichten zu und erfährt
  dabei die Gerätekennung, Titel und Text der Meldung sowie den technischen
  Datenteil (Ortsname, Aufgabenname, interne Nummern). Rechtsgrundlage ist
  die Einwilligung (Art. 6 Abs. 1 lit. a DSGVO), erteilt über die
  Systemabfrage; sie lässt sich jederzeit in den Android-Einstellungen
  zurücknehmen. Datenübermittlung in die USA auf Grundlage der
  Standardvertragsklauseln bzw. des EU-US Data Privacy Framework.

Über diese drei hinaus gibt es keine Weitergabe an Dritte. Nichts wird
verkauft, es gibt keine Werbe- und keine Analysepartner.

## Rechtsgrundlage

Berechtigtes Interesse bzw. Vertragserfüllung (Art. 6 Abs. 1 lit. b und f DSGVO):
Ohne Kennung und Namen lässt sich nicht darstellen, wer welchen Kasten wann
gegossen hat — das ist der Sinn der gemeinsamen Übersicht.

Auf **Einwilligung** (Art. 6 Abs. 1 lit. a DSGVO) beruhen dagegen: der Eintrag
als Helfer:in samt der daraus entstehenden Anfragen, die Benachrichtigungen
aufs Handy, die freiwilligen Profilfelder samt ihrer Sichtbarkeit und die
Einreichung einer Idee. Jede dieser Einwilligungen lässt sich einzeln
zurücknehmen — durch Austragen, durch Entziehen der Benachrichtigungserlaubnis
in den Android-Einstellungen bzw. durch Leeren des Feldes.

## Speicherdauer und Löschung

- Eine irrtümliche Meldung kann die meldende Person selbst zurücknehmen
  (in der App bzw. über `DELETE /api/v1/completions/{id}`); Admins können jede
  Meldung löschen.
- Auf Wunsch werden alle Meldungen einer Person gelöscht bzw. anonymisiert und
  das Konto in der Rössing-ID entfernt. Anfragen an den Dorfentwicklungskreis.
- Die Gerätekennung für Benachrichtigungen wird gelöscht, sobald die Erlaubnis
  fehlt (Entzug in den Android-Einstellungen, spätestens beim nächsten Öffnen
  der App), beim Abmelden aus der App — und auch dann, wenn Google sie als
  ungültig meldet.
- Die von Play verlangte öffentliche Seite zur Löschung von Konto und Daten
  steht unter <https://xn--rssing-wxa.de/app/daten-loeschen/>.

## Rechte der Betroffenen

Auskunft, Berichtigung, Löschung, Einschränkung, Widerspruch, Datenübertragbarkeit
sowie Beschwerde bei der Landesbeauftragten für den Datenschutz Niedersachsen.

## Verschlüsselung

Sämtliche Verbindungen der App laufen über HTTPS (TLS). Die App spricht keinen
Klartext-Endpunkt an.

## Kinder

Die App richtet sich nicht an Kinder und ist nicht für den Bereich „Familien"
gedacht. Eine Nutzung setzt eine Rössing-ID voraus, die der Dorfentwicklungskreis
vergibt.
