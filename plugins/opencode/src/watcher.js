import { call } from "./rpc.js"

export const POLL_INTERVAL_MS = 2000
export const DEFAULT_TIMEOUT_MS = 2 * 60 * 60 * 1000 // 2 horas (consistente con delegateWaitTimeout)
export const DIFF_NOTIFY_LIMIT = 4000

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

/**
 * Sondea el resultado del trabajo hasta que sea terminal o expire el timeout.
 * No acepta señal de aborto a propósito: la espera tiene que sobrevivir al
 * turno que delegó el trabajo, que es justo cuando ese turno se cancela.
 */
export async function awaitResult(id, options = {}) {
  const pollIntervalMs = options.pollIntervalMs ?? POLL_INTERVAL_MS
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS
  const callRpc = options.call ?? call
  const sleepFn = options.sleep ?? sleep
  const maxAttempts = options.maxAttempts ?? Math.ceil(timeoutMs / pollIntervalMs)

  let attempts = 0
  const startTime = Date.now()

  while (attempts < maxAttempts) {
    attempts++
    const result = await callRpc("result", { id })
    if (result && result.available) {
      return result
    }
    if (Date.now() - startTime >= timeoutMs) {
      break
    }
    await sleepFn(pollIntervalMs)
  }

  throw new Error(`se agotó el tiempo de espera (${Math.round(timeoutMs / 1000)}s) sondeando el resultado del trabajo ${id}`)
}

/**
 * Trunca de forma segura diffs excesivos para evitar saturar el contexto o socket.
 */
export function formatSafeDiff(diff, limit = DIFF_NOTIFY_LIMIT) {
  if (!diff) return ""
  if (diff.length <= limit) {
    return `\n\nDiff producido por el worktree administrado:\n\`\`\`diff\n${diff}\n\`\`\``
  }
  const truncated = diff.slice(0, limit)
  return `\n\nDiff producido por el worktree administrado:\n\`\`\`diff\n${truncated}\n\`\`\`\n(diff truncado a ${limit} caracteres)`
}

/** Compone el mensaje que se inyecta en la sesión cuando el trabajo termina. */
export function describeResult(job, result) {
  const state = result?.state || "unknown"
  const header = `El trabajo de Cliostra \`${job.id}\` (${job.adapter}) terminó con estado \`${state}\`.`
  const parts = [header]

  if (result?.reason) {
    parts.push(`Motivo: ${result.reason}`)
  }

  if (result?.result) {
    parts.push(`Resultado:\n${result.result}`)
  } else if (state === "succeeded") {
    parts.push("Resultado:\n(sin salida)")
  }

  if (result?.diff) {
    parts.push(formatSafeDiff(result.diff).trim())
  }

  if (state !== "succeeded") {
    parts.push("Informale al usuario que falló y por qué. No lo reintentes sin que te lo pida.")
  } else {
    parts.push("Resumile esto al usuario de forma concisa.")
  }

  return parts.join("\n\n")
}
