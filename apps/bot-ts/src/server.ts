import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";

import {
  handleClientEvent,
  handleOperatorEvent,
  type ClientContext,
  type ClientEvent,
  type ClientSession,
  type OperatorContext,
  type OperatorEvent,
  type OperatorSession,
} from "./index.js";

const PORT = parsePort(process.env.PORT ?? "3100");
const MAX_BODY_BYTES = 1 << 20;

interface ClientHandleRequest {
  readonly session?: ClientSession;
  readonly event?: ClientEventWire;
  readonly context?: ClientContext;
}

interface OperatorHandleRequest {
  readonly session?: OperatorSession;
  readonly event?: OperatorEventWire;
  readonly context?: OperatorContext;
}

type ClientEventWire =
  | {
      readonly type: "start";
      readonly eventId?: string;
      readonly payload?: string;
      readonly now?: string;
    }
  | {
      readonly type: "message";
      readonly eventId?: string;
      readonly text: string;
      readonly now?: string;
    }
  | {
      readonly type: "callback";
      readonly eventId?: string;
      readonly data: string;
      readonly now?: string;
    };

type OperatorEventWire =
  | {
      readonly type: "start";
      readonly eventId?: string;
      readonly payload?: string;
    }
  | {
      readonly type: "message";
      readonly eventId?: string;
      readonly text: string;
    }
  | {
      readonly type: "callback";
      readonly eventId?: string;
      readonly data: string;
    };

const server = createServer(async (request, response) => {
  try {
    if (!request.url) {
      writeJSON(response, 400, { error: "missing url" });
      return;
    }

    if (request.method === "GET" && request.url === "/health") {
      writeJSON(response, 200, { status: "ok" });
      return;
    }

    if (request.method !== "POST") {
      writeJSON(response, 405, { error: "method not allowed" });
      return;
    }

    if (request.url === "/client/handle") {
      const payload = (await readJSON(request)) as ClientHandleRequest;
      if (!payload.event || !payload.context) {
        writeJSON(response, 400, { error: "event and context are required" });
        return;
      }

      const result = await handleClientEvent(
        payload.session,
        hydrateClientEvent(payload.event),
        payload.context,
      );
      writeJSON(response, 200, result);
      return;
    }

    if (request.url === "/operator/handle") {
      const payload = (await readJSON(request)) as OperatorHandleRequest;
      if (!payload.event || !payload.context) {
        writeJSON(response, 400, { error: "event and context are required" });
        return;
      }

      const result = await handleOperatorEvent(
        payload.session,
        hydrateOperatorEvent(payload.event),
        payload.context,
      );
      writeJSON(response, 200, result);
      return;
    }

    writeJSON(response, 404, { error: "not found" });
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "unexpected server error";
    writeJSON(response, 500, { error: message });
  }
});

server.listen(PORT, () => {
  // Keep startup logging explicit for container logs and health debugging.
  console.log(`bot engine listening on :${PORT}`);
});

function parsePort(raw: string): number {
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isInteger(parsed) || parsed <= 0 || parsed > 65535) {
    return 3100;
  }
  return parsed;
}

async function readJSON(request: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  let size = 0;

  for await (const chunk of request) {
    const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
    size += buffer.length;
    if (size > MAX_BODY_BYTES) {
      throw new Error("request body is too large");
    }
    chunks.push(buffer);
  }

  const raw = Buffer.concat(chunks).toString("utf8").trim();
  if (!raw) {
    return {};
  }
  return JSON.parse(raw);
}

function hydrateClientEvent(event: ClientEventWire): ClientEvent {
  const now = parseDate(event.now);
  switch (event.type) {
    case "start":
      return {
        type: "start",
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(event.payload ? { payload: event.payload } : {}),
        ...(now ? { now } : {}),
      };
    case "message":
      return {
        type: "message",
        text: event.text,
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(now ? { now } : {}),
      };
    case "callback":
      return {
        type: "callback",
        data: event.data,
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(now ? { now } : {}),
      };
  }
}

function hydrateOperatorEvent(event: OperatorEventWire): OperatorEvent {
  switch (event.type) {
    case "start":
      return {
        type: "start",
        ...(event.eventId ? { eventId: event.eventId } : {}),
        ...(event.payload ? { payload: event.payload } : {}),
      };
    case "message":
      return {
        type: "message",
        text: event.text,
        ...(event.eventId ? { eventId: event.eventId } : {}),
      };
    case "callback":
      return {
        type: "callback",
        data: event.data,
        ...(event.eventId ? { eventId: event.eventId } : {}),
      };
  }
}

function parseDate(raw: string | undefined): Date | undefined {
  if (!raw) {
    return undefined;
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) {
    return undefined;
  }
  return parsed;
}

function writeJSON(
  response: ServerResponse<IncomingMessage>,
  statusCode: number,
  payload: unknown,
): void {
  const body = JSON.stringify(payload);
  response.statusCode = statusCode;
  response.setHeader("Content-Type", "application/json; charset=utf-8");
  response.setHeader("Content-Length", Buffer.byteLength(body));
  response.end(body);
}
