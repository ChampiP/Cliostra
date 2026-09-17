import net from "node:net"
import os from "node:os"
import path from "node:path"

const FRAME_LIMIT = 256 * 1024

/** Resuelve la ruta del socket con las mismas reglas XDG que internal/daemonpath. */
export function socketPath() {
  const runtimeDir = process.env.XDG_RUNTIME_DIR || path.join(os.tmpdir(), "cliostra-run")
  return path.join(runtimeDir, "cliostra.sock")
}

/**
 * Ejecuta una llamada RPC contra cliostrad: una línea JSON de ida, una de
 * vuelta, y cierra. El demonio usa el mismo framing JSON-lines que la CLI.
 */
export function call(method, payload) {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(socketPath())
    let buffer = ""
    let settled = false

    const finish = (err, value) => {
      if (settled) return
      settled = true
      socket.destroy()
      err ? reject(err) : resolve(value)
    }

    socket.on("connect", () => {
      socket.write(JSON.stringify({ method, payload }) + "\n")
    })

    socket.on("data", (chunk) => {
      buffer += chunk.toString("utf8")
      if (buffer.length > FRAME_LIMIT) {
        finish(new Error("respuesta excede el límite de frame de 256KiB"))
        return
      }
      const newline = buffer.indexOf("\n")
      if (newline === -1) return

      let frame
      try {
        frame = JSON.parse(buffer.slice(0, newline))
      } catch (err) {
        finish(new Error(`respuesta ilegible del demonio: ${err.message}`))
        return
      }
      if (frame.error) {
        finish(new Error(frame.error.message || "error del demonio"))
        return
      }
      finish(null, frame.payload ?? {})
    })

    socket.on("error", (err) =>
      finish(new Error(`no se pudo conectar a cliostrad (${socketPath()}): ${err.message}`)),
    )
    socket.on("close", () => finish(new Error("el demonio cerró la conexión sin responder")))
  })
}
