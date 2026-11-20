package gamestats.analysis

import gamestats.analysis.MatchSummaries.{MatchSummary, Outcome}
import io.circe.Encoder
import io.circe.generic.semiauto._

/** Estadísticas agregadas: por juego y generales. Funciones puras que
  * reciben los resúmenes ya calculados de las partidas terminadas.
  * Es lo que consulta el panel de GameConsole (GET /stats/games y
  * GET /stats/overall); la consola no recalcula nada de esto.
  */
object Aggregates {

  final case class GameAggregate(
      gameId: String,
      matches: Int,
      won: Int,
      lost: Int,
      draws: Int,
      abandoned: Int,
      winRate: Double,
      avgDurationMs: Double,
      avgMoves: Double,
      avgAiDecisions: Double,
      avgScore: Option[Double],
      bestScore: Option[Long]
  )
  object GameAggregate {
    implicit val encoder: Encoder[GameAggregate] = deriveEncoder
  }

  final case class OverallAggregate(
      games: Int,
      matches: Int,
      won: Int,
      lost: Int,
      draws: Int,
      abandoned: Int,
      winRate: Double,
      avgDurationMs: Double,
      totalMoves: Int,
      totalAiDecisions: Int
  )
  object OverallAggregate {
    implicit val encoder: Encoder[OverallAggregate] = deriveEncoder
  }

  private def ratio(num: Double, den: Double): Double =
    if (den == 0) 0.0 else num / den

  private def count(ss: List[MatchSummary], outcome: String): Int =
    ss.count(_.outcome == outcome)

  /** Tasa de victorias sobre las partidas que se completaron
    * (las abandonadas no cuentan ni a favor ni en contra).
    */
  private def winRate(ss: List[MatchSummary]): Double = {
    val completed = ss.count(s => s.outcome != Outcome.Abandoned)
    ratio(count(ss, Outcome.Won).toDouble, completed.toDouble)
  }

  /** Una fila por juego, ordenadas por gameId. */
  def byGame(summaries: List[MatchSummary]): List[GameAggregate] =
    summaries
      .groupBy(_.gameId)
      .toList
      .sortBy(_._1)
      .map { case (gameId, ss) =>
        val n      = ss.size.toDouble
        val scores = ss.flatMap(_.score)
        GameAggregate(
          gameId = gameId,
          matches = ss.size,
          won = count(ss, Outcome.Won),
          lost = count(ss, Outcome.Lost),
          draws = count(ss, Outcome.Draw),
          abandoned = count(ss, Outcome.Abandoned),
          winRate = winRate(ss),
          avgDurationMs = ratio(ss.map(_.durationMs).sum.toDouble, n),
          avgMoves = ratio(ss.map(_.moves).sum.toDouble, n),
          avgAiDecisions = ratio(ss.map(_.aiDecisions).sum.toDouble, n),
          avgScore = if (scores.isEmpty) None else Some(scores.sum.toDouble / scores.size),
          bestScore = if (scores.isEmpty) None else Some(scores.max)
        )
      }

  /** Totales sobre todos los juegos. */
  def overall(summaries: List[MatchSummary]): OverallAggregate =
    OverallAggregate(
      games = summaries.map(_.gameId).distinct.size,
      matches = summaries.size,
      won = count(summaries, Outcome.Won),
      lost = count(summaries, Outcome.Lost),
      draws = count(summaries, Outcome.Draw),
      abandoned = count(summaries, Outcome.Abandoned),
      winRate = winRate(summaries),
      avgDurationMs = ratio(summaries.map(_.durationMs).sum.toDouble, summaries.size.toDouble),
      totalMoves = summaries.map(_.moves).sum,
      totalAiDecisions = summaries.map(_.aiDecisions).sum
    )
}
