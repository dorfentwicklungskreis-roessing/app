package de.roessing.app.data

/**
 * Dünne Repository-Schicht über der API. Cache des letzten Standes hält das
 * ViewModel, damit bei Netzfehlern weiter alte Daten sichtbar bleiben.
 */
interface PlacesRepository {
    suspend fun me(): MeDto
    suspend fun places(): PlacesResponse
    suspend fun complete(taskId: Long, liters: Double?, note: String = ""): CompletionDto
    suspend fun completions(taskId: Long): List<CompletionDto>
}

class ApiPlacesRepository(private val api: DorfApi) : PlacesRepository {
    override suspend fun me(): MeDto = api.me()
    override suspend fun places(): PlacesResponse = api.places()
    override suspend fun complete(taskId: Long, liters: Double?, note: String): CompletionDto =
        api.complete(taskId, CompletionInput(liters = liters, note = note))

    override suspend fun completions(taskId: Long): List<CompletionDto> =
        api.completions(taskId).completions
}
