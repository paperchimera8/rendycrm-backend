import {
  createServer,
  type IncomingMessage,
  type Server,
  type ServerResponse,
} from "node:http";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

import {
  handleClientEvent as defaultHandleClientEvent,
  type ClientContext,
  type ClientEvent,
  type ClientSession,
} from "./client/engine.js";
import {
  handleOperatorEvent as defaultHandleOperatorEvent,
  type OperatorContext,
  type OperatorEvent,
  type OperatorSession,
} from "./operator/engine.js";

const DEFAULT_PORT = 3100;
const BODY_LIMIT_BYTES = 1 << 20;
const DEFAULT_HTTP_TIMEOUT_MS = 10_000;

export interface BotRuntimeServerConfig {
  readonly port: number;
  readonly goAPIBaseURL: string;
  readonly runtimeToken: string;
  readonly httpTimeoutMs?: number;
}

export interface BotRuntimeServerDependencies {
  readonly fetchImpl?: typeof fetch;
  readonly logger?: Pick<Console, "error" | "log" | "warn">;
  readonly handleClientEventImpl?: typeof defaultHandleClientEvent;
  readonly handleOperatorEventImpl?: typeof defaultHandleOperatorEvent;
}

interface ClientPrepareEvent {
  readonly type: "start" | "message" | "callback";
  readonly eventId?: string;
  readonly payload?: string;
  readonly text?: string;
  readonly data?: string;
  readonly now?: string;
}

interface ClientSnapshot {
  readonly accountId: string;
  readonly updateId: number;
  readonly chatId: string;
  readonly messageId?: number;
  readonly callbackId?: string;
  readonly callbackData?: string;
  readonly externalMessageId?: string;
  readonly timestamp: string;
  readonly profile: {
    readonly name?: string;
    readonly username?: string;
    readonly phone?: string;
  };
}

interface ClientPrepareResponse {
  readonly skip: boolean;
  readonly session?: ClientSession;
  readonly event?: ClientPrepareEvent;
  readonly context?: ClientContext;
  readonly snapshot?: ClientSnapshot;
}

interface OperatorSnapshot {
  readonly accountId: string;
  readonly updateId: number;
  readonly chatId: string;
  readonly telegramUserId: string;
  readonly messageId?: number;
  readonly callbackId?: string;
  readonly callbackData?: string;
}

interface OperatorPrepareResponse {
  readonly skip: boolean;
  readonly session?: OperatorSession;
  readonly event?: OperatorEvent;
  readonly context?: OperatorContext;
  readonly snapshot?: OperatorSnapshot;
}

interface ApplyResponse {
  readonly ok: boolean;
  readonly duplicate?: boolean;
}

class HTTPError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "HTTPError";
    this.status = status;
  }
}

export function readServerConfig(
  env: NodeJS.ProcessEnv = process.env,
): BotRuntimeServerConfig {
  return {
    port: readPort(env.PORT),
    goAPIBaseURL: readRequiredURL(env, "GO_API_BASE_URL"),
    runtimeToken: readRequiredEnv(env, "BOT_RUNTIME_INTERNAL_TOKEN"),
    httpTimeoutMs: DEFAULT_HTTP_TIMEOUT_MS,
  };
}

export function createBotRuntimeServer(
  config: BotRuntimeServerConfig,
  deps: BotRuntimeServerDependencies = {},
): Server {
  const fetchImpl = deps.fetchImpl ?? fetch;
  const logger = deps.logger ?? console;
  const handleClientEventImpl =
    deps.handleClientEventImpl ?? defaultHandleClientEvent;
  const handleOperatorEventImpl =
    deps.handleOperatorEventImpl ?? defaultHandleOperatorEvent;
  const httpTimeoutMs = config.httpTimeoutMs ?? DEFAULT_HTTP_TIMEOUT_MS;

  return createServer(async (req, res) => {
    try {
      await route(req, res);
    } catch (error) {
      const status = error instanceof HTTPError ? error.status : 500;
      const message =
        error instanceof Error ? error.message : "unexpected bot runtime error";
      if (status >= 500) {
        logger.error("[bot-ts] request failed:", error);
      }
      writeJSON(res, status, { error: message });
    }
  });

  async function route(
    req: IncomingMessage,
    res: ServerResponse,
  ): Promise<void> {
    const method = req.method ?? "GET";
    const url = new URL(req.url ?? "/", "http://localhost");

    if (method === "GET" && url.pathname === "/health") {
      writeJSON(res, 200, { status: "ok" });
      return;
    }

    if (method !== "POST") {
      writeJSON(res, 405, { error: "method not allowed" });
      return;
    }

    const clientMatch = url.pathname.match(
      /^\/webhooks\/telegram\/client\/([^/]+)\/([^/]+)$/,
    );
    if (clientMatch) {
      const accountId = clientMatch[1];
      const secret = clientMatch[2];
      if (!accountId || !secret) {
        throw new HTTPError(400, "client webhook path is incomplete");
      }
      await handleClientWebhook(req, res, accountId, secret);
      return;
    }

    if (url.pathname === "/webhooks/telegram/operator") {
      await handleOperatorWebhook(req, res);
      return;
    }

    writeJSON(res, 404, { error: "not found" });
  }

  async function handleClientWebhook(
    req: IncomingMessage,
    res: ServerResponse,
    accountId: string,
    secret: string,
  ): Promise<void> {
    const update = await readJSONBody(req);
    await processClientWebhook(accountId, secret, update);
    writeJSON(res, 200, { ok: true });
  }

  async function processClientWebhook(
    accountId: string,
    secret: string,
    update: unknown,
  ): Promise<void> {
    const prepared = await postGoJSON<ClientPrepareResponse>(
      "/internal/bot-runtime/client/prepare",
      {
        accountId,
        secret,
        update,
      },
    );
    if (prepared.skip) {
      return;
    }

    if (!prepared.event || !prepared.context || !prepared.snapshot) {
      throw new HTTPError(502, "client prepare response is incomplete");
    }

    const transition = await handleClientEventImpl(
      prepared.session,
      normalizeClientEvent(prepared.event),
      prepared.context,
    );
    const applied = await postGoJSON<ApplyResponse>(
      "/internal/bot-runtime/client/apply",
      {
        snapshot: prepared.snapshot,
        transition,
      },
    );
    if (applied.duplicate) {
      logger.warn("[bot-ts] duplicate client update skipped", {
        accountId,
        chatId: prepared.snapshot.chatId,
        updateId: prepared.snapshot.updateId,
      });
    }
  }

  async function handleOperatorWebhook(
    req: IncomingMessage,
    res: ServerResponse,
  ): Promise<void> {
    const secret = headerValue(req, "x-telegram-bot-api-secret-token");
    if (!secret) {
      writeJSON(res, 401, { error: "missing telegram webhook secret" });
      return;
    }

    const update = await readJSONBody(req);
    await processOperatorWebhook(secret, update);
    writeJSON(res, 200, { ok: true });
  }

  async function processOperatorWebhook(
    secret: string,
    update: unknown,
  ): Promise<void> {
    const prepared = await postGoJSON<OperatorPrepareResponse>(
      "/internal/bot-runtime/operator/prepare",
      {
        secret,
        update,
      },
    );
    if (prepared.skip) {
      return;
    }

    if (!prepared.event || !prepared.context || !prepared.snapshot) {
      throw new HTTPError(502, "operator prepare response is incomplete");
    }

    const transition = await handleOperatorEventImpl(
      prepared.session,
      prepared.event,
      prepared.context,
    );
    const applied = await postGoJSON<ApplyResponse>(
      "/internal/bot-runtime/operator/apply",
      {
        snapshot: prepared.snapshot,
        transition,
      },
    );
    if (applied.duplicate) {
      logger.warn("[bot-ts] duplicate operator update skipped", {
        chatId: prepared.snapshot.chatId,
        updateId: prepared.snapshot.updateId,
      });
    }
  }

  async function postGoJSON<TResponse>(
    path: string,
    body: unknown,
  ): Promise<TResponse> {
    let response: Response;
    try {
      response = await fetchImpl(new URL(path, config.goAPIBaseURL), {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Bot-Runtime-Token": config.runtimeToken,
        },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(httpTimeoutMs),
      });
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "request failed";
      throw new HTTPError(502, `go api ${path} request failed: ${message}`);
    }

    if (!response.ok) {
      const message = await response.text();
      throw new HTTPError(
        502,
        `go api ${path} returned ${response.status}: ${message.trim() || "empty response"}`,
      );
    }

    return (await response.json()) as TResponse;
  }
}

async function readJSONBody(req: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  let total = 0;

  for await (const chunk of req) {
    const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
    total += buffer.length;
    if (total > BODY_LIMIT_BYTES) {
      throw new HTTPError(413, "request body is too large");
    }
    chunks.push(buffer);
  }

  const raw = Buffer.concat(chunks).toString("utf8").trim();
  if (!raw) {
    throw new HTTPError(400, "request body is empty");
  }

  try {
    return JSON.parse(raw);
  } catch {
    throw new HTTPError(400, "invalid json payload");
  }
}

function normalizeClientEvent(event: ClientPrepareEvent): ClientEvent {
  switch (event.type) {
    case "start":
      return {
        type: "start",
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(event.payload ? { payload: event.payload } : {}),
        ...(event.now ? { now: new Date(event.now) } : {}),
      };
    case "message":
      return {
        type: "message",
        text: event.text ?? "",
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(event.now ? { now: new Date(event.now) } : {}),
      };
    case "callback":
      return {
        type: "callback",
        data: event.data ?? "",
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(event.now ? { now: new Date(event.now) } : {}),
      };
  }
}

function writeJSON(
  res: ServerResponse,
  status: number,
  payload: unknown,
): void {
  res.statusCode = status;
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(payload));
}

function readRequiredEnv(
  env: NodeJS.ProcessEnv,
  name: string,
): string {
  const value = env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function readRequiredURL(
  env: NodeJS.ProcessEnv,
  name: string,
): string {
  const value = readRequiredEnv(env, name);
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function readPort(raw: string | undefined): number {
  const fallback = DEFAULT_PORT;
  if (!raw?.trim()) {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isInteger(parsed) || parsed <= 0 || parsed > 65535) {
    throw new Error("PORT must be a valid TCP port");
  }
  return parsed;
}

function headerValue(req: IncomingMessage, key: string): string {
  const value = req.headers[key];
  if (Array.isArray(value)) {
    return value[0]?.trim() ?? "";
  }
  return value?.trim() ?? "";
}

function isEntrypoint(): boolean {
  const currentModulePath = fileURLToPath(import.meta.url);
  const entrypointPath = process.argv[1] ? resolve(process.argv[1]) : "";
  return entrypointPath === currentModulePath;
}

if (isEntrypoint()) {
  const config = readServerConfig(process.env);
  const server = createBotRuntimeServer(config);

  server.listen(config.port, () => {
    console.log(`[bot-ts] listening on :${config.port}`);
  });

  process.on("SIGTERM", () => {
    server.close(() => process.exit(0));
  });

  process.on("SIGINT", () => {
    server.close(() => process.exit(0));
  });
}
