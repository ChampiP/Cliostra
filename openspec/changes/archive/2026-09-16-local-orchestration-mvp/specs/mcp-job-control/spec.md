# Especificación de control MCP de trabajos

## Propósito

Definir el contrato MCP no bloqueante para controlar trabajos locales durables.

## Requisitos

### Requisito: Inicio y observación no bloqueantes

El sistema DEBE exponer `start`, `status` y `result` mediante MCP sobre el runtime local. `start` DEBE devolver el identificador sin esperar la finalización; `status` DEBE devolver el estado durable actual; `result` DEBE devolver el resultado terminal o indicar que todavía no está disponible.

#### Escenario: Inicio no bloqueante

- DADO una solicitud MCP válida para un adaptador disponible
- CUANDO se invoca `start`
- ENTONCES la respuesta contiene un identificador de trabajo antes de su finalización
- Y una consulta posterior puede observar su estado durable

#### Escenario: Resultado pendiente

- DADO un trabajo en estado no terminal
- CUANDO se invoca `result` con su identificador
- ENTONCES se indica que el resultado aún no está disponible
- Y el trabajo permanece activo sin bloqueo del cliente MCP

#### Escenario: Parámetro inválido

- DADO una invocación MCP sin identificador válido donde este es obligatorio
- CUANDO se procesa la solicitud
- ENTONCES se devuelve un error estructurado de validación sin modificar trabajos

### Requisito: Cancelación fiel a capacidades

El sistema DEBE exponer `cancel` y DEBE comunicar el resultado durable de la operación. Solo DEBE informar cancelación cuando el adaptador declare y efectúe una cancelación real; DEBE devolver no compatible cuando el adaptador no la soporte.

#### Escenario: Cancelación compatible

- DADO un trabajo activo cuyo adaptador admite cancelación
- CUANDO se invoca `cancel`
- ENTONCES el estado durable refleja la solicitud y su desenlace real

#### Escenario: Cancelación no compatible

- DADO un trabajo `agy` activo sin capacidad de cancelación declarada
- CUANDO se invoca `cancel`
- ENTONCES se devuelve no compatible
- Y el sistema NO DEBE afirmar que el trabajo fue cancelado
