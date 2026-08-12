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
    val active: Boolean = true,
    val status: String = "green",
    val lastCompletion: CompletionDto? = null,
    val dueAt: String = "",
    val redAt: String = "",
) {
    val careStatus: CareStatus get() = parseStatus(status)

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
)

@Serializable
data class CompletionInput(val liters: Double? = null, val note: String = "")
