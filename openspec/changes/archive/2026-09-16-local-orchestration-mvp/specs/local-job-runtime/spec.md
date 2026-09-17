# Especificación de runtime local de trabajos

## Propósito

Definir trabajos locales, durables y de solo lectura para un único usuario.

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

### Requisito: Aislamiento de ejecución

El sistema DEBE preparar worktrees administrados para cada trabajo y DEBE limitar los trabajos a operaciones de solo lectura. NO DEBE invocar un shell, extraer credenciales, usar flags de bypass ni evadir políticas del proveedor.

#### Escenario: Trabajo de lectura aislado

- DADO un repositorio de origen autorizado y una solicitud de solo lectura
- CUANDO se inicia el trabajo
- ENTONCES se ejecuta en un worktree administrado separado
- Y sus registros quedan separados del repositorio de origen

#### Escenario: Solicitud no permitida

- DADO una solicitud que exige escritura, shell, red TCP o acceso a credenciales
- CUANDO se valida antes de iniciar el trabajo
- ENTONCES el sistema la rechaza sin crear proceso ni worktree
