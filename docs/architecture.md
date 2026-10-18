# Arquitectura de GameStats

## 1. Visión general

GameStats separa la **generación** de eventos de su **análisis**. El videojuego
(simulado) produce eventos; el módulo Go los recibe, valida y agrupa por
partida; el módulo Scala los convierte en estadísticas y reglas; los resultados
se guardan y se pueden consultar.

```
                         HTTP/JSON                       HTTP/JSON
 ┌──────────────┐   POST /events   ┌────────────────┐  POST /analyze  ┌─────────────────┐
 │  Fuente      │ ───────────────► │  go-ingestion  │ ──────────────► │ scala-analysis  │
 │  de eventos  │                  │  (Go, :8080)   │  lote de UNA    │ (Scala, :8081)  │
 │  (simulador, │                  │                │  partida        │                 │
 │  JSON, curl) │                  │ 1. valida      │                 │ 1. estadísticas │
 └──────────────┘                  │ 2. agrupa por  │ ◄────────────── │ 2. reglas       │
                                   │    partida     │  JSON resultado │ 3. guarda       │
                                   │ 3. concurrencia│                 │    (SQLite)     │
                                   └────────────────┘                 └────────┬────────┘
                                                                               │
                                              GET /results, /results/{matchId} ▼
                                                                         quien consulta
```

Cada módulo es un servicio independiente con su propio contenedor
(`docker-compose.yml`). El único contrato entre ellos es el formato de
intercambio de `docs/event-spec.md` (sección 7), de modo que una fuente externa
puede producir eventos sin conocer la implementación interna.

## 2. Flujo de un evento

1. La fuente envía `POST /events` con un evento o un arreglo de eventos.
2. **Go valida** la estructura mínima (`internal/validate`). Los inválidos se
   registran en el log y se informan en la respuesta (`rejected`); el resto se
   acepta. Si hubo rechazados la respuesta es `207`; si no, `202`.
3. Cada evento válido se entrega con `Manager.Dispatch` al **worker de su
   partida** (`matchId`), que lo acumula en memoria.
4. Al llegar `MATCH_FINISHED`, el worker ordena los eventos por `timestamp`,
   arma el lote `{matchId, events}` y lo envía a Scala (`POST /analyze`).
5. **Scala analiza** el lote con colecciones funcionales, **guarda** el
   resultado en SQLite y lo devuelve a Go, que lo registra en su log.
6. Cualquier cliente puede consultar el resultado después con
   `GET /results` y `GET /results/{matchId}`, aunque los contenedores se hayan
   reiniciado (la base vive en un volumen de Docker).

## 3. Módulo Go: ingestión y control del flujo

| Responsabilidad del enunciado | Dónde está |
|---|---|
| Recibir eventos de una fuente externa | `cmd/server/main.go` (`POST /events`) |
| Validar la estructura básica | `internal/validate/validator.go` |
| Administrar el flujo de procesamiento | `internal/match/manager.go` |
| Mantener estado para reconstruir partidas | `matchWorker.events` (eventos acumulados por partida) |
| Procesamiento concurrente | una goroutine + un channel por partida |
| Preparar y enviar datos a Scala | `internal/client/scala_client.go` |
| Fuente simulada | `cmd/simulator` (3 partidas en paralelo) |

### Modelo de concurrencia

- `net/http` atiende cada request en su propia goroutine, así que varias
  fuentes pueden enviar a la vez.
- `Manager` mantiene un `map[matchID]*matchWorker` protegido por
  `sync.RWMutex` (con doble verificación al crear un worker nuevo).
- Cada worker es **una goroutine que consume un `chan model.Event` con buffer
  de 128**. Así los eventos de una partida se procesan en serie (sin carreras
  sobre su estado) y las partidas distintas avanzan en paralelo sin bloquearse.
- El buffer actúa como contrapresión: si una partida se satura, `Dispatch`
  espera en lugar de perder eventos.
- Al terminar la partida, el worker envía el lote, se elimina del mapa y su
  channel se cierra, liberando los recursos.
- Los tests se ejecutan con el detector de carreras (`go test -race`).

## 4. Módulo Scala: análisis funcional

El análisis es una composición de funciones puras sobre `List[Event]`
(`analysis/Statistics.scala`, `analysis/Rules.scala`):

| Operación | Uso |
|---|---|
| `filter` | quedarse solo con `LAP_COMPLETED`, `PENALTY_APPLIED`, etc. |
| `map` / `flatMap` | extraer tiempos y posiciones desde el campo `data` (con `Option`) |
| `groupBy` | agrupar eventos por jugador |
| `foldLeft` | sumar y promediar tiempos, penalizaciones y pits |
| `collectFirst` | encontrar quién hizo la vuelta más rápida |
| `for`-comprehension sobre `Option` | regla de remontada (peor posición y posición final) |
| `sortBy` | ordenar remontadas por posiciones ganadas |

Las funciones no tienen estado ni efectos: reciben eventos y devuelven valores,
lo que permite probarlas con eventos armados a mano (`AnalysisSuite`). Los
únicos efectos (servidor HTTP y SQLite) están aislados en `Main.scala` y
`Storage.scala`. El resultado se serializa a JSON con circe.

## 5. Decisiones de diseño y justificación

| Decisión | Alternativas | Por qué |
|---|---|---|
| **HTTP/REST + JSON** entre módulos | cola de mensajes, archivos | Es simple de ejecutar y depurar (curl), no exige infraestructura adicional y define un contrato claro y documentado. |
| **Go envía un lote por partida terminada** | enviar evento por evento | Las estadísticas y la regla de remontada necesitan la secuencia completa; Scala recibe todo ordenado y sin estado compartido. |
| **Una goroutine por partida** | pool fijo de workers, un solo lock global | El estado de cada partida es independiente: serializarlo en su goroutine evita locks por evento y refleja el modelo "partidas en paralelo". |
| **SQLite en un volumen** | PostgreSQL, archivos JSON | Cero administración, suficiente para el volumen del proyecto y persistente entre reinicios. |
| **`data` abierto (`Map[String, Json]`)** | clases por tipo de evento | El sobre del evento no depende del juego; nuevos tipos no obligan a cambiar el parseo ni a Go. |
| **Docker Compose + GHCR** | VM manual, Kubernetes | Reproducible, con un comando, y las imágenes las publica el CI. |

## 6. Imperativo (Go) frente a funcional (Scala)

**Lo que se resolvió mejor de forma imperativa en Go**

- Orquestación y control del flujo: recibir, validar, decidir cuándo una
  partida está lista y enviarla. Es un problema de *procesos y tiempo*.
- Concurrencia con estado mutable acotado: goroutines, channels y un mapa
  protegido por mutex expresan directamente "una partida = un worker".
- Entrada/salida (HTTP) con manejo explícito de errores (`if err != nil`) y
  binarios pequeños y rápidos de arrancar.

**Lo que se benefició del enfoque funcional en Scala**

- Transformar colecciones: cada estadística es una tubería de
  `filter → groupBy → map → fold`, corta y declarativa.
- Datos que pueden faltar: `Option` y `for`-comprehensions hacen explícito qué
  pasa si un evento no trae `lapTimeMs` o si no hay `MATCH_FINISHED`, sin
  `null` ni excepciones.
- Pruebas: al ser funciones puras, se prueban con listas de eventos, sin
  servidor ni base de datos.
- Inmutabilidad: no hay riesgo de que un análisis modifique los eventos de otro.

**Limitaciones de cada enfoque**

- Go: más código repetitivo para transformar datos (bucles y mapas) y sin
  tipos suma; el manejo de JSON dinámico (`map[string]interface{}`) es verboso.
- Scala: arranque y compilación más pesados (JVM, sbt), y el análisis por
  lotes completos usa memoria proporcional al tamaño de la partida.

**Un sistema con ambos** permite usar cada lenguaje donde es más natural y
desplegarlos como servicios independientes con un contrato JSON entre ellos.

## 7. Limitaciones conocidas y trabajo futuro

Se documentan para que sean decisiones conscientes y no sorpresas:

- **Una partida que nunca recibe `MATCH_FINISHED` no se analiza** y su worker
  permanece en memoria (no hay temporizador de inactividad). Mejora: cerrar
  partidas inactivas tras un tiempo y analizarlas como "incompletas".
- **Si Scala no está disponible al terminar una partida**, Go registra el
  error y el lote no se reintenta. Mejora: reintentos con espera y/o cola
  persistente.
- **Eventos tardíos** (llegan después de `MATCH_FINISHED`) crean una partida
  nueva con el mismo `matchId`; hoy no se descartan ni se marcan.
- **El análisis de Scala es específico de carreras**: el sobre del evento, la
  validación de campos comunes, la agrupación, la comunicación y la
  persistencia son independientes del juego, pero `Statistics`/`Rules` calculan
  métricas de carreras. Para otro juego se agrega su propio análisis (ver
  `docs/game-design.md`, sección 4). Del lado de Go, el mapa `playerRequired`
  también lista tipos de carreras.
- `Statistics.positionEvolution` (evolución de posición por vuelta, combinando
  `LAP_COMPLETED` y `POSITION_CHANGED`) está implementada y probada, pero **aún
  no se incluye en el JSON de `/analyze`**.
- El servidor HTTP de Scala usa el ejecutor por defecto del JDK (un solo hilo):
  suficiente para la demostración, no para alta carga.
- No hay autenticación en las APIs: están pensadas para una red interna; en el
  despliegue el puerto 8081 se publica solo en `127.0.0.1` por defecto.
