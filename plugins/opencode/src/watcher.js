import { call } from "./rpc.js"

const POLL_INTERVAL_MS = 2000

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

/**
 * Sondea el resultado del trabajo hasta que sea terminal y lo devuelve.
 * No acepta señal de aborto a propósito: la espera tiene que sobrevivir al
 * turno que delegó el trabajo, que es justo cuando ese turno se cancela.
 */
export async function awaitResult(id) {
  for (;;) {
    const result = await call("result", { id })
    if (result.available) return result
    await sleep(POLL_INTERVAL_MS)
  }
}

/** Compone el mensaje que se inyecta en la sesión cuando el trabajo termina. */
export function describeResult(job, result) {
  const header = `El trabajo de Cliostra \`${job.id}\` (${job.adapter}) terminó con estado \`${result.state}\`.`
  if (result.state !== "succeeded") {
    return `${header}\n\nMotivo: ${result.reason || "sin detalle"}\n\nInformale al usuario que falló y por qué. No lo reintentes sin que te lo pida.`
  }
  const diff = result.diff ? `\n\nDiff producido por el worktree administrado:\n\`\`\`diff\n${result.diff}\n\`\`\`` : ""
  return `${header}\n\nResultado:\n${result.result || "(sin salida)"}${diff}\n\nResumile esto al usuario de forma concisa.`
}
