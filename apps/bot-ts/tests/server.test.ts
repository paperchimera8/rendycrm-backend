import type { AddressInfo } from "node:net";

import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createBotRuntimeServer,
  type BotRuntimeServerConfig,
} from "../src/server.js";

const baseConfig: BotRuntimeServerConfig = {
  port: 3100,
  goAPIBaseURL: "http://go-api.test",
  runtimeToken: "runtime-token",
  httpTimeoutMs: 1_000,
};

const silentLogger = {
  log: vi.fn(),
  warn: vi.fn(),
  error: vi.fn(),
};

const servers = new Set<ReturnType<typeof createBotRuntimeServer>>();

afterEach(async () => {
  await Promise.all(
    [...servers].map(
      (server) =>
        new Promise<void>((resolve, reject) => {
          server.close((error) => {
            if (error) {
              reject(error);
              return;
            }
            resolve();
          });
        }),
    ),
  );
  servers.clear();
  vi.restoreAllMocks();
});

describe("bot runtime server", () => {
  it("returns 502 when client webhook processing fails before the update is acknowledged", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () =>
      new Response(JSON.stringify({ error: "upstream unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const { baseURL } = await startTestServer(fetchImpl);
    const response = await fetch(
      `${baseURL}/webhooks/telegram/client/cha_test/secret`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ update_id: 1 }),
      },
    );

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual(
      expect.objectContaining({
        error: expect.stringContaining(
          "go api /internal/bot-runtime/client/prepare returned 503",
        ),
      }),
    );
    expect(fetchImpl).toHaveBeenCalledTimes(1);
  });

  it("acknowledges client webhook only after prepare and apply succeed", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async (input, init) => {
      const url = new URL(
        typeof input === "string" || input instanceof URL
          ? input.toString()
          : input.url,
      );
      if (url.pathname === "/internal/bot-runtime/client/prepare") {
        return Response.json({
          skip: false,
          session: {
            route: {
              kind: "awaiting_master_phone",
              promptedAt: "1970-01-01T00:00:00.000Z",
            },
            booking: { kind: "idle" },
            recentEventIds: [],
          },
          event: {
            type: "start",
            eventId: "telegram:update:1",
            now: "2026-03-19T20:00:00.000Z",
          },
          context: {
            mode: "global",
            masters: [],
            slotsByWorkspace: {},
          },
          snapshot: {
            accountId: "cha_test",
            updateId: 1,
            chatId: "123456",
            timestamp: "2026-03-19T20:00:00.000Z",
            profile: {
              name: "Test User",
            },
          },
        });
      }

      if (url.pathname === "/internal/bot-runtime/client/apply") {
        const rawBody = init?.body;
        expect(typeof rawBody).toBe("string");
        const payload = JSON.parse(String(rawBody));
        expect(payload.snapshot).toEqual(
          expect.objectContaining({
            accountId: "cha_test",
            chatId: "123456",
          }),
        );
        expect(payload.transition.effects).toEqual(
          expect.arrayContaining([
            expect.objectContaining({
              type: "reply",
              text: expect.stringContaining("Введите номер мастера"),
            }),
          ]),
        );
        return Response.json({ ok: true });
      }

      throw new Error(`unexpected path ${url.pathname}`);
    });

    const { baseURL } = await startTestServer(fetchImpl);
    const response = await fetch(
      `${baseURL}/webhooks/telegram/client/cha_test/secret`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ update_id: 1 }),
      },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ ok: true });
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it("acknowledges operator webhook even when background processing fails", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async () =>
      new Response("operator prepare failed", {
        status: 500,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    const { baseURL } = await startTestServer(fetchImpl);
    const response = await fetch(`${baseURL}/webhooks/telegram/operator`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Bot-Api-Secret-Token": "operator-secret",
      },
      body: JSON.stringify({ update_id: 2 }),
    });

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ ok: true });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(fetchImpl).toHaveBeenCalledTimes(1);
  });

  it("acknowledges operator webhook before background prepare and apply complete", async () => {
    const fetchImpl = vi.fn<typeof fetch>(async (input) => {
      const url = new URL(
        typeof input === "string" || input instanceof URL
          ? input.toString()
          : input.url,
      );
      if (url.pathname === "/internal/bot-runtime/operator/prepare") {
        await new Promise((resolve) => setTimeout(resolve, 25));
        return Response.json({
          skip: false,
          session: {
            binding: {
              kind: "bound",
              workspaceId: "ws_smoke",
              userId: "usr_1",
              chatId: "123456",
            },
            interaction: { kind: "idle" },
            recentEventIds: [],
          },
          event: {
            type: "message",
            eventId: "telegram:update:2",
            text: "/dashboard",
          },
          context: {
            linkBindings: [],
            workspaces: [
              {
                id: "ws_smoke",
                name: "Smoke Workspace",
                dashboard: {
                  todayBookings: 1,
                  newMessages: 2,
                  revenue: 3000,
                  freeSlots: 4,
                  nextSlot: "Thu 19.03 14:00",
                },
                conversations: [],
                weekSlots: [],
                reminders: [],
                settings: {
                  autoReply: true,
                  handoffEnabled: true,
                  telegramChatLabel: "123456",
                  webhookUrl: "https://example.com/webhooks/telegram/operator",
                  faqQuestions: [],
                },
              },
            ],
          },
          snapshot: {
            accountId: "cha_operator",
            updateId: 2,
            chatId: "123456",
            telegramUserId: "777",
          },
        });
      }

      if (url.pathname === "/internal/bot-runtime/operator/apply") {
        return Response.json({ ok: true });
      }

      throw new Error(`unexpected path ${url.pathname}`);
    });

    const { baseURL } = await startTestServer(fetchImpl);
    const response = await fetch(`${baseURL}/webhooks/telegram/operator`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Bot-Api-Secret-Token": "operator-secret",
      },
      body: JSON.stringify({ update_id: 2 }),
    });

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ ok: true });
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });
});

async function startTestServer(fetchImpl: typeof fetch): Promise<{
  readonly baseURL: string;
}> {
  const server = createBotRuntimeServer(baseConfig, {
    fetchImpl,
    logger: silentLogger,
  });
  servers.add(server);

  await new Promise<void>((resolve, reject) => {
    server.listen(0, "127.0.0.1", (error?: Error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });

  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("expected TCP server address");
  }

  return {
    baseURL: `http://127.0.0.1:${(address as AddressInfo).port}`,
  };
}
