# Referencia de las APIs

Ambos módulos hablan HTTP con JSON (UTF-8). No tienen autenticación: están
pensados para una red interna (ver `architecture.md`, sección 7).

## Módulo Go — `go-ingestion` (puerto 8080)

### `POST /events`

Recibe **un evento** (objeto) o **varios** (arreglo) con el formato de
[`event-spec.md`](event-spec.md).

| Código | Cuándo |
|---|---|
| `202 Accepted` | todos los eventos eran válidos |
| `207 Multi-Status` | hubo eventos inválidos; los válidos sí se aceptaron |
| `400 Bad Request` | el cuerpo no es JSON válido (o un campo tiene un tipo incorrecto, p. ej. un `timestamp` que no es una fecha ISO-8601) |
| `405 Method Not Allowed` | método distinto de `POST` |

Cuerpo de la respuesta:

```json
{ "accepted": 5, "rejected": [] }
```

Si hay rechazados, cada uno aparece como `"<eventId>: <motivo>"`, por ejemplo
`"bad-001: eventId es obligatorio"`.

Campos obligatorios: `eventId`, `timestamp`, `gameId`, `matchId`, `type`; y
`playerId` para los tipos de evento de jugador (`PLAYER_JOINED`, `PLAYER_LEFT`,
`PLAYER_DISCONNECTED`, `LAP_COMPLETED`, `POSITION_CHANGED`, `PENALTY_APPLIED`,
`PIT_STOP`, `RACE_INCIDENT`).

La respuesta **no** incluye el análisis: es asíncrono. Se genera al llegar
`MATCH_FINISHED` y se consulta en Scala (abajo) o en el log de `go-ingestion`.

### `GET /health`

```json
{ "status": "ok", "activeMatches": 3 }
```

`activeMatches` es la cantidad de partidas que Go tiene abiertas en ese momento.

## Módulo Scala — `scala-analysis` (puerto 8081)

### `POST /analyze`

Lo llama Go al terminar una partida (también se puede usar a mano para
pruebas). Cuerpo:

```json
{ "matchId": "match-42", "events": [ { "...": "evento" } ] }
```

Respuesta `200` con el resultado (ver `statistics-and-rules.md`) y lo guarda en
SQLite, reemplazando el resultado anterior si ese `matchId` ya existía.
`400` si el JSON no se puede interpretar.

### `GET /results`

Lista de partidas analizadas, más recientes primero:

```json
[ { "matchId": "match-comeback", "createdAt": "2026-10-18 10:15:02" } ]
```

`createdAt` está en hora de Costa Rica (el contenedor usa `TZ=America/Costa_Rica`).

### `GET /results/{matchId}`

| Código | Cuándo |
|---|---|
| `200` | el resultado completo guardado de esa partida |
| `404` | no hay resultado para ese `matchId` |
| `500` | error al leer la base de datos |

### `GET /health`

```json
{ "status": "ok" }
```

## Ejemplos (PowerShell)

```powershell
# Enviar un escenario
Invoke-RestMethod -Uri http://localhost:8080/events -Method Post -ContentType "application/json" `
  -Body (Get-Content -Raw -Path data/scenario-comeback.json)

# Consultar
Invoke-RestMethod -Uri http://localhost:8081/results
Invoke-RestMethod -Uri http://localhost:8081/results/match-comeback | ConvertTo-Json -Depth 6
```

## Ejemplos (bash / curl)

```bash
curl -X POST http://localhost:8080/events -H "Content-Type: application/json" \
  -d @data/scenario-comeback.json
curl http://localhost:8081/results/match-comeback
```
