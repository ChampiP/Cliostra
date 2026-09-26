import { tool } from "@opencode-ai/plugin"
import { call } from "./rpc.js"
import { awaitResult, describeResult } from "./watcher.js"

/**
 * Registro en memoria de entregas de notificaciones.
 * Permite que fallos de inyección sean observables, auditables y recuperables.
 */
export const deliveryStore = new Map()

export function getDelivery(jobId) {
  return deliveryStore.get(jobId) || null
}

export function listDeliveries() {
  return Array.from(deliveryStore.values())
}

export function clearDeliveries() {
  deliveryStore.clear()
}

/**
 * Procesa la espera e inyección de la notificación de un trabajo en segundo plano.
 * Registra el estado antes y después del intento de entrega sin lanzar errores no controlados.
 */
export async function processJobWatch({ client, sessionID, job, options = {} }) {
  const record = deliveryStore.get(job.id) || {
    jobId: job.id,
    adapter: job.adapter,
    sessionID,
    state: "pending",
    delivered: false,
    status: "pending",
    text: null,
    error: null,
    reason: null,
    result: null,
    createdAt: Date.now(),
    completedAt: null,
    deliveredAt: null,
    attempts: 0,
  }
  deliveryStore.set(job.id, record)

  let text
  try {
    const result = await awaitResult(job.id, options)
    record.result = result
    record.state = result.state || "unknown"
    record.reason = result.reason || null
    text = describeResult(job, result)
  } catch (err) {
    record.state = "failed"
    record.reason = err.message
    text = `El seguimiento del trabajo de Cliostra \`${job.id}\` falló: ${err.message}. Informale al usuario que el resultado no pudo recuperarse.`
  }

  record.text = text
  record.completedAt = Date.now()
  record.attempts++

  try {
    if (!client?.session?.prompt) {
      throw new Error("client.session.prompt no está disponible")
    }
    await client.session.prompt({
      path: { id: sessionID },
      body: { parts: [{ type: "text", text }] },
    })
    record.delivered = true
    record.status = "delivered"
    record.deliveredAt = Date.now()
    record.error = null
  } catch (err) {
    record.delivered = false
    record.status = "failed"
    record.error = err.message || String(err)
    console.error(`[cliostra-plugin] Error al entregar notificación para trabajo ${job.id}: ${record.error}`)
  }

  return record
}

/**
 * Plugin de OpenCode para Cliostra.
 *
 * Aporta lo que el servidor MCP no puede: delegación realmente asíncrona.
 * La tool devuelve el control apenas encola el trabajo, y cuando el trabajo
 * termina el plugin inyecta el resultado en la misma sesión con
 * `client.session.prompt`. El orquestador reacciona solo, sin que nadie le
 * pida "status".
 */
export const CliostraPlugin = async ({ client }) => {
  return {
    tool: {
      cliostra_delegate: tool({
        description:
          "Delega una tarea a otro agente de programación y devuelve el control DE INMEDIATO. Claude Code trabaja en un worktree git desechable; agy trabaja deliberadamente sobre el repositorio real para que sus cambios se vean en tiempo real. " +
          "No bloquea y no hace falta consultar el estado después: cuando el trabajo termina, su resultado llega solo a esta sesión. " +
          "Usala para trabajo largo; avisá al usuario que seguís disponible mientras corre.",
        args: {
          adapter: tool.schema
            .enum(["claude-code", "agy", "codex"])
            .describe("agente que ejecuta el trabajo (otro harness distinto a OpenCode)"),
          repo: tool.schema.string().describe("ruta absoluta al repositorio git"),
          prompt: tool.schema.string().describe("instrucción para el agente delegado"),
          read_only: tool.schema
            .boolean()
            .describe(
              "true: solicita solo inspección mediante las restricciones del adaptador; agy sigue ejecutándose sobre el repositorio real, sin aislamiento estructural. false: Claude Code edita en su worktree y devuelve un diff; agy puede editar y ejecutar sobre el repo real. Los procesos tienen los permisos del usuario: Git/GitHub ayudan a recuperar cambios, pero no impiden borrar archivos locales durante la ejecución.",
            ),
        },
        async execute(args, context) {
          if (args.adapter === "opencode") {
            throw new Error("OpenCode no puede orquestarse a sí mismo: seleccioná otro adaptador (claude-code, agy, codex)")
          }
          const { id } = await call("start", {
            adapter: args.adapter,
            repo: args.repo,
            prompt: args.prompt,
            read_only: args.read_only,
            caller: "opencode",
          })

          // Deliberadamente NO se propaga context.abort: esa señal se cancela
          // al terminar el turno, que es exactamente cuando el watcher tiene
          // que seguir vivo. Vive mientras viva el servidor de OpenCode.
          watchInBackground({ client, sessionID: context.sessionID, job: { id, adapter: args.adapter } })

          return {
            title: `Cliostra · ${args.adapter}`,
            output:
              `Trabajo ${id} encolado en ${args.adapter}. Seguí con lo tuyo: el resultado va a llegar solo a esta sesión cuando termine. ` +
              `No consultes el estado ni esperes; no le prometas al usuario que vas a avisar, el aviso es automático.`,
            metadata: { jobID: id, adapter: args.adapter },
          }
        },
      }),
      cliostra_notifications: tool({
        description:
          "Consulta el estado y registro de entrega de las notificaciones de trabajos delegados asíncronamente en Cliostra. Permite recuperar resultados o verificar fallos de inyección.",
        args: {
          job_id: tool.schema.string().optional().describe("ID del trabajo a consultar (opcional)"),
          undelivered_only: tool.schema.boolean().optional().describe("si es true, lista solo las notificaciones cuya entrega falló o sigue pendiente"),
        },
        async execute(args) {
          if (args?.job_id) {
            const entry = getDelivery(args.job_id)
            if (!entry) {
              return {
                title: `Cliostra · Notificación ${args.job_id}`,
                output: `No hay registro de notificación para el trabajo ${args.job_id}.`,
                metadata: { found: false },
              }
            }
            return {
              title: `Cliostra · Notificación ${args.job_id}`,
              output:
                `Trabajo: ${entry.jobId}\nAdaptador: ${entry.adapter}\nEstado del trabajo: ${entry.state}\n` +
                `Entregado: ${entry.delivered ? "Sí" : "No"}\nEstado de entrega: ${entry.status}\n` +
                `Error de entrega: ${entry.error || "ninguno"}\n` +
                (entry.reason ? `Motivo: ${entry.reason}\n` : "") +
                `\nMensaje preparado:\n${entry.text || "(ninguno)"}`,
              metadata: entry,
            }
          }

          let items = listDeliveries()
          if (args?.undelivered_only) {
            items = items.filter((item) => !item.delivered)
          }

          if (items.length === 0) {
            return {
              title: "Cliostra · Notificaciones",
              output: args?.undelivered_only
                ? "No hay notificaciones pendientes ni fallidas."
                : "No hay registro de notificaciones.",
              metadata: { count: 0, items: [] },
            }
          }

          const summary = items
            .map(
              (i) =>
                `- Trabajo \`${i.jobId}\` (${i.adapter}): estado=\`${i.state}\`, entregado=${i.delivered ? "sí" : "NO"}${i.error ? ` (error: ${i.error})` : ""}`,
            )
            .join("\n")

          return {
            title: "Cliostra · Notificaciones",
            output: `Notificaciones registradas (${items.length}):\n${summary}`,
            metadata: { count: items.length, items },
          }
        },
      }),
    },
  }
}

/**
 * Espera el trabajo fuera del turno actual e inyecta el desenlace en la
 * sesión. Cualquier fallo se reporta también en el registro de entrega.
 */
function watchInBackground({ client, sessionID, job, options }) {
  void processJobWatch({ client, sessionID, job, options })
}

export default CliostraPlugin
