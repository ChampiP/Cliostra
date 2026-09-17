## Exploración: MVP de orquestación local

### Estado actual
Cliostra está en fase prealfa y solo contiene documentación: no hay código Go, módulo, pruebas ni contratos MCP implementados. La visión ya define un núcleo local, agnóstico de proveedor y orientado a MCP, con trabajos de larga duración, resultados finales separados de los registros y salidas de trabajadores tratadas como datos no confiables. La evidencia local previa confirma que OpenCode puede consumir MCP/plugins, Claude Code ofrece sesiones estructuradas en segundo plano y `agy` ofrece ejecución no interactiva JSON/NDJSON, sandbox, modo de planificación y conversaciones; no hay cancelación documentada para `agy`.

### Áreas afectadas
- `openspec/changes/local-orchestration-mvp/exploration.md` — registra el alcance y la decisión previa a la propuesta.
- `openspec/config.yaml` — exige abrir un issue antes de cambiar arquitectura, proveedores, límites de seguridad, contratos MCP o trabajos/sesiones.
- `docs/PROJECT_VISION.md` — define el modelo de host MCP, trabajos no bloqueantes, aislamiento de contexto y adaptadores por proveedor.
- `docs/SECURITY_AND_POLICY.md` — exige interfaces oficiales, privilegio mínimo, invocación estructurada y preservación de sandbox/aprobaciones.

### Enfoques
1. **Demonio local persistente con fachada MCP** — Un proceso Go de usuario posee el socket Unix, el estado durable de trabajos y los adaptadores; la CLI y el servidor MCP son clientes del mismo núcleo.
   - Ventajas: mantiene los trabajos después de que OpenCode termine; unifica CLI y MCP; permite estado, resultados y cancelación con semántica explícita; evita depender de extensiones específicas del host.
   - Desventajas: requiere definir el ciclo de vida del demonio, el almacenamiento local y la supervisión de procesos.
   - Esfuerzo: Medio.

2. **Plugin de OpenCode que lanza procesos directamente** — Un plugin/MCP de OpenCode inicia Claude Code o `agy` y conserva el estado en el proceso del host.
   - Ventajas: menor superficie inicial y configuración concentrada en OpenCode.
   - Desventajas: los trabajos quedan acoplados al ciclo de vida del host; no hay CLI reutilizable; la persistencia y la cancelación se duplicarían al añadir otros hosts.
   - Esfuerzo: Medio.

3. **MCP basado solo en tareas nativas** — Delegar el ciclo de vida asíncrono a las extensiones de tareas del protocolo MCP.
   - Ventajas: contrato conceptual reducido para hosts que lo soporten.
   - Desventajas: la compatibilidad de tareas MCP es desigual; no resuelve por sí sola la supervisión durable de procesos ni la CLI local.
   - Esfuerzo: Alto.

### Recomendación
Adoptar el enfoque 1 con un MVP deliberadamente estrecho: `cliostrad` local por usuario, expuesto únicamente mediante socket Unix, con estado durable de trabajo, CLI y cuatro herramientas MCP (`start`, `status`, `result`, `cancel`). `start` debe devolver inmediatamente un identificador de trabajo; `status` y `result` deben permitir sondeo por hosts sin soporte de tareas MCP. El núcleo debe modelar las capacidades por adaptador: Claude Code puede anunciar cancelación; `agy` debe anunciarla como no disponible y `cancel` debe devolver un resultado explícito en vez de simularla.

El MVP debe aceptar solo trabajos de lectura aislados en un worktree Git creado y controlado por el demonio, con directorio de trabajo fijado, invocación de procesos sin shell, salida final acotada y registros separados. Quedan fuera del MVP: descubrimiento, TUI, reanudación de conversaciones, selección de modelos, herramientas externas, acceso de escritura, red/TCP y flags que reduzcan controles del proveedor.

### Riesgos
- Un worktree es aislamiento de espacio de trabajo, no un sandbox de seguridad; cada adaptador debe preservar sus controles nativos de lectura, sandbox y aprobación.
- La creación y limpieza de worktrees, los árboles de procesos huérfanos y los reinicios del demonio requieren un ciclo de vida idempotente antes de afirmar durabilidad.
- `agy` no documenta cancelación; el contrato debe exponer esa capacidad en lugar de terminar procesos con una semántica engañosa.
- La validez de la automatización de proveedores, especialmente `agy`, debe verificarse con documentación y políticas vigentes antes de habilitar adaptadores; no se ejecutaron pruebas contra proveedores.
- La regla del repositorio exige un issue previo para este cambio de arquitectura, proveedores, MCP y trabajos/sesiones.

### Listo para propuesta
Sí, condicionado a abrir o vincular el issue de arquitectura requerido y a mantener los adaptadores como experimentales hasta completar la revisión vigente de políticas del proveedor. La propuesta debe fijar la máquina de estados, el contrato de errores/cancelación, la política de worktrees y los límites de datos persistidos antes de planificar implementación.
