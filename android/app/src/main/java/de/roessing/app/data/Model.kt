package de.roessing.app.data

import kotlinx.serialization.Serializable

/** Ampel-Status — Werte kommen 1:1 vom Backend. */
enum class CareStatus { green, yellow, red }

private fun parseStatus(raw: String): CareStatus =
    runCatching { CareStatus.valueOf(raw) }.getOrDefault(CareStatus.green)

@Serializable
data class CompletionDto(
    val id: Long = 0,
    val taskId: Long = 0,
    val userSub: String = "",
    val userName: String = "",
    val liters: Double? = null,
    val note: String = "",
    val doneAt: String = "",
)

@Serializable
data class TaskDto(
    val id: Long,
    val placeId: Long = 0,
    val kind: String = "giessen",
    val title: String = "",
    val liters: Double? = null,
    val intervalDays: Double = 7.0,
    val redAfterDays: Double = 14.0,
    /**
     * Einmalige Aufgabe („einmal zum Bahnhof fahren"): An die Stelle des
     * Intervalls tritt dann das Fälligkeitsdatum.
     */
    val oneOff: Boolean = false,
    /** Termin einer einmaligen Aufgabe (RFC3339) oder null. */
    val dueDate: String? = null,
    /** Nach dem Erledigen von Karte und Liste nehmen. */
    val removeWhenDone: Boolean = false,
    val active: Boolean = true,
    val status: String = "green",
    val lastCompletion: CompletionDto? = null,
    val dueAt: String = "",
    val redAt: String = "",
    /**
     * Spielschutz: bis dahin (RFC3339) darf nicht erneut gemeldet werden.
     * Fehlt, wenn die Aufgabe frei ist.
     */
    val lockedUntil: String? = null,
    /** Laufender Vergabe-Vorgang (null = gerade keiner). */
    val assignment: AssignmentDto? = null,
    /** Wie viele sich hier als Helfer:innen eingetragen haben. */
    val signupCount: Int = 0,
    /** Ob ich selbst hier eingetragen bin. */
    val signedUp: Boolean = false,
) {
    val careStatus: CareStatus get() = parseStatus(status)

    /** Zeitpunkt, bis zu dem der Spielschutz greift (oder null). */
    val lockedUntilInstant: java.time.Instant?
        get() = lockedUntil?.let { runCatching { java.time.Instant.parse(it) }.getOrNull() }

    /**
     * Eine einmalige Aufgabe, die schon erledigt ist. Sie wird nicht wieder
     * fällig, und das Backend weist eine zweite Meldung mit 409 ab — der
     * Knopf gehört also weg, nicht bloß gesperrt.
     */
    val erledigtUndVorbei: Boolean get() = oneOff && lastCompletion != null

    /** Termin einer einmaligen Aufgabe als Zeitpunkt (oder null). */
    val dueDateInstant: java.time.Instant?
        get() = dueDate?.let { runCatching { java.time.Instant.parse(it) }.getOrNull() }

    /** Menschenlesbarer Name der Aufgabe. */
    val displayName: String
        get() = title.ifEmpty {
            when (kind) {
                "giessen" -> "Gießen"
                "jaeten" -> "Jäten"
                else -> "Pflege"
            }
        }
}

/**
 * Ein laufender Vergabe-Vorgang zu genau einer Aufgabe. Die Regeln stehen im
 * Backend (internal/vergabe); hier interessiert nur, was anzuzeigen ist:
 * Hat jemand zugesagt, wer war es und bis wann hält die Zusage.
 */
@Serializable
data class AssignmentDto(
    val id: Long = 0,
    val taskId: Long = 0,
    /** offen · uebernommen · rundruf · beendet */
    val state: String = "offen",
    val claimedBy: String = "",
    val claimedByName: String = "",
    val claimedUntil: String? = null,
    val nextOfferAt: String? = null,
    val askedCount: Int = 0,
) {
    val uebernommen: Boolean get() = claimedBy.isNotEmpty()

    /** Habe ich selbst zugesagt? */
    fun vonMir(meinSub: String?): Boolean = claimedBy.isNotEmpty() && claimedBy == meinSub
}

/**
 * Eine Benachrichtigung aus der Vergabe: entweder eine Anfrage („du bist
 * dran"), auf die man zusagen kann, oder ein Hinweis, der nach dem Lesen
 * erledigt ist.
 */
@Serializable
data class NotificationDto(
    val id: Long = 0,
    val assignmentId: Long = 0,
    val taskId: Long = 0,
    val taskKind: String = "",
    val taskName: String = "",
    val placeId: Long = 0,
    val placeName: String = "",
    /** anfrage · rundruf · zusage_abgelaufen · zusage_aufgehoben · vorgang_beendet · vorgang_entfallen */
    val kind: String = "",
    val title: String = "",
    val text: String = "",
    val createdAt: String = "",
    val expiresAt: String? = null,
    val acknowledgedAt: String? = null,
) {
    /** Anfragen wollen eine Antwort; alles andere ist ein Hinweis. */
    val istAnfrage: Boolean get() = kind == ANFRAGE || kind == RUNDRUF

    companion object {
        const val ANFRAGE = "anfrage"
        const val RUNDRUF = "rundruf"
    }
}

@Serializable
data class NotificationsResponse(val notifications: List<NotificationDto> = emptyList())

/** Eingabe von POST /api/v1/places/{id}/signup (taskKind leer = alle Aufgaben). */
@Serializable
data class SignupInput(val taskKind: String = "")

/** Eingabe von POST/DELETE /api/v1/me/devices. */
@Serializable
data class DeviceInput(val token: String, val platform: String = "android")

@Serializable
data class PlaceDto(
    val id: Long,
    val name: String,
    val description: String = "",
    val kind: String = "blumenkasten",
    val lat: Double,
    val lon: Double,
    val active: Boolean = true,
    val status: String = "green",
    /**
     * Der Verein oder Arbeitskreis, dem dieser Ort gehört — der
     * Ansprechpartner. Leer, wenn das Backend ihn nicht mitschickt.
     *
     * Der Name kommt fertig vom Server, samt Verdeckung: Eine geschlossene
     * Gruppe heißt für Außenstehende „Eine Gruppe aus dem Dorf". Die App
     * entscheidet daran nichts, sie zeigt an — sonst gäbe es die
     * Sichtbarkeitsregel zweimal.
     */
    val traegerName: String = "",
    val tasks: List<TaskDto> = emptyList(),
) {
    val careStatus: CareStatus get() = parseStatus(status)
}

@Serializable
data class PlacesResponse(
    val places: List<PlaceDto> = emptyList(),
    val wateringFactor: Double = 1.0,
)

@Serializable
data class CompletionsResponse(val completions: List<CompletionDto> = emptyList())

@Serializable
data class MeDto(
    val sub: String = "",
    val name: String = "",
    val email: String = "",
    val roles: List<String> = emptyList(),
    val isAdmin: Boolean = false,
    /** Das eigene Profil — liefert das Backend beim Anmelden gleich mit. */
    val profile: ProfileDto? = null,
)

/**
 * Sichtbarkeit je Profilfeld. Werte des Backends: "dorf" (alle angemeldeten
 * Dorfbewohner) oder "verwaltung" (nur Verwaltende).
 *
 * Die Vorbelegung entspricht der des Backends: Kontaktdaten bleiben bei der
 * Verwaltung, bis jemand sie bewusst freigibt. Ein Wert, den diese
 * App-Version nicht kennt, gilt vorsichtshalber als nicht öffentlich.
 */
@Serializable
data class ProfileVisibilityDto(
    val displayName: String = SICHTBAR_DORF,
    val nickname: String = SICHTBAR_DORF,
    val phone: String = SICHTBAR_VERWALTUNG,
    val email: String = SICHTBAR_VERWALTUNG,
    val note: String = SICHTBAR_VERWALTUNG,
) {
    val displayNameIsPublic: Boolean get() = displayName == SICHTBAR_DORF
    val nicknameIsPublic: Boolean get() = nickname == SICHTBAR_DORF
    val phoneIsPublic: Boolean get() = phone == SICHTBAR_DORF
    val emailIsPublic: Boolean get() = email == SICHTBAR_DORF
    val noteIsPublic: Boolean get() = note == SICHTBAR_DORF

    companion object {
        const val SICHTBAR_DORF = "dorf"
        const val SICHTBAR_VERWALTUNG = "verwaltung"

        /** Wandelt einen Schalter der Oberfläche in den Backend-Wert. */
        fun wert(oeffentlich: Boolean): String =
            if (oeffentlich) SICHTBAR_DORF else SICHTBAR_VERWALTUNG
    }
}

/** Das eigene Profil. */
@Serializable
data class ProfileDto(
    val userSub: String = "",
    val displayName: String = "",
    val nickname: String = "",
    val phone: String = "",
    val email: String = "",
    val note: String = "",
    val visibility: ProfileVisibilityDto = ProfileVisibilityDto(),
    val updatedAt: String = "",
)

/** Eingabe von PUT /api/v1/me/profile. */
@Serializable
data class ProfileInput(
    val displayName: String = "",
    val nickname: String = "",
    val phone: String = "",
    val email: String = "",
    val note: String = "",
    val visibility: ProfileVisibilityDto = ProfileVisibilityDto(),
)

/**
 * Eine Person in der Dorfbewohner-Liste — mit genau den Feldern, die sie
 * freigegeben hat. Nicht freigegebene Felder kommen gar nicht erst mit.
 */
@Serializable
data class MemberDto(
    val userSub: String = "",
    /** Name in Rangliste und Erledigungen (Nickname, sonst Anzeigename). */
    val name: String = "",
    val displayName: String = "",
    val nickname: String = "",
    val phone: String = "",
    val email: String = "",
    val note: String = "",
    /**
     * Felder, die nur Verwaltende sehen, weil die Person sie nicht
     * freigegeben hat. Für gewöhnliche Mitglieder immer leer.
     */
    val restricted: List<String> = emptyList(),
) {
    fun nurFuerVerwaltung(feld: String): Boolean = feld in restricted
}

@Serializable
data class MembersResponse(
    val members: List<MemberDto> = emptyList(),
    /** true, wenn die Liste alles zeigt, weil der Abruf von Verwaltenden kam. */
    val adminView: Boolean = false,
)

@Serializable
data class CompletionInput(val liters: Double? = null, val note: String = "")

/** Eine Auszeichnung („Gießkanne des Monats" …) — Regeln stehen im Backend. */
@Serializable
data class BadgeDto(
    val key: String = "",
    val label: String = "",
    val description: String = "",
)

/** Eine Zeile der Rangliste. rank 0 heißt: im Zeitraum noch nichts gemeldet. */
@Serializable
data class LeaderboardEntryDto(
    val rank: Int = 0,
    val userSub: String = "",
    val userName: String = "",
    val completions: Int = 0,
    val byKind: Map<String, Int> = emptyMap(),
    val liters: Double = 0.0,
    val lastCompletion: String? = null,
    val badges: List<BadgeDto> = emptyList(),
)

/** Gesamtsummen des Dorfes im Zeitraum. */
@Serializable
data class LeaderboardTotalsDto(
    val completions: Int = 0,
    val byKind: Map<String, Int> = emptyMap(),
    val liters: Double = 0.0,
    val participants: Int = 0,
)

@Serializable
data class LeaderboardDto(
    val period: String = "saison",
    val from: String = "",
    val to: String = "",
    val entries: List<LeaderboardEntryDto> = emptyList(),
    val totals: LeaderboardTotalsDto = LeaderboardTotalsDto(),
    /** Der eigene Eintrag — auch, wenn er nicht in entries steht. */
    val me: LeaderboardEntryDto? = null,
)

/** Fehlerantwort des Backends (z.B. bei HTTP 409 mit Sperrfrist). */
@Serializable
data class ApiErrorDto(val error: String = "", val retryAfter: String? = null)
