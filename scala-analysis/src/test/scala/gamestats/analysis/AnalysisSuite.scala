package gamestats.analysis

import gamestats.model.{Event, EventTypes}
import io.circe.Json

import java.time.Instant

/** Pruebas unitarias de Statistics y Rules, con eventos armados a mano
  * (no se necesita levantar el servidor ni la base de datos).
  */
class AnalysisSuite extends munit.FunSuite {

  // --- Helpers para armar eventos de prueba ---

  private def event(tipo: String, playerId: Option[String], data: (String, Json)*): Event =
    Event(
      eventId = "evt-test",
      timestamp = Instant.parse("2026-09-25T14:32:10Z"),
      gameId = "racing-circuit",
      gameVersion = "1.0",
      matchId = "match-test",
      playerId = playerId,
      `type` = tipo,
      action = None,
      data = data.toMap
    )

  private def lap(player: String, lapNumber: Int, lapTimeMs: Long, position: Int): Event =
    event(
      EventTypes.LapCompleted,
      Some(player),
      "lapNumber" -> Json.fromInt(lapNumber),
      "lapTimeMs" -> Json.fromLong(lapTimeMs),
      "position"  -> Json.fromInt(position)
    )

  private def positionChanged(player: String, from: Int, to: Int, lapNumber: Int): Event =
    event(
      EventTypes.PositionChanged,
      Some(player),
      "fromPosition" -> Json.fromInt(from),
      "toPosition"   -> Json.fromInt(to),
      "lapNumber"    -> Json.fromInt(lapNumber)
    )

  private def penalty(player: String, timeMs: Long): Event =
    event(
      EventTypes.PenaltyApplied,
      Some(player),
      "reason" -> Json.fromString("corte de pista"),
      "timeMs" -> Json.fromLong(timeMs)
    )

  private def finished(standings: (String, Int)*): Event =
    event(
      EventTypes.MatchFinished,
      None,
      "finalStandings" -> Json.arr(standings.map { case (p, pos) =>
        Json.obj("playerId" -> Json.fromString(p), "finalPosition" -> Json.fromInt(pos))
      }: _*)
    )
  
  private def pitStop(player: String, durationMs: Long, lapNumber: Int): Event =
    event(
      EventTypes.PitStop,
      Some(player),
      "durationMs" -> Json.fromLong(durationMs),
      "lapNumber"  -> Json.fromInt(lapNumber)
    )

  private def incident(player: String): Event =
    event(
      EventTypes.RaceIncident,
      Some(player),
      "incidentType" -> Json.fromString("choque"),
      "severity"     -> Json.fromInt(2)
    )

  // --- Statistics ---

  test("lapEvents deja solo los eventos LAP_COMPLETED") {
    val events = List(lap("p1", 1, 90000L, 1), penalty("p1", 2000L), positionChanged("p1", 2, 1, 1))
    assertEquals(Statistics.lapEvents(events).size, 1)
  }

  test("playerStats calcula mejor vuelta, promedio y penalizaciones por jugador") {
    val events = List(
      lap("p1", 1, 90000L, 1),
      lap("p1", 2, 92000L, 1),
      penalty("p1", 2000L),
      penalty("p1", 3000L),
      lap("p2", 1, 95000L, 2),
      finished("p1" -> 1, "p2" -> 2)
    )
    val stats = Statistics.playerStats(events)
    val p1    = stats.find(_.playerId == "p1").get
    val p2    = stats.find(_.playerId == "p2").get

    assertEquals(p1.bestLapMs, Some(90000L))
    assertEquals(p1.avgLapMs, Some(91000.0))
    assertEquals(p1.totalPenaltyMs, 5000L)
    assertEquals(p1.penaltyCount, 2)
    assertEquals(p1.finalPosition, Some(1))

    assertEquals(p2.bestLapMs, Some(95000L))
    assertEquals(p2.penaltyCount, 0)
    assertEquals(p2.totalPenaltyMs, 0L)
    assertEquals(p2.finalPosition, Some(2))
  }

  test("matchStats obtiene la vuelta más rápida de la carrera y quién la hizo") {
    val events = List(
      lap("p1", 1, 90000L, 2),
      lap("p1", 2, 92000L, 2),
      lap("p2", 1, 89000L, 1),
      lap("p2", 2, 95000L, 1)
    )
    val stats = Statistics.matchStats("match-test", events)

    assertEquals(stats.fastestLapMs, Some(89000L))
    assertEquals(stats.fastestLapPlayer, Some("p2"))
    assertEquals(stats.avgLapMsAllPlayers, Some(91500.0))
    assertEquals(stats.playerStats.size, 2)
  }

  test("matchStats sin vueltas deja las estadísticas de vuelta vacías") {
    val stats = Statistics.matchStats("match-test", List(event(EventTypes.MatchStarted, None)))

    assertEquals(stats.fastestLapMs, None)
    assertEquals(stats.fastestLapPlayer, None)
    assertEquals(stats.avgLapMsAllPlayers, None)
  }

  test("finalStandings lee la posición final de cada jugador") {
    val events = List(lap("p1", 1, 90000L, 1), finished("p1" -> 1, "p2" -> 2))
    assertEquals(Statistics.finalStandings(events), Map("p1" -> 1, "p2" -> 2))
  }

  test("finalStandings queda vacío si no hay MATCH_FINISHED") {
    val events = List(lap("p1", 1, 90000L, 1))
    assertEquals(Statistics.finalStandings(events), Map.empty[String, Int])
  }

  test("positionEvolution combina LAP_COMPLETED y POSITION_CHANGED por vuelta") {
    val events = List(
      lap("p1", 1, 90000L, 4),
      lap("p1", 2, 91000L, 3),
      positionChanged("p1", 3, 2, 3)
    )
    val evolution = Statistics.positionEvolution(events)
    assertEquals(evolution("p1"), List((1, 4), (2, 3), (3, 2)))
  }

  test("playerStats cuenta pit stops, tiempo en pits e incidentes") {
    val events = List(
      pitStop("p1", 2500L, 2),
      pitStop("p1", 3000L, 4),
      incident("p1"),
      lap("p2", 1, 95000L, 1)
    )
    val stats = Statistics.playerStats(events)
    val p1    = stats.find(_.playerId == "p1").get
    val p2    = stats.find(_.playerId == "p2").get

    assertEquals(p1.pitStopCount, 2)
    assertEquals(p1.totalPitMs, 5500L)
    assertEquals(p1.incidentCount, 1)
    assertEquals(p2.pitStopCount, 0)
    assertEquals(p2.incidentCount, 0)
  }

  // --- Rules (remontada) ---

  test("el umbral de remontada coincide con docs/event-spec.md") {
    assertEquals(Rules.ComebackThreshold, 3)
  }

  test("detectComebacks detecta una remontada que justo alcanza el umbral") {
    val events = List(
      lap("p1", 1, 90000L, 4),
      positionChanged("p1", 4, 3, 1),
      positionChanged("p1", 3, 2, 2),
      positionChanged("p1", 2, 1, 3),
      finished("p1" -> 1)
    )
    assertEquals(Rules.detectComebacks(events), List(Rules.ComebackResult("p1", 4, 1, 3)))
  }

  test("detectComebacks no marca remontada si no llega al umbral") {
    val events = List(
      lap("p1", 1, 90000L, 3),
      positionChanged("p1", 3, 1, 1),
      finished("p1" -> 1)
    )
    assertEquals(Rules.detectComebacks(events), List.empty[Rules.ComebackResult])
  }

  test("detectComebacks ordena primero a quien ganó más posiciones") {
    val events = List(
      lap("p1", 1, 90000L, 5),
      lap("p2", 1, 91000L, 5),
      finished("p1" -> 1, "p2" -> 2)
    )
    assertEquals(
      Rules.detectComebacks(events),
      List(Rules.ComebackResult("p1", 5, 1, 4), Rules.ComebackResult("p2", 5, 2, 3))
    )
  }
}