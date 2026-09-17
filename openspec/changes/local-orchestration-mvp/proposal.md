# Propuesta: MVP de orquestación local

## Intención

Permitir que OpenCode inicie trabajos locales de solo lectura en Claude Code o Google Antigravity (`agy`), continúe trabajando y consulte después su estado o resultado, sin extraer credenciales ni debilitar controles de proveedores.

## Alcance

### Incluido
- `cliostrad` por usuario, socket Unix y estado durable de trabajos.
- CLI y fachada MCP portable con `start`, `status`, `result` y `cancel`.
- Adaptadores experimentales para Claude Code y `agy`, con capacidades declaradas.
- Trabajos de solo lectura en worktrees administrados, sin shell y con registros separados.

### Excluido
- Escritura, TUI, descubrimiento, reanudación, modelos y herramientas externas.
- Listener TCP/remoto, extracción de credenciales, evasión de políticas/cuotas o flags inseguros.
- Prometer cancelación para `agy` o ejecutar pruebas reales contra proveedores en esta fase.

## Capacidades

### Capacidades nuevas
- `local-job-runtime`: ciclo durable de trabajos, socket, CLI, worktrees y límites de datos.
- `mcp-job-control`: contrato no bloqueante para iniciar, consultar, obtener resultados y cancelar.
- `initial-worker-adapters`: ejecución de Claude Code y `agy` con capacidades y restricciones explícitas.

### Capacidades modificadas
Ninguna.

## Enfoque

Un único núcleo Go en `cliostrad` atenderá a CLI y MCP mediante socket Unix. Estados y errores serán persistentes e idempotentes. Cada adaptador usará argumentos estructurados, preservará controles nativos y declarará cancelación real; `agy` responderá “no soportado”.

## Áreas afectadas

| Área | Impacto | Descripción |
|------|---------|-------------|
| `go.mod`, `cmd/` | Nuevo | Módulo, demonio y CLI |
| `internal/jobs/`, `internal/mcp/` | Nuevo | Estado y MCP |
| `internal/adapters/` | Nuevo | Claude Code y `agy` |

## Riesgos

| Riesgo | Probabilidad | Mitigación |
|--------|--------------|------------|
| Worktree confundido con sandbox | Media | Preservar controles nativos y solo lectura |
| Reinicios o procesos huérfanos corrompen estados | Media | Transiciones idempotentes y recuperación explícita |
| Política del proveedor incierta | Alta | Adaptadores experimentales; revisión vigente antes de habilitarlos |
| Superar 400 líneas revisables | Alta | Dividir tareas en unidades verificables antes de aplicar |

## Plan de reversión

Retirar la configuración MCP, detener `cliostrad` y eliminar únicamente socket, worktrees y estado creados por Cliostra; ningún repositorio ni credencial requerirá migración.

## Dependencias

- Issue arquitectónico [#1](https://github.com/ChampiP/Cliostra/issues/1).
- Git y al menos un proveedor instalado; revisión de política antes de habilitarlo.

## Criterios de éxito

- [ ] `start` devuelve un ID sin bloquear y el trabajo sobrevive al cliente MCP.
- [ ] CLI y MCP observan el mismo estado y resultado durable.
- [ ] Solo se ejecutan trabajos de lectura sin shell, TCP ni acceso a credenciales.
- [ ] `cancel` refleja fielmente la capacidad del adaptador, incluida la no disponibilidad en `agy`.
