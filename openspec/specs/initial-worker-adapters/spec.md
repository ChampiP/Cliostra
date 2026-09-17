# Especificación de adaptadores iniciales de trabajo

## Propósito

Definir adaptadores experimentales para Claude Code y `agy` sin alterar los controles de sus proveedores.

## Requisitos

### Requisito: Registro experimental y capacidades

El sistema DEBE registrar Claude Code y `agy` como adaptadores experimentales. Cada adaptador DEBE declarar sus capacidades, incluida la cancelación, antes de aceptar un trabajo; una capacidad no declarada NO DEBE asumirse disponible.

#### Escenario: Adaptador disponible

- DADO un adaptador instalado con capacidades declaradas
- CUANDO se solicita iniciar un trabajo compatible
- ENTONCES el runtime acepta el adaptador experimental y conserva sus capacidades declaradas

#### Escenario: Adaptador ausente o no compatible

- DADO un adaptador no instalado o una solicitud fuera de sus capacidades
- CUANDO se intenta iniciar el trabajo
- ENTONCES el sistema devuelve un error estructurado sin iniciar proceso

### Requisito: Límites de proveedor y proceso

Los adaptadores DEBEN usar argumentos estructurados y NO DEBEN invocar un shell. DEBEN preservar autenticación, cuotas, políticas y controles nativos del proveedor; NO DEBEN extraer credenciales ni habilitar bypasses o evasiones de política. `agy` DEBE declarar cancelación no compatible. La validación del MVP NO DEBE requerir pruebas contra proveedores reales.

#### Escenario: Invocación segura de Claude Code

- DADO una solicitud de solo lectura válida para Claude Code
- CUANDO el adaptador prepara la ejecución
- ENTONCES transmite argumentos estructurados sin shell ni secretos
- Y mantiene los controles nativos del proveedor

#### Escenario: Restricción de `agy`

- DADO una solicitud para `agy`
- CUANDO se consultan sus capacidades o se solicita cancelación
- ENTONCES declara cancelación no compatible y no afirma una cancelación inexistente

#### Escenario: Intento de bypass

- DADO una solicitud que incluye flags inseguros o evasión de política
- CUANDO el adaptador la valida
- ENTONCES la rechaza antes de invocar al proveedor
