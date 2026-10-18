# GameStats

![CI](https://github.com/SeishiroGojo/gamestats/actions/workflows/ci.yml/badge.svg)

Plataforma para procesar y analizar eventos de partidas de videojuegos.
Este equipo eligió **carreras** como juego base para diseñar los eventos y
las estadísticas (ver `docs/event-spec.md` para el detalle completo).

## Arquitectura

```
[ simulador (Go) ] --POST /events--> [ go-ingestion ] --POST /analyze--> [ scala-analysis ]
                                        (Go: recibe,                      (Scala: análisis
                                         valida, agrupa                    funcional, reglas,
                                         por partida,                      estadísticas)
                                         concurrencia)
```

- **`go-ingestion`** (Go): recibe eventos por HTTP, valida su estructura
  mínima, y mantiene el estado de cada partida activa en su propia
  goroutine (comunicada por un `channel`), lo que permite procesar varias
  partidas/juegos en paralelo. Cuando una partida termina (`MATCH_FINISHED`),
  envía el lote completo de eventos a Scala.
- **`scala-analysis`** (Scala): recibe el lote de una partida y calcula
  estadísticas (por jugador, por partida, y una que combina vuelta+posición)
  usando `filter`/`map`/`groupBy`/`fold`, además de una regla de detección
  de **remontada** que exige recorrer la secuencia de eventos de un jugador,
  no un evento aislado. Guarda cada resultado en **SQLite** (archivo
  persistido en un volumen de Docker) y permite consultarlo después con
  `GET /results` (lista de partidas analizadas) y
  `GET /results/{matchId}` (resultado completo de una partida).
- **Comunicación entre módulos**: HTTP/REST con JSON (decisión documentada
  en `docs/event-spec.md`, sección 7).

## Estructura del repositorio

```
gamestats/
├── docs/                         # documentación (ver índice abajo)
├── go-ingestion/                 # módulo Go
│   ├── cmd/server/                # servidor de ingestión
│   ├── cmd/simulator/             # fuente externa simulada de eventos
│   ├── cmd/smoketest/             # prueba de humo de punta a punta
│   └── internal/{model,validate,match,client}
├── scala-analysis/               # módulo Scala
│   └── src/{main,test}/scala/gamestats/{model,analysis,Main.scala}
├── data/                         # escenarios de prueba (válidos, remontada, desconexión, inválidos)
├── docker-compose.yml            # desarrollo: construye las imágenes desde el código
├── docker-compose.deploy.yml     # despliegue: usa las imágenes publicadas en GHCR
└── .github/workflows/ci.yml      # CI/CD: tests, integración, publicación y despliegue
```

### Documentación

| Documento | Contenido |
|---|---|
| [`docs/architecture.md`](docs/architecture.md) | arquitectura, concurrencia, decisiones y comparación imperativo (Go) vs funcional (Scala) |
| [`docs/game-design.md`](docs/game-design.md) | juego elegido, justificación de los eventos y cómo extender a otros juegos |
| [`docs/event-spec.md`](docs/event-spec.md) | especificación del formato de eventos y del intercambio Go → Scala |
| [`docs/statistics-and-rules.md`](docs/statistics-and-rules.md) | estadísticas y regla de remontada, con escenarios de prueba |
| [`docs/api.md`](docs/api.md) | referencia de los endpoints de ambos módulos |
| [`docs/installation.md`](docs/installation.md) | instalación, ejecución y solución de problemas |
| [`docs/ci-cd.md`](docs/ci-cd.md) | pipeline de CI/CD |
| [`docs/deployment.md`](docs/deployment.md) | despliegue con las imágenes publicadas |


## Cómo correrlo

### Con Docker Compose (recomendado)

```bash
docker compose up --build
```

Esto expone:
- `go-ingestion` en `http://localhost:8080`
- `scala-analysis` en `http://localhost:8081`

### Generar datos de prueba

Con los servicios arriba, correr el simulador (localmente, apuntando al
contenedor de ingestión):

```bash
cd go-ingestion
INGESTION_URL=http://localhost:8080 go run ./cmd/simulator
```

Esto crea 3 partidas en paralelo con varios jugadores y vueltas; cada una,
al terminar, dispara automáticamente el análisis en Scala.

### Probar manualmente con el JSON de ejemplo

```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d @data/sample-events.json
```

En Windows con PowerShell, `curl` es un alias distinto; usar en su lugar:

```powershell
$body = Get-Content -Raw -Path data/sample-events.json
Invoke-RestMethod -Uri http://localhost:8080/events -Method Post -ContentType "application/json" -Body $body
```

El resultado del análisis no se devuelve en esta llamada (es asíncrono);
aparece en los logs de `go-ingestion` unos segundos después, como
`resultado del análisis para <matchId>: {...}`.

### Consultar resultados directamente en Scala

Go reenvía cada partida terminada a Scala automáticamente, pero también se
puede llamar `scala-analysis` de forma directa para pruebas:

```bash
curl -X POST http://localhost:8081/analyze \
  -H "Content-Type: application/json" \
  -d '{"matchId":"match-demo","events": <arreglo de eventos> }'
```

## Resultados hasta ahora

- [x] Arquitectura definida y documentada (`docs/event-spec.md`): sobre de
      evento genérico + catálogo de eventos y estadísticas de carreras.
- [x] Módulo Go: servidor HTTP (`/events`, `/health`), validación de
      eventos, y agrupación por partida con una goroutine + channel por
      partida (concurrencia real, varias partidas en paralelo).
- [x] Módulo Scala: servidor HTTP (`/analyze`, `/health`), estadísticas
      por jugador y por partida, y regla de detección de remontada, todo
      implementado con `filter`/`map`/`groupBy`/`fold`.
- [x] Comunicación Go → Scala por HTTP/REST, documentada y probada.
- [x] Dockerfiles de ambos módulos + `docker-compose.yml` que los levanta
      integrados.
- [x] **Probado de punta a punta** (Windows + PowerShell + VS Code): se
      envió `data/sample-events.json` a `go-ingestion`, que validó y
      agrupó los 5 eventos por partida y, al recibir `MATCH_FINISHED`, se
      los pasó a `scala-analysis`, que devolvió estadísticas correctas
      (mejor vuelta, promedio, por jugador). Ver "Cómo repetir esta
      prueba" abajo.
- [x] Simulador (`cmd/simulator`) que genera partidas de carreras con
      varios jugadores y vueltas en paralelo.
- [x] Pipeline de CI/CD (`.github/workflows/ci.yml`): formato, `vet` y tests
      de Go con detector de carreras, compilación y tests de Scala, prueba de
      integración con Docker Compose, y publicación de las imágenes en GitHub
      Container Registry (más despliegue opcional por SSH). Ver `docs/ci-cd.md`.
- [x] Repositorio en Git/GitHub con historial de commits.
- [x] **Persistencia**: cada resultado de análisis se guarda en SQLite
      (`scala-analysis/src/main/scala/gamestats/analysis/Storage.scala`),
      en un archivo dentro de un volumen de Docker (`scala-data`) que
      sobrevive a `docker compose down`/`up` y a reconstrucciones.
- [x] **Consulta de resultados**: `GET /results` (lista de partidas
      analizadas) y `GET /results/{matchId}` (resultado completo).
      Probado de punta a punta, incluyendo reinicio de contenedores para
      confirmar que el dato persiste.

### Cómo repetir la prueba de punta a punta

```powershell
docker compose up --build
```

En otra terminal, con los servicios arriba:

```powershell
$body = Get-Content -Raw -Path data/sample-events.json
Invoke-RestMethod -Uri http://localhost:8080/events -Method Post -ContentType "application/json" -Body $body
```

El resultado del análisis aparece en la terminal donde corre
`docker compose`, como `resultado del análisis para match-demo: ...`, y
también queda guardado. Para consultarlo después (aunque hayas cerrado y
vuelto a levantar los contenedores):

```powershell
Invoke-RestMethod -Uri http://localhost:8081/results
Invoke-RestMethod -Uri http://localhost:8081/results/match-demo
```

El campo `comebacks` queda vacío con este ejemplo porque ningún jugador
gana las 3+ posiciones que exige el umbral definido en
`docs/event-spec.md`; para ver una remontada real hace falta correr el
simulador (`cmd/simulator`), que fuerza a un jugador a arrancar último y
remontar durante la carrera.

### Correr las pruebas automatizadas

```powershell
cd go-ingestion
go test -race ./...

cd ../scala-analysis
sbt test
```

### Prueba de humo de punta a punta

Con los servicios levantados (`docker compose up --build`), envía todos los
escenarios de `data/` y verifica los resultados (termina en
`RESULTADO: todo correcto`):

```powershell
cd go-ingestion
go run ./cmd/smoketest -data ../data
```

### Desplegar las imágenes publicadas

Las imágenes `gamestats-ingestion` y `gamestats-analysis` se publican en GHCR
en cada `push` a `main` (y en cada etiqueta `vX.Y.Z`). Para ejecutarlas sin
compilar nada, ver [`docs/deployment.md`](docs/deployment.md):

```powershell
docker compose -f docker-compose.deploy.yml pull
docker compose -f docker-compose.deploy.yml up -d
```

## Cumplimiento del enunciado

| Requisito | Dónde se cumple |
|---|---|
| Go: ingestión, validación, flujo, estado por partida y concurrencia | `go-ingestion/` (una goroutine y un channel por partida) |
| Scala: colecciones funcionales, estadísticas y reglas | `scala-analysis/` (`Statistics`, `Rules`); `docs/statistics-and-rules.md` |
| Estadística por jugador, por partida y combinando eventos | `playerStats` (combina vueltas, penalizaciones, pits, incidentes y clasificación final) y `matchStats` |
| Regla basada en una secuencia de eventos | remontada (`Rules.detectComebacks`) |
| Resultados estructurados y consultables | JSON en SQLite; `GET /results` y `GET /results/{matchId}` |
| Fuente de eventos simulada | `cmd/simulator` y escenarios en `data/` |
| Formato de eventos documentado y justificación del diseño | `docs/event-spec.md`, `docs/game-design.md` |
| Git, Docker, CI/CD y despliegue | historial de commits, Dockerfiles, `docker-compose*.yml`, `ci.yml`, `docs/deployment.md` |
| Instalación, ejecución y demostración | `docs/installation.md` |
| Imperativo vs funcional | `docs/architecture.md`, sección 6 |

Las limitaciones conocidas y el trabajo futuro están en
`docs/architecture.md`, sección 7.

## Entrega

Domingo **18 de octubre de 2026**, antes de las 10:00 pm, por el Tec Digital.