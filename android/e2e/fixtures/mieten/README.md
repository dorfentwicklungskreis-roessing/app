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
| `sets.json` | 4 — `GET /api/v1/sets` |
| `availability-free.json`, `availability-taken.json` | 5 — `GET /api/v1/availability` |
| `occupancy.json` | 6 — `GET /api/v1/occupancy` |
| `me.json`, `me-lender.json` | 7 — `GET /api/v1/me` |
| `me-updated.json` | 8 — `PATCH /api/v1/me` |
| `lender-request.json` | 9 — `POST /api/v1/me/lender-request` |
| `my-bookings.json` | 10 — `GET /api/v1/bookings/mine` |
| `booking-created.json` | 11 — `POST /api/v1/bookings` |
| `status-cancelled.json` | 12 — `POST /api/v1/bookings/{id}/cancel` |
| `owner-bookings.json` | 13 — `GET /api/v1/owner/bookings` |
| `status-approved.json` | 14 — `POST /api/v1/bookings/{id}/approve` |
| `owner-items.json` | 16 — `GET /api/v1/owner/items` |
| `blocks.json` | 17 — `GET /api/v1/owner/blocks` |
| `block-created.json` | 18 — `POST /api/v1/owner/blocks` |
| `block-deleted.json` | 19 — `DELETE /api/v1/owner/blocks/{id}` |
| `error-occupied.json` | 409 auf Route 11 |
| `error-block-occupied.json` | 409 auf Route 18 |
| `error-profile-incomplete.json` | 400 auf Route 11 |
| `error-not-a-lender.json` | 403 auf den Routen 13 bis 19 |
| `error-conflict.json` | 409 auf den Routen 12, 14 und 15 |
| `error-token-audience.json` | 401 auf jeder angemeldeten Route |

Route 15 (`reject`) antwortet in derselben Form wie Route 14, nur mit
`"rejected"`; dafür liegt keine eigene Datei hier, sondern der Test setzt den
einen Wert selbst. `me-lender.json` ist dasselbe Profil wie `me.json`, nur mit
`lenderStatus: "approved"` — daran entscheidet sich, ob die Vermieteransicht
überhaupt erscheint.
