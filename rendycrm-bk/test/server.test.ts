import { after, before, test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { once } from "node:events";

type CapturedRequest = {
  method: string;
  url: string;
  body: string;
};

const tgRequests: CapturedRequest[] = [];
const mockServerPort = 3301;
const appPort = 3302;
let mockServer = createServer();
let appProcess: ChildProcessWithoutNullStreams | undefined;

before(async () => {
  mockServer = createServer((req: IncomingMessage, res: ServerResponse) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      tgRequests.push({
        method: req.method ?? "",
        url: req.url ?? "",
        body,
      });
      res.writeHead(200, { "content-type": "application/json" });
      res.end(JSON.stringify({ ok: true, result: { message_id: 123 } }));
    });
  });
  mockServer.listen(mockServerPort, "127.0.0.1");
  await once(mockServer, "listening");

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
      TELEGRAM_API_BASE_URL: `http://127.0.0.1:${mockServerPort}`,
      TELEGRAM_CLIENT_BOT_TOKEN: "test-client-token",
      TELEGRAM_OPERATOR_BOT_TOKEN: "test-operator-token",
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
      throw new Error("backend process exited before startup");
    }
    if (Date.now() > deadline) {
      throw new Error("backend process did not start in time");
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
});

after(async () => {
  if (appProcess && appProcess.exitCode === null) {
    appProcess.kill("SIGTERM");
    await once(appProcess, "exit");
  }
  mockServer.close();
  await once(mockServer, "close");
});

test("health endpoint responds", async () => {
  const response = await fetch(`http://127.0.0.1:${appPort}/health`);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { status: "ok" });
});

test("auth endpoints issue and validate token", async () => {
  const loginResponse = await fetch(`http://127.0.0.1:${appPort}/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      email: "operator@rendycrm.local",
      password: "password",
    }),
  });
  assert.equal(loginResponse.status, 200);
  const loginBody = (await loginResponse.json()) as { token?: string };
  assert.equal(typeof loginBody.token, "string");
  assert.ok(loginBody.token);

  const meResponse = await fetch(`http://127.0.0.1:${appPort}/auth/me`, {
    headers: {
      authorization: `Bearer ${loginBody.token}`,
    },
  });
  assert.equal(meResponse.status, 200);
  const meBody = (await meResponse.json()) as { user?: { email?: string } };
  assert.equal(meBody.user?.email, "operator@rendycrm.local");
});

test("client webhook triggers telegram sendMessage", async () => {
  tgRequests.length = 0;
  const response = await fetch(`http://127.0.0.1:${appPort}/webhooks/telegram/client/ws-main/client-secret`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      update_id: 100,
      message: {
        message_id: 10,
        text: "/start",
        chat: { id: 777 },
        from: { id: 1 },
      },
    }),
  });

  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { ok: true, handled: true });
  assert.equal(tgRequests.length, 1);
  assert.equal(tgRequests[0]?.url, "/bottest-client-token/sendMessage");

  const body = JSON.parse(tgRequests[0]?.body ?? "{}") as { chat_id?: number; text?: string };
  assert.equal(body.chat_id, 777);
  assert.match(body.text ?? "", /Здравствуйте!/);
});

test("operator webhook validates secret header and triggers sendMessage", async () => {
  tgRequests.length = 0;
  const response = await fetch(`http://127.0.0.1:${appPort}/webhooks/telegram/operator`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-Telegram-Bot-Api-Secret-Token": "operator-secret",
    },
    body: JSON.stringify({
      update_id: 101,
      message: {
        message_id: 20,
        text: "/start",
        chat: { id: 888 },
        from: { id: 2 },
      },
    }),
  });

  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { ok: true, handled: true });
  assert.equal(tgRequests.length, 1);
  assert.equal(tgRequests[0]?.url, "/bottest-operator-token/sendMessage");
});
