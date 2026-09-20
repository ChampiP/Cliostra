# Especificación de runtime local de trabajos

## Propósito

Definir trabajos locales y durables para un único usuario.

## Requisitos

### Requisito: Demonio y estado durable

El sistema DEBE ejecutar un único demonio por usuario, accesible solamente mediante socket Unix. DEBE persistir el identificador, estado, metadatos seguros, resultado o error de cada trabajo para que CLI y MCP observen el mismo registro. NO DEBE abrir listeners TCP.

#### Escenario: Inicio durable

- DADO un demonio local disponible
- CUANDO se crea un trabajo válido
- ENTONCES se asigna un identificador único y se persiste su estado inicial
- Y el cliente puede desconectarse sin cancelar el trabajo

#### Escenario: Consulta después de reinicio

- DADO un trabajo terminal persistido
- CUANDO el demonio se reinicia y se consulta el identificador
- ENTONCES se devuelve el mismo estado terminal y resultado o error persistido

#### Escenario: Identificador inexistente

- DADO un identificador que no pertenece al usuario
- CUANDO se solicita su estado o resultado
- ENTONCES el sistema devuelve un error de no encontrado sin crear estado nuevo

### Requisito: Ejecución compartida en el repositorio

El sistema DEBE ejecutar los cuatro adaptadores (`claude-code`, `agy`, `codex` y `opencode`) directamente sobre la raíz real del repositorio (`gitToplevel`). NO DEBE crear un worktree git descartable ni administrar un diff de revisión para ningún adaptador. Los cambios DEBEN ser visibles durante la ejecución y poder inspeccionarse con `git diff`.

`read_only` DEBE tratarse como una instrucción para el proveedor, no como aislamiento estructural del sistema de archivos.

El sistema NO DEBE invocar un shell, extraer credenciales ni abrir listeners TCP.

#### Escenario: Ejecución compartida para cualquier adaptador

- DADO un repositorio de origen y un trabajo configurado con `claude-code`, `agy`, `codex` u `opencode`
- CUANDO se ejecuta el trabajo
- ENTONCES el proceso se ejecuta directamente sobre la raíz real del repositorio
- Y no se crea un worktree git descartable ni se administra un diff de revisión

#### Escenario: Inspección de cambios durante la ejecución

- DADO un trabajo con escritura habilitada para cualquier adaptador
- CUANDO el proceso modifica archivos durante la ejecución
- ENTONCES los cambios son visibles en el repositorio compartido
- Y pueden inspeccionarse con `git diff`

#### Escenario: `read_only` sin aislamiento estructural

- DADO un trabajo configurado con `read_only: true` para cualquier adaptador
- CUANDO se ejecuta el trabajo
- ENTONCES `read_only` se transmite como instrucción al proveedor
- Y el proceso sigue ejecutándose sobre el repositorio compartido sin aislamiento estructural

#### Escenario: Invocación estructurada sin shell

- DADO un trabajo válido en preparación para cualquier adaptador
- CUANDO el runtime inicia el proceso
- ENTONCES ejecuta el binario con argumentos estructurados directos sin invocar un shell
- Y preserva las políticas y credenciales del entorno sin exponer listeners de red
