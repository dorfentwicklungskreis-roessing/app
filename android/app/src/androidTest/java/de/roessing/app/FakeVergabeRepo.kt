package de.roessing.app

import de.roessing.app.data.AssignmentDto
import de.roessing.app.data.NotificationDto
import de.roessing.app.data.VergabeRepository

/**
 * Vergabe-Attrappe für die Oberflächen-Tests: liefert die übergebenen
 * Benachrichtigungen und merkt sich, was gedrückt wurde. Kein Netz.
 */
class FakeVergabeRepo(
    private var offene: List<NotificationDto> = emptyList(),
) : VergabeRepository {
    val eingetragen = mutableListOf<Pair<Long, String?>>()
    val zugesagt = mutableListOf<Long>()

    override suspend fun notifications(): List<NotificationDto> = offene

    override suspend fun ack(id: Long) {
        offene = offene.filterNot { it.id == id && !it.istAnfrage }
    }

    override suspend fun signup(placeId: Long, taskKind: String?) {
        eingetragen += placeId to taskKind
    }

    override suspend fun signoff(placeId: Long, taskKind: String?) {
        eingetragen.removeAll { it.first == placeId }
    }

    override suspend fun claim(assignmentId: Long): AssignmentDto {
        zugesagt += assignmentId
        return AssignmentDto(id = assignmentId, state = "uebernommen")
    }

    override suspend fun release(assignmentId: Long) = AssignmentDto(id = assignmentId, state = "offen")
}
