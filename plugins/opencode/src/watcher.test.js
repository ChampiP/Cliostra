import test from "node:test"
import assert from "node:assert/strict"
import {
  awaitResult,
  describeResult,
  formatSafeDiff,
  DIFF_NOTIFY_LIMIT,
} from "./watcher.js"
import {
  CliostraPlugin,
  processJobWatch,
  deliveryStore,
  getDelivery,
  listDeliveries,
  clearDeliveries,
} from "./index.js"

test("describeResult formats successful job with result and safe diff", () => {
  const job = { id: "job-123", adapter: "claude-code" }
  const result = {
    state: "succeeded",
    result: "All tests pass",
    diff: "diff --git a/a.txt b/a.txt\n+hello",
  }

  const text = describeResult(job, result)
  assert.ok(text.includes("El trabajo de Cliostra `job-123` (claude-code) terminó con estado `succeeded`."))
  assert.ok(text.includes("Resultado:\nAll tests pass"))
  assert.ok(text.includes("Diff producido por el worktree administrado:\n```diff\ndiff --git a/a.txt b/a.txt\n+hello\n```"))
  assert.ok(text.includes("Resumile esto al usuario de forma concisa."))
})

test("describeResult truncates diffs exceeding limit safely", () => {
  const job = { id: "job-big", adapter: "agy" }
  const hugeDiff = "x".repeat(DIFF_NOTIFY_LIMIT + 500)
  const result = {
    state: "succeeded",
    result: "Large diff done",
    diff: hugeDiff,
  }

  const text = describeResult(job, result)
  assert.ok(text.includes(`diff truncado a ${DIFF_NOTIFY_LIMIT} caracteres`))
  assert.ok(!text.includes("x".repeat(DIFF_NOTIFY_LIMIT + 500)))
})

test("describeResult handles empty result and failed state with reason", () => {
  const job = { id: "job-fail", adapter: "codex" }
  const result = {
    state: "failed",
    reason: "process exited with code 1",
  }

  const text = describeResult(job, result)
  assert.ok(text.includes("El trabajo de Cliostra `job-fail` (codex) terminó con estado `failed`."))
  assert.ok(text.includes("Motivo: process exited with code 1"))
  assert.ok(text.includes("Informale al usuario que falló y por qué. No lo reintentes sin que te lo pida."))
})

test("awaitResult returns immediately when available is true", async () => {
  let calls = 0
  const fakeCall = async (method, payload) => {
    calls++
    assert.equal(method, "result")
    assert.equal(payload.id, "job-fast")
    return { available: true, state: "succeeded", result: "done" }
  }

  const res = await awaitResult("job-fast", {
    call: fakeCall,
    pollIntervalMs: 10,
    timeoutMs: 1000,
  })

  assert.equal(calls, 1)
  assert.equal(res.state, "succeeded")
  assert.equal(res.result, "done")
})

test("awaitResult polls until result is available", async () => {
  let calls = 0
  const fakeCall = async () => {
    calls++
    if (calls < 3) {
      return { available: false, state: "running" }
    }
    return { available: true, state: "succeeded", result: "finished after polling" }
  }

  const sleeps = []
  const fakeSleep = async (ms) => {
    sleeps.push(ms)
  }

  const res = await awaitResult("job-polling", {
    call: fakeCall,
    sleep: fakeSleep,
    pollIntervalMs: 50,
    timeoutMs: 5000,
  })

  assert.equal(calls, 3)
  assert.equal(sleeps.length, 2)
  assert.equal(res.state, "succeeded")
  assert.equal(res.result, "finished after polling")
})

test("awaitResult throws bounded timeout error when attempts expire", async () => {
  const fakeCall = async () => ({ available: false, state: "running" })
  const fakeSleep = async () => {}

  await assert.rejects(
    async () => {
      await awaitResult("job-timeout", {
        call: fakeCall,
        sleep: fakeSleep,
        pollIntervalMs: 10,
        timeoutMs: 30,
        maxAttempts: 3,
      })
    },
    {
      message: /se agotó el tiempo de espera/,
    },
  )
})

test("processJobWatch successfully delivers notification and updates delivery store", async () => {
  clearDeliveries()
  const job = { id: "job-ok", adapter: "claude-code" }
  let prompted = null

  const fakeClient = {
    session: {
      prompt: async (args) => {
        prompted = args
      },
    },
  }

  const fakeCall = async () => ({
    available: true,
    state: "succeeded",
    result: "OK!",
    diff: "",
  })

  const record = await processJobWatch({
    client: fakeClient,
    sessionID: "ses-1",
    job,
    options: { call: fakeCall, pollIntervalMs: 10, timeoutMs: 100 },
  })

  assert.equal(record.delivered, true)
  assert.equal(record.status, "delivered")
  assert.equal(record.error, null)
  assert.equal(record.state, "succeeded")
  assert.ok(prompted)
  assert.equal(prompted.path.id, "ses-1")
  assert.ok(prompted.body.parts[0].text.includes("OK!"))

  // Observable from deliveryStore
  const stored = getDelivery("job-ok")
  assert.equal(stored.delivered, true)
  assert.equal(listDeliveries().length, 1)
})

test("processJobWatch captures prompt delivery failure and keeps reportable record", async () => {
  clearDeliveries()
  const job = { id: "job-fail-inject", adapter: "agy" }

  const fakeClient = {
    session: {
      prompt: async () => {
        throw new Error("connection closed by host")
      },
    },
  }

  const fakeCall = async () => ({
    available: true,
    state: "succeeded",
    result: "All good",
  })

  const record = await processJobWatch({
    client: fakeClient,
    sessionID: "ses-broken",
    job,
    options: { call: fakeCall, pollIntervalMs: 10, timeoutMs: 100 },
  })

  assert.equal(record.delivered, false)
  assert.equal(record.status, "failed")
  assert.equal(record.error, "connection closed by host")
  assert.equal(record.state, "succeeded")
  assert.ok(record.text.includes("All good"))

  const queried = getDelivery("job-fail-inject")
  assert.ok(queried)
  assert.equal(queried.delivered, false)
  assert.equal(queried.status, "failed")
  assert.equal(queried.error, "connection closed by host")
})

test("processJobWatch records watcher failure when polling fails", async () => {
  clearDeliveries()
  const job = { id: "job-watch-fail", adapter: "codex" }
  let deliveredText = ""

  const fakeClient = {
    session: {
      prompt: async ({ body }) => {
        deliveredText = body.parts[0].text
      },
    },
  }

  const fakeCall = async () => {
    throw new Error("RPC daemon unreachable")
  }

  const record = await processJobWatch({
    client: fakeClient,
    sessionID: "ses-rpc",
    job,
    options: { call: fakeCall, pollIntervalMs: 10, timeoutMs: 100, maxAttempts: 1 },
  })

  assert.equal(record.delivered, true)
  assert.equal(record.status, "delivered")
  assert.equal(record.state, "failed")
  assert.ok(record.reason.includes("RPC daemon unreachable"))
  assert.ok(deliveredText.includes("RPC daemon unreachable"))
})

test("CliostraPlugin rejects self-orchestration with opencode adapter", async () => {
  const plugin = await CliostraPlugin({ client: {} })
  const delegateTool = plugin.tool.cliostra_delegate

  await assert.rejects(
    async () => {
      await delegateTool.execute({ adapter: "opencode" }, { sessionID: "s1" })
    },
    {
      message: /OpenCode no puede orquestarse a sí mismo/,
    },
  )
})

test("CliostraPlugin notifications tool queries and filters deliveries", async () => {
  clearDeliveries()
  deliveryStore.set("job-1", {
    jobId: "job-1",
    adapter: "claude-code",
    state: "succeeded",
    delivered: true,
    status: "delivered",
    error: null,
    text: "job 1 finished",
  })
  deliveryStore.set("job-2", {
    jobId: "job-2",
    adapter: "agy",
    state: "succeeded",
    delivered: false,
    status: "failed",
    error: "socket timeout",
    text: "job 2 finished",
  })

  const plugin = await CliostraPlugin({ client: {} })
  const notifTool = plugin.tool.cliostra_notifications

  // Query specific
  const single = await notifTool.execute({ job_id: "job-1" })
  assert.ok(single.output.includes("Trabajo: job-1"))
  assert.ok(single.output.includes("Entregado: Sí"))

  // Query not found
  const missing = await notifTool.execute({ job_id: "job-nonexistent" })
  assert.ok(missing.output.includes("No hay registro"))

  // Query all
  const all = await notifTool.execute({})
  assert.equal(all.metadata.count, 2)

  // Query undelivered only
  const undelivered = await notifTool.execute({ undelivered_only: true })
  assert.equal(undelivered.metadata.count, 1)
  assert.ok(undelivered.output.includes("job-2"))
})
