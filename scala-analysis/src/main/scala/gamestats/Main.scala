package gamestats

import com.sun.net.httpserver.{HttpExchange, HttpHandler, HttpServer}
import gamestats.analysis.{Rules, Statistics, Storage}
import gamestats.model.MatchBatch
import io.circe.generic.semiauto._
import io.circe.parser.decode
import io.circe.syntax._
import io.circe.Encoder

import java.io.OutputStream
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import scala.io.Source
import scala.util.{Failure, Success, Try}

/** Módulo de análisis de GameStats.
  *
  * Recibe (POST /analyze) el lote de eventos de una partida terminada,
  * enviado por el módulo Go, calcula estadísticas y reglas, guarda el
  * resultado en SQLite, y permite consultarlo después con:
  *   - GET /results             -> lista de partidas analizadas
  *   - GET /results/{matchId}   -> resultado completo de una partida
  */
object Main {

  final case class AnalysisResult(
      matchStats: Statistics.MatchStats,
      comebacks: List[Rules.ComebackResult]
  )
  object AnalysisResult {
    implicit val encoder: Encoder[AnalysisResult] = deriveEncoder
  }

  final case class ResultSummaryView(matchId: String, createdAt: String)
  object ResultSummaryView {
    implicit val encoder: Encoder[ResultSummaryView] = deriveEncoder
  }

  def main(args: Array[String]): Unit = {
    val port   = sys.env.get("PORT").flatMap(p => Try(p.toInt).toOption).getOrElse(8081)
    val dbPath = sys.env.getOrElse("DB_PATH", "data/gamestats.db")

    Storage.init(dbPath)
    println(s"Base de datos lista en $dbPath")

    val server = HttpServer.create(new InetSocketAddress(port), 0)
    server.createContext("/analyze", analyzeHandler)
    server.createContext("/results", resultsHandler)
    server.createContext("/health", healthHandler)
    server.setExecutor(null) // executor por defecto de un solo hilo; suficiente para la demo
    server.start()
    println(s"GameStats analysis escuchando en :$port")
  }

  private val analyzeHandler: HttpHandler = (exchange: HttpExchange) => {
    if (exchange.getRequestMethod != "POST") {
      respond(exchange, 405, """{"error":"método no permitido"}""")
    } else {
      val body = Source.fromInputStream(exchange.getRequestBody, StandardCharsets.UTF_8.name()).mkString

      decode[MatchBatch](body) match {
        case Left(error) =>
          respond(exchange, 400, s"""{"error":"JSON inválido: ${error.getMessage.replace("\"", "'")}"}""")
        case Right(batch) =>
          val stats     = Statistics.matchStats(batch.matchId, batch.events)
          val comebacks = Rules.detectComebacks(batch.events)
          val result    = AnalysisResult(stats, comebacks)
          val resultJson = result.asJson.noSpaces

          Try(Storage.save(batch.matchId, resultJson)) match {
            case Success(_) => ()
            case Failure(e) => System.err.println(s"error guardando resultado de ${batch.matchId}: ${e.getMessage}")
          }

          respond(exchange, 200, resultJson)
      }
    }
  }

  /** Maneja tanto GET /results como GET /results/{matchId}, según si el
    * path trae algo después de "/results/".
    */
  private val resultsHandler: HttpHandler = (exchange: HttpExchange) => {
    if (exchange.getRequestMethod != "GET") {
      respond(exchange, 405, """{"error":"método no permitido"}""")
    } else {
      val path   = exchange.getRequestURI.getPath
      val prefix = "/results/"

      if (path.startsWith(prefix) && path.length > prefix.length) {
        val matchId = path.substring(prefix.length)
        Try(Storage.findByMatchId(matchId)) match {
          case Success(Some(json)) => respond(exchange, 200, json)
          case Success(None)       => respond(exchange, 404, s"""{"error":"no hay resultado guardado para $matchId"}""")
          case Failure(e)          => respond(exchange, 500, s"""{"error":"error consultando: ${e.getMessage}"}""")
        }
      } else {
        Try(Storage.listAll()) match {
          case Success(summaries) =>
            val view = summaries.map(s => ResultSummaryView(s.matchId, s.createdAt))
            respond(exchange, 200, view.asJson.noSpaces)
          case Failure(e) =>
            respond(exchange, 500, s"""{"error":"error listando resultados: ${e.getMessage}"}""")
        }
      }
    }
  }

  private val healthHandler: HttpHandler = (exchange: HttpExchange) => {
    respond(exchange, 200, """{"status":"ok"}""")
  }

  private def respond(exchange: HttpExchange, status: Int, body: String): Unit = {
    val bytes = body.getBytes(StandardCharsets.UTF_8)
    exchange.getResponseHeaders.add("Content-Type", "application/json")
    exchange.sendResponseHeaders(status, bytes.length.toLong)
    val os: OutputStream = exchange.getResponseBody
    Try(os.write(bytes)) match {
      case Success(_) => ()
      case Failure(e) => System.err.println(s"error escribiendo respuesta: ${e.getMessage}")
    }
    os.close()
  }
}