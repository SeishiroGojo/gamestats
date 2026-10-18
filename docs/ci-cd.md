# Pipeline de CI/CD

Definido en [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) (GitHub
Actions). Se ejecuta en cada `push` a `main`, en cada *pull request* hacia
`main` y al crear etiquetas de versión `v*`.

```
 build-go ─┐
           ├─► e2e ─► publish (matriz: ingestion, analysis) ─► deploy (opcional)
 build-scala┘
```

| Job | Qué hace | Falla si... |
|---|---|---|
| `build-go` | `go vet`, `go build`, `go test -race` | no compila, hay advertencias de `vet`, un test falla o el detector encuentra una condición de carrera |
| `build-scala` | `sbt compile` y `sbt test` (munit) con caché de sbt | no compila o un test falla |
| `e2e` | `docker compose up -d --build --wait`, luego `go run ./cmd/smoketest`, muestra los logs si falla y limpia con `down -v` | las imágenes no construyen, los servicios no quedan sanos o la prueba de humo detecta resultados distintos a los esperados |
| `publish` | construye y publica las dos imágenes en GHCR (`ghcr.io/<usuario>/gamestats-ingestion` y `…-analysis`) con caché | no se puede autenticar o publicar |
| `deploy` | por SSH, copia `docker-compose.deploy.yml`, hace `pull` y `up -d` y espera a `/health` | solo corre si se activa (ver abajo) |

`publish` y `deploy` **solo corren en `push`** (no en pull requests) y después de
que `e2e` pasó, de modo que nunca se publica una versión que no funciona.

## Etiquetas de las imágenes

| Evento | Etiquetas publicadas |
|---|---|
| push a `main` | `latest` y `sha-<commit corto>` |
| etiqueta `v1.2.3` | `1.2.3` y `sha-<commit corto>` |

Para publicar una versión numerada:

```powershell
git tag v1.0.0
git push origin v1.0.0
```

## Administración de contenedores y archivos

- Las imágenes se construyen con **builds multi-etapa** (compilar en una imagen
  con las herramientas y copiar solo el binario/JAR a una imagen mínima).
- `.dockerignore` en cada módulo evita enviar carpetas generadas al build.
- La persistencia usa un **volumen** (`scala-data`) y `e2e` lo elimina al final
  (`down -v`) para que cada ejecución parta de cero.
- `GITHUB_TOKEN` (sin secretos manuales) es suficiente para publicar en GHCR.

## Configuración necesaria en GitHub (una sola vez)

1. **Settings → Actions → General → Workflow permissions**: permitir
   *Read and write permissions* (necesario para publicar paquetes).
2. Tras el primer `publish` exitoso: **perfil → Packages →** cada paquete
   **→ Package settings → Change visibility → Public** (así cualquier máquina
   puede hacer `pull` sin credenciales).
3. Para el despliegue automático opcional, ver `deployment.md`, sección 4.

## Estado del pipeline

Se puede mostrar con una insignia en el README:

```markdown
![CI](https://github.com/SeishiroGojo/gamestats/actions/workflows/ci.yml/badge.svg)
```
