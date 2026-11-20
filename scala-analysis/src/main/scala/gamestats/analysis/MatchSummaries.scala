package gamestats.analysis

import gamestats.model.Event
import io.circe.Encoder
import io.circe.generic.semiauto._

import java.time.Duration

/** Resumen genérico de una partida, válido para cualquier juego distinto
  * del de carreras (que conserva su análisis propio en Statistics/Rules).
  *
  * Todo el cálculo son funciones puras sobre la lista de eventos: no hay
  * estado mutable ni acceso a la base de datos aquí. Se apoya únicamente en
  * los eventos de ciclo de vida y de juego descritos en docs/event-spec.md:
  *
  *   - MATCH_STARTED            modo de juego (data.mode)
  *   - PLAYER_ACTION / MOVE     acciones del jugador
  *   - AI_DECISION              decisiones de la computadora
  *   - PLAYER_WON / PLAYER_LOST resultado desde el punto de vista del jugador
  *   - MATCH_FINISHED           resultado final (data.outcome, data.score, data.winner)
  */
object MatchSummaries {

  /** Resultados posibles de una partida. */
  object Outcome {
    val Won       = "WON"
    val Lost      = "LOST"
    val Draw      = "DRAW"
    val Abandoned = "ABANDONED"
    val Unknown   = "UNKNOWN"
  }

  final case class MatchSummary(
      matchId: String,
      gameId: String,
      mode: String,
      outcome: String,
      winner: Option[String],
      players: List[String],
      durationMs: Long,
      moves: Int,
      aiDecisions: Int,
      score: Option[Long]
  )
  object MatchSummary {
    implicit val encoder: Encoder[MatchSummary] = deriveEncoder
  }

  private val ActionTypes = Set("PLAYER_ACTION", "MOVE")

  /** Resume una partida. Devuelve None si no hay eventos. */
  def summarize(matchId: String, events: List[Event]): Option[MatchSummary] =
    events.headOption.map { first =>
      val last     = events.last
      val started  = events.find(_.`type` == "MATCH_STARTED")
      val finished = events.find(_.`type` == "MATCH_FINISHED")

      val mode = started
        .flatMap(_.data.get("mode"))
        .flatMap(_.asString)
        .getOrElse("solo")

      val outcome = finished
        .flatMap(_.data.get("outcome"))
        .flatMap(_.asString)
        .map(_.toUpperCase)
        .orElse(outcomeFromResultEvents(events))
        .getOrElse(Outcome.Unknown)

      val winner = finished
        .flatMap(_.data.get("winner"))
        .flatMap(_.asString)

      val score = finished
        .flatMap(_.data.get("score"))
        .flatMap(_.asNumber)
        .flatMap(_.toLong)

      MatchSummary(
        matchId = matchId,
        gameId = first.gameId,
        mode = mode,
        outcome = outcome,
        winner = winner,
        players = events.flatMap(_.playerId).distinct,
        durationMs = Duration.between(first.timestamp, last.timestamp).toMillis,
        moves = events.count(e => ActionTypes.contains(e.`type`)),
        aiDecisions = events.count(_.`type` == "AI_DECISION"),
        score = score
      )
    }

  private def outcomeFromResultEvents(events: List[Event]): Option[String] =
    events.collectFirst {
      case e if e.`type` == "PLAYER_WON"  => Outcome.Won
      case e if e.`type` == "PLAYER_LOST" => Outcome.Lost
    }
}
