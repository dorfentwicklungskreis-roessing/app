package de.roessing.app

import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlaceEingabe
import de.roessing.app.data.TaskDto
import de.roessing.app.data.TaskEingabe
import de.roessing.app.data.VerwaltungRepository

/**
 * Verwaltung für die Oberflächentests: merkt sich, was ans Backend gegangen
 * wäre. Die verbindlichen Regeln stehen im Backend und werden dort getestet.
 */
class FakeVerwaltung : VerwaltungRepository {
    val orte = mutableListOf<PlaceEingabe>()
    val aufgaben = mutableListOf<Pair<Long, TaskEingabe>>()
    val geloeschteAufgaben = mutableListOf<Long>()
    val geloeschteOrte = mutableListOf<Long>()

    override suspend fun ortAnlegen(eingabe: PlaceEingabe): PlaceDto {
        orte += eingabe
        return PlaceDto(id = orte.size.toLong(), name = eingabe.name, lat = eingabe.lat, lon = eingabe.lon)
    }

    override suspend fun ortAendern(id: Long, eingabe: PlaceEingabe): PlaceDto {
        orte += eingabe
        return PlaceDto(id = id, name = eingabe.name, lat = eingabe.lat, lon = eingabe.lon)
    }

    override suspend fun ortLoeschen(id: Long) {
        geloeschteOrte += id
    }

    override suspend fun aufgabeAnlegen(placeId: Long, eingabe: TaskEingabe): TaskDto {
        aufgaben += placeId to eingabe
        return TaskDto(id = aufgaben.size.toLong(), placeId = placeId, kind = eingabe.kind)
    }

    override suspend fun aufgabeAendern(id: Long, eingabe: TaskEingabe): TaskDto {
        aufgaben += id to eingabe
        return TaskDto(id = id, kind = eingabe.kind)
    }

    override suspend fun aufgabeLoeschen(id: Long) {
        geloeschteAufgaben += id
    }
}
