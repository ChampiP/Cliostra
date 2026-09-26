export function resolve(specifier, context, nextResolve) {
  if (specifier === "@opencode-ai/plugin") {
    return {
      url: new URL("./mock-plugin.mjs", import.meta.url).href,
      format: "module",
      shortCircuit: true,
    }
  }
  return nextResolve(specifier, context)
}
