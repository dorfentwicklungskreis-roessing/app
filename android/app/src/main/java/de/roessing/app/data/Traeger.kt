package de.roessing.app.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import retrofit2.HttpException

/**
 * Träger (Vereine und Gruppen) und die Beitritte zu ihnen.
 *
 * What arrives here is already decided. The list holds only what this person
 * may see, and every entry carries what applies to them: member, may
 * administer, may join — and if not, why not, as a finished German sentence.
 * None of that is recomputed here. The rules live in `model.Zugriff` in the
 * backend; a second check in the app would answer differently than the server
 * at the next special case.
 *
 * The field names follow the wire format, which is German — a DTO's job is to
 * match the JSON, not to translate it.
 */

/** One Traeger, in the view of the person who asked. */
@Serializable
data class TraegerDto(
    val id: Long = 0,
    val name: String = "",
    val beschreibung: String = "",
    /** beantragt · zugelassen · gesperrt */
    val status: String = "",
    /** offen · geschlossen */
    val sichtbarkeit: String = "",
    /**
     * The roof this Traeger works under — 0 means none. Exactly one level:
     * association → working group. Rights travel along it in neither
     * direction; whoever administers the working group does not thereby
     * administer the association.
     */
    val parentId: Long = 0,
    val istMitglied: Boolean = false,
    val darfVerwalten: Boolean = false,
    /** This person can file a request right now. */
    val beitrittMoeglich: Boolean = false,
    /** Why not — meant to be shown verbatim. */
    val beitrittHindernis: String = "",
    /** How my own request stands: beantragt · erteilt · abgelehnt. */
    val beitrittStatus: String = "",
    /** Undecided requests — filled only for those who administer. */
    val offeneBeitritte: Int = 0,
) {
    /**
     * A closed group is in the directory for members only, and it takes no
     * requests — it takes people in itself. What follows for the buttons is
     * [beitrittMoeglich]; this is only for the sentence beside them.
     */
    val istGeschlossen: Boolean get() = sichtbarkeit == "geschlossen"
}

@Serializable
data class TraegerListResponse(val traeger: List<TraegerDto> = emptyList())

@Serializable
data class TraegerResponse(val traeger: TraegerDto = TraegerDto())

/**
 * A request for membership in a Traeger.
 *
 * A granted request is **not** the membership: that one lives in the
 * Rössing-ID. Here stands the proceeding — who asked when, who decided when.
 * Which is why granting can fail while the request stays open.
 */
@Serializable
data class BeitrittDto(
    val id: Long = 0,
    val traegerId: Long = 0,
    val traegerName: String = "",
    val userSub: String = "",
    val userName: String = "",
    /** beantragt · erteilt · abgelehnt */
    val status: String = "",
    val begruendung: String = "",
    val notiz: String = "",
    val createdAt: String = "",
) {
    /** The server resolves the name from the profile; the bare identifier is
     *  the fallback, not the normal case. */
    val anzeigename: String get() = userName.ifBlank { userSub }
}

@Serializable
data class BeitritteResponse(val beitritte: List<BeitrittDto> = emptyList())

/** Body of `POST /api/v1/traeger/{id}/beitritt`. */
@Serializable
data class BeitrittInput(val begruendung: String = "")

/** Body of `POST /api/v1/beitritte/{id}`. */
@Serializable
data class BeitrittDecisionInput(val status: String, val notiz: String = "")

/** Body of `POST /api/v1/traeger/{id}/mitglieder`. */
@Serializable
data class MitgliedInput(val userSub: String, val notiz: String = "")

/**
 * The server refused, and it says why in plain German.
 *
 * This one exception covers every refusal that carries a sentence — 409
 * ("you already belong", "closed group"), 403, and above all 502/503 from
 * writing back into the Rössing-ID. The sentence is passed on word for word:
 * an app that reports "taken in" while the door stays shut would be worse
 * than no app at all.
 */
class TraegerRefusedException(val grund: String) : RuntimeException(grund)

interface TraegerRepository {
    suspend fun list(): List<TraegerDto>
    suspend fun detail(id: Long): TraegerDto
    suspend fun join(id: Long, reason: String): BeitrittDto
    suspend fun requests(traegerId: Long): List<BeitrittDto>
    suspend fun myRequests(): List<BeitrittDto>
    suspend fun decide(requestId: Long, status: String): BeitrittDto
    suspend fun addMember(traegerId: Long, userSub: String): BeitrittDto
    suspend fun villagers(): List<MemberDto>
}

class ApiTraegerRepository(private val api: DorfApi) : TraegerRepository {
    override suspend fun list(): List<TraegerDto> = api.traeger().traeger

    override suspend fun detail(id: Long): TraegerDto = api.traegerDetail(id).traeger

    override suspend fun join(id: Long, reason: String): BeitrittDto =
        mitGrund { api.beitritt(id, BeitrittInput(reason)) }

    override suspend fun requests(traegerId: Long): List<BeitrittDto> =
        mitGrund { api.beitritte(traegerId).beitritte }

    override suspend fun myRequests(): List<BeitrittDto> = api.meineBeitritte().beitritte

    override suspend fun decide(requestId: Long, status: String): BeitrittDto =
        mitGrund { api.entscheideBeitritt(requestId, BeitrittDecisionInput(status)) }

    override suspend fun addMember(traegerId: Long, userSub: String): BeitrittDto =
        mitGrund { api.mitgliedAufnehmen(traegerId, MitgliedInput(userSub)) }

    override suspend fun villagers(): List<MemberDto> = api.members().members

    /**
     * Turns a refusal that carries a sentence into [TraegerRefusedException].
     * Everything else stays as it is — a broken connection is not an answer
     * from the server and must not be dressed up as one.
     */
    private suspend fun <T> mitGrund(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        val grund = fehlergrund(e)
        if (grund.isNotBlank()) throw TraegerRefusedException(grund) else throw e
    }

    private fun fehlergrund(e: HttpException): String = runCatching {
        val roh = e.response()?.errorBody()?.string().orEmpty()
        Json { ignoreUnknownKeys = true }.decodeFromString<ApiErrorDto>(roh).error
    }.getOrNull().orEmpty()
}

/**
 * Ein leeres Verzeichnis — die Vorgabe für Oberflächen-Tests und Vorschauen,
 * die mit Vereinen nichts zu tun haben.
 */
object KeineTraeger : TraegerRepository {
    override suspend fun list(): List<TraegerDto> = emptyList()
    override suspend fun detail(id: Long): TraegerDto = TraegerDto(id = id)
    override suspend fun join(id: Long, reason: String): BeitrittDto = BeitrittDto(traegerId = id)
    override suspend fun requests(traegerId: Long): List<BeitrittDto> = emptyList()
    override suspend fun myRequests(): List<BeitrittDto> = emptyList()
    override suspend fun decide(requestId: Long, status: String): BeitrittDto =
        BeitrittDto(id = requestId, status = status)

    override suspend fun addMember(traegerId: Long, userSub: String): BeitrittDto =
        BeitrittDto(traegerId = traegerId, userSub = userSub, status = "erteilt")

    override suspend fun villagers(): List<MemberDto> = emptyList()
}
