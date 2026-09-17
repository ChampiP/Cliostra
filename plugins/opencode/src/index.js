import { tool } from "@opencode-ai/plugin"
import { call } from "./rpc.js"
import { awaitResult, describeResult } from "./watcher.js"

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
          "Delega una tarea a otro agente de programación (Claude Code o agy) en un worktree git aislado y devuelve el control DE INMEDIATO. " +
          "No bloquea y no hace falta consultar el estado después: cuando el trabajo termina, su resultado llega solo a esta sesión. " +
          "Usala para trabajo largo; avisá al usuario que seguís disponible mientras corre.",
        args: {
          adapter: tool.schema
            .enum(["claude-code", "agy"])
            .describe("agente que ejecuta el trabajo"),
          repo: tool.schema.string().describe("ruta absoluta al repositorio git"),
          prompt: tool.schema.string().describe("instrucción para el agente delegado"),
          read_only: tool.schema
            .boolean()
            .describe(
              "true: solo inspecciona. false: edita y ejecuta comandos, pero SOLO dentro de un worktree git desechable; el repo real nunca se toca y el resultado incluye el diff.",
            ),
        },
        async execute(args, context) {
          const { id } = await call("start", {
            adapter: args.adapter,
            repo: args.repo,
            prompt: args.prompt,
            read_only: args.read_only,
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
    },
  }
}

/**
 * Espera el trabajo fuera del turno actual e inyecta el desenlace en la
 * sesión. Cualquier fallo se reporta también como mensaje: un trabajo que
 * muere en silencio es peor que uno que avisa que falló.
 */
function watchInBackground({ client, sessionID, job }) {
  void (async () => {
    let text
    try {
      const result = await awaitResult(job.id)
      text = describeResult(job, result)
    } catch (err) {
      text = `El seguimiento del trabajo de Cliostra \`${job.id}\` falló: ${err.message}. Informale al usuario que el resultado no pudo recuperarse.`
    }
    await client.session
      .prompt({ path: { id: sessionID }, body: { parts: [{ type: "text", text }] } })
      .catch(() => {})
  })()
}

export default CliostraPlugin
