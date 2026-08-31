# Beispielantworten der Mietplattform

Ausschnitte aus `docs/mieten-api.md` — je Route eine Datei, in genau der
Form, die der Vertrag festlegt. Sie sind die Vorlage für die Tests des
Bereichs „Maschinchenring" (`RentalApiTest`), damit ein geänderter Vertrag
hier auffällt und nicht erst als leere Liste auf dem Telefon.

Adressen in den Beispielen zeigen bewusst auf `example.invalid`: Ein Test
fasst keinen entfernten Server an, und eine echte Adresse in einer
Testdatei wäre eine Einladung dazu.

| Datei | Route |
| --- | --- |
| `items.json` | 1 — `GET /api/v1/items` |
| `item-detail.json` | 2 — `GET /api/v1/items/{id}` |
| `search.json` | 3 — `GET /api/v1/search` |
| `availability-free.json`, `availability-taken.json` | 5 — `GET /api/v1/availability` |
| `occupancy.json` | 6 — `GET /api/v1/occupancy` |
| `me.json` | 7 — `GET /api/v1/me` |
| `my-bookings.json` | 10 — `GET /api/v1/bookings/mine` |
| `booking-created.json` | 11 — `POST /api/v1/bookings` |
| `error-occupied.json` | 409 auf Route 11 |
| `error-profile-incomplete.json` | 400 auf Route 11 |
| `error-token-audience.json` | 401 auf jeder angemeldeten Route |

Sets (Route 4) und die Vermieterseite (Routen 13 bis 19) hat der Bereich
bewusst nicht — deshalb liegen dafür auch keine Beispiele hier.
