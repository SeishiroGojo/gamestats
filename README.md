# GameStats

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
├── docs/event-spec.md          # especificación de eventos y estadísticas
├── go-ingestion/                # módulo Go
│   ├── cmd/server/               # servidor de ingestión
│   ├── cmd/simulator/            # fuente externa simulada de eventos
│   └── internal/{model,validate,match,client}
├── scala-analysis/              # módulo Scala
│   └── src/main/scala/gamestats/{model,analysis,Main.scala}
├── data/sample-events.json      # ejemplo de eventos para pruebas manuales
├── docker-compose.yml           # levanta ambos módulos integrados
└── .github/workflows/ci.yml     # pipeline de CI (build Go + Scala + imágenes)
```

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
- [x] Pipeline de CI básico (`.github/workflows/ci.yml`): build de Go,
      compilación de Scala, y build de las imágenes Docker.
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
go test ./...

cd ../scala-analysis
sbt test
```

## Pendientes

- [x] **Simulador verificado**: se corrió `cmd/simulator` con 3 partidas
      en paralelo y se detectó una remontada real por partida
      (`comebacks` no vacío).
- [x] **Estadísticas ampliadas**: pit stops (cantidad y tiempo total) e
      incidentes por jugador, documentadas en `docs/event-spec.md`.
- [x] **Pruebas automatizadas**: unit tests en Go (`validate` y `match`,
      este último con un Scala simulado vía `httptest`) y en Scala
      (`Statistics` y `Rules`, con munit). Se ejecutan también en el CI.
- [ ] Deployment de una versión funcional en un entorno definido por el
      equipo (sección 10 del enunciado), y documentarlo.
- [ ] Preparar la demostración funcional del sistema para la entrega
      (28 de octubre de 2026).