package gamestats.analysis

import java.io.File
import java.sql.{Connection, DriverManager}

/** Persistencia simple de resultados de análisis, usando SQLite (un
  * archivo, sin necesidad de instalar ni levantar un servidor de base de
  * datos aparte). Guarda el JSON completo que ya calculó Statistics/Rules,
  * indexado por matchId, para poder consultarlo más tarde.
  */
object Storage {

  final case class ResultSummary(matchId: String, createdAt: String)

  private var jdbcUrl: String = "jdbc:sqlite:data/gamestats.db"

  /** Debe llamarse una vez al arrancar el servidor. Crea el archivo y la
    * tabla si todavía no existen.
    */
  def init(dbPath: String): Unit = {
    val parent = new File(dbPath).getParentFile
    if (parent != null) parent.mkdirs()

    jdbcUrl = s"jdbc:sqlite:$dbPath"

    withConnection { conn =>
      val stmt = conn.createStatement()
      try {
        stmt.execute(
          """CREATE TABLE IF NOT EXISTS match_results (
            |  match_id     TEXT PRIMARY KEY,
            |  result_json  TEXT NOT NULL,
            |  created_at   TEXT NOT NULL DEFAULT (datetime('now','localtime'))
            |)""".stripMargin
        )
        stmt.execute(
          """CREATE TABLE IF NOT EXISTS match_summaries (
            |  match_id     TEXT PRIMARY KEY,
            |  game_id      TEXT NOT NULL,
            |  mode         TEXT NOT NULL,
            |  outcome      TEXT NOT NULL,
            |  winner       TEXT,
            |  players      TEXT NOT NULL,
            |  duration_ms  INTEGER NOT NULL,
            |  moves        INTEGER NOT NULL,
            |  ai_decisions INTEGER NOT NULL,
            |  score        INTEGER,
            |  created_at   TEXT NOT NULL DEFAULT (datetime('now','localtime'))
            |)""".stripMargin
        )
      } finally stmt.close()
    }
  }

  /** Guarda (o reemplaza) el resumen de una partida de un juego genérico
    * (todos menos el de carreras), para poder agregar estadísticas por juego.
    */
  def saveSummary(s: MatchSummaries.MatchSummary): Unit = withConnection { conn =>
    val ps = conn.prepareStatement(
      """INSERT INTO match_summaries
        |  (match_id, game_id, mode, outcome, winner, players, duration_ms, moves, ai_decisions, score, created_at)
        |VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now','localtime'))
        |ON CONFLICT(match_id) DO UPDATE SET
        |  game_id = excluded.game_id, mode = excluded.mode, outcome = excluded.outcome,
        |  winner = excluded.winner, players = excluded.players, duration_ms = excluded.duration_ms,
        |  moves = excluded.moves, ai_decisions = excluded.ai_decisions, score = excluded.score,
        |  created_at = excluded.created_at
        |""".stripMargin
    )
    try {
      ps.setString(1, s.matchId)
      ps.setString(2, s.gameId)
      ps.setString(3, s.mode)
      ps.setString(4, s.outcome)
      ps.setString(5, s.winner.orNull)
      ps.setString(6, s.players.mkString(","))
      ps.setLong(7, s.durationMs)
      ps.setInt(8, s.moves)
      ps.setInt(9, s.aiDecisions)
      s.score match {
        case Some(v) => ps.setLong(10, v)
        case None    => ps.setNull(10, java.sql.Types.INTEGER)
      }
      ps.executeUpdate()
    } finally ps.close()
  }

  /** Todos los resúmenes guardados (de todos los juegos genéricos). */
  def listSummaries(): List[MatchSummaries.MatchSummary] = withConnection { conn =>
    val stmt = conn.createStatement()
    try {
      val rs = stmt.executeQuery(
        "SELECT match_id, game_id, mode, outcome, winner, players, duration_ms, moves, ai_decisions, score FROM match_summaries"
      )
      try {
        val buf = scala.collection.mutable.ListBuffer.empty[MatchSummaries.MatchSummary]
        while (rs.next()) {
          val score = rs.getLong("score")
          val scoreOpt = if (rs.wasNull()) None else Some(score)
          val playersRaw = rs.getString("players")
          buf += MatchSummaries.MatchSummary(
            matchId = rs.getString("match_id"),
            gameId = rs.getString("game_id"),
            mode = rs.getString("mode"),
            outcome = rs.getString("outcome"),
            winner = Option(rs.getString("winner")),
            players = if (playersRaw == null || playersRaw.isEmpty) Nil else playersRaw.split(",").toList,
            durationMs = rs.getLong("duration_ms"),
            moves = rs.getInt("moves"),
            aiDecisions = rs.getInt("ai_decisions"),
            score = scoreOpt
          )
        }
        buf.toList
      } finally rs.close()
    } finally stmt.close()
  }

  /** Guarda (o reemplaza, si ya existía) el resultado de una partida. */
  def save(matchId: String, resultJson: String): Unit = withConnection { conn =>
    val ps = conn.prepareStatement(
      """INSERT INTO match_results (match_id, result_json, created_at)
        |VALUES (?, ?, datetime('now','localtime'))
        |ON CONFLICT(match_id) DO UPDATE SET
        |  result_json = excluded.result_json,
        |  created_at  = excluded.created_at
        |""".stripMargin
    )
    try {
      ps.setString(1, matchId)
      ps.setString(2, resultJson)
      ps.executeUpdate()
    } finally ps.close()
  }

  /** Busca el resultado guardado de una partida puntual. */
  def findByMatchId(matchId: String): Option[String] = withConnection { conn =>
    val ps = conn.prepareStatement("SELECT result_json FROM match_results WHERE match_id = ?")
    try {
      ps.setString(1, matchId)
      val rs = ps.executeQuery()
      try {
        if (rs.next()) Some(rs.getString("result_json")) else None
      } finally rs.close()
    } finally ps.close()
  }

  /** Lista todas las partidas que ya tienen un resultado guardado, más
    * recientes primero.
    */
  def listAll(): List[ResultSummary] = withConnection { conn =>
    val stmt = conn.createStatement()
    try {
      val rs = stmt.executeQuery("SELECT match_id, created_at FROM match_results ORDER BY created_at DESC")
      try {
        val buf = scala.collection.mutable.ListBuffer.empty[ResultSummary]
        while (rs.next()) {
          buf += ResultSummary(rs.getString("match_id"), rs.getString("created_at"))
        }
        buf.toList
      } finally rs.close()
    } finally stmt.close()
  }

  private def withConnection[A](f: Connection => A): A = {
    Class.forName("org.sqlite.JDBC")
    val conn = DriverManager.getConnection(jdbcUrl)
    try f(conn) finally conn.close()
  }
}