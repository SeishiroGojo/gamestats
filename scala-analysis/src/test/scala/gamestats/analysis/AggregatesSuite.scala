package gamestats.analysis

import gamestats.analysis.MatchSummaries.{MatchSummary, Outcome}
import gamestats.model.Event
import io.circe.Json

import java.time.Instant

/** Pruebas del resumen genérico de partidas y de las estadísticas agregadas
  * (por juego y generales) que consulta el panel de GameConsole.
  */
class AggregatesSuite extends munit.FunSuite {

  private val t0 = Instant.parse("2026-11-10T15:00:00Z")

  private def ev(
      tipo: String,
      secs: Long,
      player: Option[String] = None,
      game: String = "connect4",
      data: Map[String, Json] = Map.empty
  ): Event =
    Event(
      eventId = s"e-$tipo-$secs",
      timestamp = t0.plusSeconds(secs),
      gameId = game,
      gameVersion = "1.0",
      matchId = "m-1",
      playerId = player,
      `type` = tipo,
      action = None,
      data = data
    )

  private def summary(
      game: String,
      outcome: String,
      durationMs: Long = 1000L,
      moves: Int = 10,
      ai: Int = 0,
      score: Option[Long] = None
  ): MatchSummary =
    MatchSummary("m", game, "solo", outcome, None, Nil, durationMs, moves, ai, score)

  test("summarize: partida de Conecta 4 ganada por la persona") {
    val events = List(
      ev("MATCH_STARTED", 0, Some("human"), data = Map("mode" -> Json.fromString("vs_ai"))),
      ev("PLAYER_ACTION", 2, Some("human")),
      ev("AI_DECISION", 3, Some("prolog-ai")),
      ev("PLAYER_ACTION", 5, Some("human")),
      ev("AI_DECISION", 6, Some("prolog-ai")),
      ev("PLAYER_WON", 20, Some("human")),
      ev(
        "MATCH_FINISHED",
        21,
        Some("human"),
        data = Map("outcome" -> Json.fromString("won"), "winner" -> Json.fromString("human"))
      )
    )
    val s = MatchSummaries.summarize("m-1", events).get
    assertEquals(s.gameId, "connect4")
    assertEquals(s.mode, "vs_ai")
    assertEquals(s.outcome, Outcome.Won)
    assertEquals(s.winner, Some("human"))
    assertEquals(s.moves, 2)
    assertEquals(s.aiDecisions, 2)
    assertEquals(s.durationMs, 21000L)
    assertEquals(s.players, List("human", "prolog-ai"))
    assertEquals(s.score, None)
  }

  test("summarize: toma el puntaje de MATCH_FINISHED (2048)") {
    val events = List(
      ev("MATCH_STARTED", 0, Some("p1"), game = "game-2048"),
      ev("MOVE", 1, Some("p1"), game = "game-2048"),
      ev(
        "MATCH_FINISHED",
        9,
        Some("p1"),
        game = "game-2048",
        data = Map("outcome" -> Json.fromString("LOST"), "score" -> Json.fromInt(2048))
      )
    )
    val s = MatchSummaries.summarize("m-1", events).get
    assertEquals(s.mode, "solo") // sin data.mode se asume "solo"
    assertEquals(s.outcome, Outcome.Lost)
    assertEquals(s.score, Some(2048L))
    assertEquals(s.moves, 1)
  }

  test("summarize: sin data.outcome usa PLAYER_WON / PLAYER_LOST") {
    val won = List(ev("MATCH_STARTED", 0), ev("PLAYER_WON", 3), ev("MATCH_FINISHED", 4))
    val lost = List(ev("MATCH_STARTED", 0), ev("PLAYER_LOST", 3), ev("MATCH_FINISHED", 4))
    val none = List(ev("MATCH_STARTED", 0), ev("MATCH_FINISHED", 4))
    assertEquals(MatchSummaries.summarize("m", won).get.outcome, Outcome.Won)
    assertEquals(MatchSummaries.summarize("m", lost).get.outcome, Outcome.Lost)
    assertEquals(MatchSummaries.summarize("m", none).get.outcome, Outcome.Unknown)
  }

  test("summarize: lista vacía no produce resumen") {
    assertEquals(MatchSummaries.summarize("m", Nil), None)
  }

  test("byGame: agrupa por juego y calcula conteos, promedios y tasa de victorias") {
    val ss = List(
      summary("connect4", Outcome.Won, durationMs = 20000, moves = 8, ai = 8),
      summary("connect4", Outcome.Lost, durationMs = 40000, moves = 12, ai = 12),
      summary("connect4", Outcome.Draw, durationMs = 30000, moves = 10, ai = 10),
      summary("connect4", Outcome.Abandoned, durationMs = 10000, moves = 2, ai = 2),
      summary("game-2048", Outcome.Won, durationMs = 1000, moves = 100, score = Some(2048)),
      summary("game-2048", Outcome.Lost, durationMs = 3000, moves = 50, score = Some(512))
    )
    val rows = Aggregates.byGame(ss)
    assertEquals(rows.map(_.gameId), List("connect4", "game-2048"))

    val c4 = rows.head
    assertEquals(c4.matches, 4)
    assertEquals((c4.won, c4.lost, c4.draws, c4.abandoned), (1, 1, 1, 1))
    assertEquals(c4.winRate, 1.0 / 3.0) // 1 victoria de 3 partidas completadas
    assertEquals(c4.avgDurationMs, 25000.0)
    assertEquals(c4.avgMoves, 8.0)
    assertEquals(c4.avgAiDecisions, 8.0)
    assertEquals(c4.avgScore, None)

    val g = rows(1)
    assertEquals(g.matches, 2)
    assertEquals(g.avgScore, Some(1280.0))
    assertEquals(g.bestScore, Some(2048L))
    assertEquals(g.winRate, 0.5)
  }

  test("byGame: sin partidas devuelve lista vacía") {
    assertEquals(Aggregates.byGame(Nil), Nil)
  }

  test("overall: totales sobre todos los juegos") {
    val ss = List(
      summary("connect4", Outcome.Won, durationMs = 2000, moves = 5, ai = 5),
      summary("game-2048", Outcome.Lost, durationMs = 4000, moves = 7),
      summary("minesweeper", Outcome.Abandoned, durationMs = 6000, moves = 3)
    )
    val o = Aggregates.overall(ss)
    assertEquals(o.games, 3)
    assertEquals(o.matches, 3)
    assertEquals((o.won, o.lost, o.draws, o.abandoned), (1, 1, 0, 1))
    assertEquals(o.winRate, 0.5) // 1 de 2 completadas
    assertEquals(o.avgDurationMs, 4000.0)
    assertEquals(o.totalMoves, 15)
    assertEquals(o.totalAiDecisions, 5)
  }

  test("overall: sin partidas no divide por cero") {
    val o = Aggregates.overall(Nil)
    assertEquals(o.matches, 0)
    assertEquals(o.winRate, 0.0)
    assertEquals(o.avgDurationMs, 0.0)
  }
}
