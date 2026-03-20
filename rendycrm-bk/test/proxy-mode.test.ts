import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { once } from "node:events";

type ForwardedRequest = {
  method: string;
  url: string;
  body: string;
  headers: Record<string, string | undefined>;
};

const forwardedRequests: ForwardedRequest[] = [];
const goMockPort = 3311;
const appPort = 3312;
let goMockServer = createServer();
let appProcess: ChildProcessWithoutNullStreams | undefined;

before(async () => {
  goMockServer = createServer((req: IncomingMessage, res: ServerResponse) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      forwardedRequests.push({
        method: req.method ?? "",
        url: req.url ?? "",
        body,
        headers: {
          "x-bot-runtime-secret": req.headers["x-bot-runtime-secret"] as string | undefined,
          "x-telegram-bot-api-secret-token": req.headers["x-telegram-bot-api-secret-token"] as string | undefined,
        },
      });
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ ok: true }));
    });
  });
  goMockServer.listen(goMockPort, "127.0.0.1");
  await once(goMockServer, "listening");

  appProcess = spawn("npm", ["run", "dev"], {
    cwd: "/Users/vital/Documents/rendycrm-app/rendycrm-bk",
    env: {
      ...process.env,
      FORCE_COLOR: "0",
      PORT: String(appPort),
      AUTH_SECRET: "test-secret",
      ADMIN_EMAIL: "operator@rendycrm.local",
      ADMIN_PASSWORD: "password",
      TELEGRAM_CLIENT_WEBHOOK_SECRET: "client-secret",
      TELEGRAM_OPERATOR_WEBHOOK_SECRET: "operator-secret",
      GO_API_BASE_URL: `http://127.0.0.1:${goMockPort}`,
      BOT_RUNTIME_INTERNAL_SECRET: "shared-secret",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  let ready = false;
  const onData = (chunk: Buffer) => {
    if (chunk.toString("utf8").includes(`backend listening on :${appPort}`)) {
      ready = true;
    }
  };
  appProcess.stdout.on("data", onData);
  appProcess.stderr.on("data", onData);

  const deadline = Date.now() + 20_000;
  while (!ready) {
    if (!appProcess || appProcess.exitCode !== null) {
      throw new Error("proxy-mode backend process exited before startup");
    }
    if (Date.now() > deadline) {
      throw new Error("proxy-mode backend process did not start in time");
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
});

after(async () => {
  if (appProcess && appProcess.exitCode === null) {
    appProcess.kill("SIGTERM");
    await once(appProcess, "exit");
  }
  goMockServer.close();
  await once(goMockServer, "close");
});

test("operator webhook proxy mode acknowledges immediately and forwards to go runtime", async () => {
  forwardedRequests.length = 0;

  const response = await fetch(`http://127.0.0.1:${appPort}/webhooks/telegram/operator`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-Telegram-Bot-Api-Secret-Token": "operator-secret",
      "X-Bot-Runtime-Secret": "shared-secret",
    },
    body: JSON.stringify({
      update_id: 201,
      message: {
        message_id: 20,
        text: "/start",
        chat: { id: 888 },
        from: { id: 2 },
      },
    }),
  });

  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { ok: true, handled: true, accepted: true });

  const deadline = Date.now() + 5_000;
  while (forwardedRequests.length === 0 && Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  assert.equal(forwardedRequests.length, 1);
  assert.equal(forwardedRequests[0]?.method, "POST");
  assert.equal(forwardedRequests[0]?.url, "/internal/bot-runtime/telegram/operator");
  assert.equal(forwardedRequests[0]?.headers["x-bot-runtime-secret"], "shared-secret");
  assert.equal(forwardedRequests[0]?.headers["x-telegram-bot-api-secret-token"], "operator-secret");
});

test("client webhook proxy mode deduplicates repeated updates", async () => {
  forwardedRequests.length = 0;
  const payload = {
    update_id: 301,
    message: {
      message_id: 30,
      text: "/start",
      chat: { id: 777 },
      from: { id: 1 },
    },
  };

  const first = await fetch(`http://127.0.0.1:${appPort}/webhooks/telegram/client/cha_1/client-secret`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-Bot-Runtime-Secret": "shared-secret",
    },
    body: JSON.stringify(payload),
  });
  const second = await fetch(`http://127.0.0.1:${appPort}/webhooks/telegram/client/cha_1/client-secret`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-Bot-Runtime-Secret": "shared-secret",
    },
    body: JSON.stringify(payload),
  });

  assert.equal(first.status, 200);
  assert.deepEqual(await first.json(), { ok: true, handled: true, accepted: true });
  assert.equal(second.status, 200);
  assert.deepEqual(await second.json(), { ok: true, handled: false, accepted: true });

  const deadline = Date.now() + 5_000;
  while (forwardedRequests.length === 0 && Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 50));
  }

  assert.equal(forwardedRequests.length, 1);
  assert.equal(forwardedRequests[0]?.url, "/internal/bot-runtime/telegram/client/cha_1/client-secret");
});
