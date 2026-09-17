# Diseño: MVP de orquestación local

## Enfoque técnico

`cliostrad` será propietario de procesos y estado. CLI y MCP usarán RPC JSON acotado sobre socket Unix. Cada trabajo ejecutará un adaptador estructurado en un worktree administrado. Solo se añadirá el SDK oficial MCP de Go.

## Decisiones de arquitectura

| Opción | Tradeoff | Decisión y motivo |
|---|---|---|
| JSON por trabajo frente a SQLite | Sin consultas ni esquema | JSON atómico; un demonio serializa el pequeño volumen MVP. |
| RPC privado frente a lógica duplicada | Protocolo interno | JSON por líneas, 256 KiB, socket `0600`; CLI y MCP comparten semántica. |
| Worktree de `HEAD` frente a copia | Escribe metadatos Git | OID detached ofrece una instantánea reproducible. |
| Dos adaptadores directos frente a registro general | Menor extensibilidad | Interfaz mínima; sin descubrimiento ni argumentos arbitrarios. |
| Recuperación frente a fallo explícito | No reanuda | Al reiniciar, estados no terminales pasan a `failed/daemon_restarted`. |

## Flujo de datos y estados

```text
CLI ─┐                         ┌─ Claude Code
MCP ─┴─ Unix RPC → Runtime → Adapter ─ agy
                     │   └→ worktree + proceso
                     └→ job.json + stdout/stderr + result

queued → preparing → running → succeeded | failed
                         └→ canceling → canceled
```

`start` persiste `queued` antes de responder. Claude usa TERM/KILL; solo `Wait` produce `canceled`. `agy` devuelve `unsupported` sin transición. Un límite produce `failed`, nunca `canceled`.

## Cambios de archivos

| Archivo | Acción | Descripción |
|---|---|---|
| `go.mod` | Crear | Módulo y `github.com/modelcontextprotocol/go-sdk/mcp` fijado. |
| `cmd/cliostrad/main.go` | Crear | Socket y ciclo del demonio. |
| `cmd/cliostra/main.go` | Crear | Cuatro comandos y modo MCP. |
| `internal/api/api.go` | Crear | DTO, códigos de error y cliente/servidor RPC. |
| `internal/runtime/runtime.go` | Crear | Estados, almacenamiento, worktrees y procesos. |
| `internal/adapters/adapters.go` | Crear | Interfaz y adaptadores Claude/`agy` con capacidades fijas. |
| `internal/mcp/server.go` | Crear | Herramientas MCP que delegan al RPC. |
| `internal/runtime/runtime_test.go` | Crear | Estado, RPC, Git y procesos falsos. |
| `internal/adapters/adapters_test.go` | Crear | Argumentos, capacidades y resultados. |
| `internal/mcp/server_test.go` | Crear | Contratos MCP en memoria. |

## Concurrencia y escalado

`internal/runtime` no ejecuta cada trabajo en una goroutine suelta: mantiene un **pool acotado de workers** (cola con buffer + N goroutines, N configurable vía flag/env, default bajo p.ej. 4). `start` encola y persiste `queued` de inmediato; un worker libre lo toma y transiciona a `preparing`/`running`. Backlog más allá de la capacidad permanece `queued` sin bloquear el RPC. Esto permite sumar trabajos o adaptadores sin reescribir el core: es una cola en memoria con límite fijo, no un scheduler distribuido, no hay red ni múltiples procesos. Ceiling conocido: si el proceso muere, el backlog en memoria se pierde (los jobs `queued` no persistidos como `running` se recuperan igual en el restart per diseño existente); subir esto a persistencia de cola cross-restart si el volumen lo justifica.

## Interfaces y contratos

```go
type Adapter interface {
	Name() string
	Capabilities() Capabilities
	Build(StartRequest, string) (ProcessSpec, error)
	Result([]byte) ([]byte, error)
}
type ProcessSpec struct { Path string; Args []string; Dir string; Stdin []byte }
```

`StartRequest` acepta adaptador, repositorio absoluto, prompt y `read_only`; nunca ejecutable, entorno ni flags. El runner hereda `PATH`, `HOME`, usuario, temporales y locale, sin registrar secretos. Claude usa modo restringido/de planificación; `agy`, lectura/sandbox. Ambos son experimentales.

Estado, worktrees y socket residirán bajo `XDG_STATE_HOME`, `XDG_CACHE_HOME` y `XDG_RUNTIME_DIR`, con permisos privados. El estado usa temporal, `fsync`, rename y `fsync` del directorio; no persiste el prompt. Límites: prompt 64 KiB, stream 8 MiB y resultado 1 MiB; excederlos termina el proceso con fallo y `truncated=true`.

El repositorio canonicalizado debe ser raíz Git con `HEAD`. Se ejecuta `git -C <repo> worktree add --detach <dest> <oid>` con argumentos separados. Se retiran bits de escritura; al terminar se restauran dentro de la raíz administrada y se elimina. Es una barrera operativa, no un sandbox.

## Estrategia de pruebas

| Capa | Prueba | Enfoque |
|---|---|---|
| Unidad | Estados, persistencia, límites, argv | Temporales y runner grabador. |
| Integración | RPC, reinicio, worktree, cancelación | Repo temporal y helper de test. |
| MCP | Cuatro herramientas | Transporte en memoria; cero proveedores reales. |

## Matriz de amenazas

| Límite | Aplicabilidad | Comportamiento seguro/fallo | RED planificada |
|---|---|---|---|
| Rutas tipo documentación | N/A: no se clasifican ejecutables | Binarios fijos del adaptador | Ninguna. |
| Selección de repositorio Git | Aplicable | Acepta raíz absoluta canonicalizada; rechaza relativa, inexistente o subdirectorio | `git -C` conserva un argumento; relativa falla; absoluta válida funciona. |
| Estado de commit | Aplicable | Usa `HEAD`; ignora índice/árbol sucio; sin commit falla | Staged, ausencia de `commit -a` e índice vacío con/sin `HEAD`. |
| Estado de push | N/A: no existe push | No se construyen refspecs | Ninguna. |
| Comandos PR | N/A: no existe automatización PR | No se invoca `gh` | Ninguna. |
| Proceso worker | Aplicable | Prompt por stdin, argv fijo y entorno filtrado; bypass falla antes del proceso | Metacaracteres no crean argv; flag y secreto no llegan al helper. |
| Salida/proceso/cancelación | Aplicable | Límite termina y falla; cancelación solo tras salida; `agy` no cambia | Exceso por stream, descendiente, Claude TERM/KILL y `agy unsupported`. |

## Migración / despliegue

Sin migración. Adaptadores experimentales solo tras revisión vigente. Reversión: detener el demonio y retirar socket, estado y worktrees administrados.

## Preguntas abiertas

Ninguna bloqueante; los argumentos se verificarán contra la versión instalada antes de habilitar cada adaptador.
