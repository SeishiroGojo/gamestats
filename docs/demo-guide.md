# Guion de la demostración (≈ 10 minutos)

Todos los comandos se ejecutan desde la raíz del proyecto en PowerShell, salvo
donde se indica. Se necesitan **dos terminales**.

## 0. Preparación (antes de empezar)

1. Abrir Docker Desktop y esperar a que termine de arrancar.
2. Terminal A: `docker compose up --build` y dejarla corriendo hasta ver
   `GameStats analysis escuchando en :8081` y `GameStats ingestion escuchando en :8080`.
3. Terminal B: definir un ayudante para enviar escenarios:

```powershell
function Send-Events($file) {
  Invoke-RestMethod -Uri http://localhost:8080/events -Method Post `
    -ContentType "application/json" -Body (Get-Content -Raw -Path $file)
}
```

## 1. Arquitectura (1 min)

Mostrar el diagrama de [`architecture.md`](architecture.md): fuente de eventos →
**Go** (recibe, valida, agrupa por partida, concurrencia) → **Scala**
(estadísticas y reglas funcionales, SQLite) → consulta de resultados.

## 2. Servicios sanos (30 s)

```powershell
Invoke-RestMethod http://localhost:8080/health      # status ok, activeMatches 0
Invoke-RestMethod http://localhost:8081/health      # status ok
```

## 3. Validación en Go (1 min)

```powershell
Send-Events data/scenario-invalid-events.json
```

Resultado: `accepted = 0` y 6 mensajes en `rejected`, uno por cada campo
obligatorio faltante. **Idea a transmitir:** Go es la puerta de entrada y
protege al análisis de datos incompletos.

## 4. Una partida con remontada (2 min)

```powershell
Send-Events data/scenario-comeback.json
Start-Sleep -Seconds 3
Invoke-RestMethod http://localhost:8081/results/match-comeback | ConvertTo-Json -Depth 6
```

Señalar en el JSON:

- `fastestLapMs: 89800` y `fastestLapPlayer: p4` (estadística por **partida**).
- Por jugador: penalización de `p2` (5000 ms), pit stop de `p4`, incidente de `p3`
  (estadística por **jugador** que combina varios tipos de evento).
- `comebacks`: `p4` pasó de la posición 4 a la 1 (**regla basada en una
  secuencia** de eventos, umbral de 3 posiciones).

En la Terminal A también se ve el resultado en el log de `go-ingestion`.

## 5. Casos especiales (1 min)

```powershell
Send-Events data/scenario-no-comeback.json      # adelantamientos que NO llegan al umbral
Send-Events data/scenario-disconnect.json       # jugador desconectado
Start-Sleep -Seconds 3
Invoke-RestMethod http://localhost:8081/results/match-clean | ConvertTo-Json -Depth 6        # comebacks vacío
Invoke-RestMethod http://localhost:8081/results/match-disconnect | ConvertTo-Json -Depth 6   # p3 con finalPosition null
```

## 6. Concurrencia (1 min)

Terminal B:

```powershell
cd go-ingestion
go run ./cmd/simulator          # 3 partidas de carreras en paralelo; termina con "simulación terminada"
cd ..
Start-Sleep -Seconds 3
Invoke-RestMethod http://localhost:8081/results     # lista match-1, match-2 y match-3
```

En la Terminal A se ven los análisis de las tres partidas llegando a Scala casi
al mismo tiempo. **Idea a transmitir:** cada partida es una goroutine con su
propio channel, por eso avanzan en paralelo sin mezclar sus eventos. El
simulador fuerza que un jugador salga último y remonte, así que `comebacks`
trae una remontada en cada partida.

## 7. Persistencia (1 min)

Terminal A: `Ctrl+C`, luego `docker compose up` (sin `-v`). Con los servicios
de nuevo arriba:

```powershell
Invoke-RestMethod http://localhost:8081/results     # las partidas siguen ahí
```

## 8. Calidad y automatización (1.5 min)

```powershell
cd go-ingestion
go run ./cmd/smoketest -data ../data    # termina en "RESULTADO: todo correcto"
go test -race ./...                     # tests de Go
```

Mostrar en GitHub la pestaña **Actions** (CI en verde: tests, integración con
Docker Compose y publicación de imágenes) y **Packages** (imágenes en GHCR).
Si hay sbt disponible: `cd ../scala-analysis; sbt test`.

## 9. Cierre (30 s)

Resumen: arquitectura de dos lenguajes con responsabilidades claras, análisis
funcional probado, resultados persistentes y consultables, y entrega
reproducible con CI/CD y Docker.

## Preguntas probables y respuestas breves

| Pregunta | Respuesta |
|---|---|
| ¿Por qué Go para ingestar y Scala para analizar? | Go: concurrencia ligera y control del flujo (goroutines, channels). Scala: colecciones inmutables, `Option` y funciones puras para transformar datos. Ver `architecture.md`, sección 6. |
| ¿Cómo se agrega otro juego? | El sobre del evento y el flujo son genéricos; se definen sus eventos y se agrega su análisis en Scala. Ver `game-design.md`, sección 4. |
| ¿Qué pasa si Scala está caído? | Go registra el error; hoy no reintenta (limitación documentada, con mejora propuesta). |
| ¿Y si una partida nunca termina? | No se analiza hasta recibir `MATCH_FINISHED`; se propone un temporizador de inactividad. |
| ¿Cómo se garantiza que los eventos lleguen ordenados? | Pueden llegar en cualquier orden; Go los ordena por `timestamp` antes de enviarlos. |
| ¿Cómo se prueba? | Unit tests en Go y Scala, prueba de humo de punta a punta y el CI. |
