package de.roessing.app

import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject

/**
 * The backend's test knobs under `/dev`. They exist only while the backend
 * runs with `AUTH_MODE=insecure-dev` — in production those paths are not
 * registered at all (see `backend/internal/devmode`).
 *
 * They are here so that no test has to wait. The assignment of care tasks is
 * a matter of days: a task falls due after its interval, and the background
 * timer then asks one helper after another. A test that sleeps until that
 * happens no longer claims "an overdue task gets its helper asked" but
 * "…within N seconds of wall clock" — which says nothing about the village
 * and everything about how busy the emulator is. That is what made the same
 * commit go green on one emulator and red on the other.
 *
 * So instead: move the clock to the day the task is due, run one assignment
 * pass, look at the result. No sleeping, same answer every time.
 *
 * Whoever moves the clock has to move it back — it belongs to the whole
 * backend process, and the next test would otherwise inherit a village
 * living in the future. Best done in an `@After`, so it also happens when
 * the test fails.
 */
class DevBackend(baseUrl: String = BuildConfig.API_BASE_URL) {
    private val base = baseUrl.trimEnd('/')
    private val client = OkHttpClient()
    private val json = "application/json".toMediaType()

    /** The village's local time — the backend's quiet hours are measured in it. */
    private val villageZone: ZoneId = ZoneId.of("Europe/Berlin")

    /** What time the backend currently thinks it is. */
    fun clock(): ZonedDateTime = parse(get("/dev/clock"))

    /**
     * Moves the clock [days] days ahead of where the backend stands now and
     * puts it at [hour] o'clock village time.
     *
     * The time of day is not a detail: between 21:00 and 07:00 the backend
     * delivers nothing (quiet hours — nobody's phone rings at night). A jump
     * that happened to land at night would create the assignment but defer
     * the request to the next morning, and the test would depend on what
     * time the machine happens to have. Mid-morning it always works.
     */
    fun travelForward(days: Long, hour: Int = 10): ZonedDateTime {
        val target = clock().plusDays(days).withHour(hour).withMinute(0).withSecond(0).withNano(0)
        return parse(post("/dev/clock/set", """{"time":"${DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(target)}"}"""))
    }

    /** Back to the system clock. Every test that travels has to come back. */
    fun resetClock() {
        post("/dev/clock/reset", "")
    }

    /**
     * Runs one assignment pass and only returns once the notifications are
     * written. It is the very same pass the background timer runs, it works
     * synchronously and it is repeatable at will — it only does something
     * when a task is really due.
     *
     * @return how many notifications this pass produced.
     */
    fun runAssignment(): Int = post("/dev/assignment/run", "").getInt("notifications")

    private fun parse(answer: JSONObject): ZonedDateTime =
        ZonedDateTime.parse(answer.getString("now")).withZoneSameInstant(villageZone)

    private fun get(path: String): JSONObject = call(Request.Builder().url(base + path).get().build(), path)

    private fun post(path: String, body: String): JSONObject =
        call(Request.Builder().url(base + path).post(body.toRequestBody(json)).build(), path)

    private fun call(request: Request, path: String): JSONObject {
        client.newCall(request).execute().use { response ->
            val text = response.body?.string().orEmpty()
            check(response.isSuccessful) {
                "$path: HTTP ${response.code} $text — läuft das Backend mit AUTH_MODE=insecure-dev?"
            }
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        }
    }
}
