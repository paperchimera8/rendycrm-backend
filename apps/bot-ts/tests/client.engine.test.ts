import { describe, expect, it } from "vitest";

import {
  InMemoryDedupStore,
  createClientSession,
  handleClientEvent,
  type ClientContext,
} from "../src/index.js";

const now = new Date("2026-03-19T09:00:00.000Z");

const globalContext: ClientContext = {
  mode: "global",
  dedupStore: new InMemoryDedupStore(),
  masters: [
    {
      workspaceId: "ws_smoke",
      workspaceName: "Smoke Workspace",
      masterPhone: "79991112233",
      telegramEnabled: true,
    },
    {
      workspaceId: "ws_disabled",
      workspaceName: "Disabled Workspace",
      masterPhone: "79991112244",
      telegramEnabled: false,
    },
  ],
  slotsByWorkspace: {
    ws_smoke: [
      { id: "slot_1", label: "19.03 14:00" },
      { id: "slot_2", label: "19.03 15:00" },
    ],
  },
};

class ThrowingDedupStore {
  async claim(): Promise<boolean> {
    throw new Error("dedup backend unavailable");
  }
}

describe("client engine", () => {
  it("starts global flow with welcome prompt", async () => {
    const result = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-1", now },
      globalContext,
    );

    expect(result.state.route.kind).toBe("awaiting_master_phone");
    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Введите номер мастера"),
      }),
    );
  });

  it("keeps selected master on repeated start in global mode", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      { type: "start", eventId: "evt-1-ready", now },
      globalContext,
    );

    expect(result.state.route).toEqual(
      expect.objectContaining({
        kind: "ready",
        workspaceId: "ws_smoke",
      }),
    );
    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Мастер выбран: Smoke Workspace"),
      }),
    );
  });

  it("accepts sessions restored from backend without recentEventIds", async () => {
    const sessionWithoutRecentEventIds = {
      route: {
        kind: "awaiting_master_phone" as const,
        promptedAt: new Date(0).toISOString(),
      },
      booking: { kind: "idle" as const },
    };

    const result = await handleClientEvent(
      sessionWithoutRecentEventIds,
      { type: "start", eventId: "evt-missing-recent-client", now },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.recentEventIds).toEqual([
      "evt-missing-recent-client",
    ]);
    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Введите номер мастера"),
      }),
    );
  });

  it("fails fast in workspace mode when fixed workspace config is missing", async () => {
    const result = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-2", now },
      {
        mode: "workspace",
        dedupStore: new InMemoryDedupStore(),
        masters: [],
        slotsByWorkspace: {},
      },
    );

    expect(result.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "configuration.error",
          bot: "client",
        }),
      ]),
    );
  });

  it("opens fixed workspace immediately in workspace mode", async () => {
    const result = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-3", now },
      {
        mode: "workspace",
        dedupStore: new InMemoryDedupStore(),
        masters: [],
        slotsByWorkspace: {},
        fixedWorkspace: {
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
        },
      },
    );

    expect(result.state.route).toEqual(
      expect.objectContaining({
        kind: "ready",
        workspaceId: "ws_smoke",
      }),
    );
    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Бот подключён к Smoke Workspace"),
      }),
    );
  });

  it("treats prompt-like text as prompt request instead of phone parsing", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "awaiting_master_phone",
          promptedAt: new Date(0).toISOString(),
        },
      },
      {
        type: "message",
        eventId: "evt-4",
        text: "Введите номер мастера",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.route.kind).toBe("awaiting_master_phone");
    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Введите номер мастера, к которому хотите записаться.",
      }),
    );
  });

  it("selects master by phone and switches route to ready", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "awaiting_master_phone",
          promptedAt: new Date(0).toISOString(),
        },
      },
      {
        type: "message",
        eventId: "evt-5",
        text: "+7 999 111-22-33",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.route).toEqual(
      expect.objectContaining({
        kind: "ready",
        workspaceId: "ws_smoke",
      }),
    );
    expect(result.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "client.route_selected",
          workspaceId: "ws_smoke",
        }),
        expect.objectContaining({
          type: "reply",
          text: expect.stringContaining("Мастер выбран: Smoke Workspace"),
          buttons: expect.arrayContaining([
            expect.objectContaining({
              text: "Записаться",
              action: "client:open_calendar",
            }),
          ]),
        }),
      ]),
    );
  });

  it("opens calendar reply for booking action text", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      {
        type: "message",
        eventId: "evt-open-calendar-text",
        text: "записаться",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Откройте календарь и выберите удобный свободный слот.",
        buttons: [
          {
            text: "Открыть календарь",
            action: "client:open_calendar",
          },
        ],
      }),
    );
  });

  it("redirects legacy free-slots callback to calendar", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      {
        type: "callback",
        eventId: "evt-open-calendar-legacy-slots",
        data: "client:slots",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Откройте календарь и выберите удобный свободный слот.",
        buttons: [
          {
            text: "Открыть календарь",
            action: "client:open_calendar",
          },
        ],
      }),
    );
  });

  it("returns master not found reply for unknown phone", async () => {
    const result = await handleClientEvent(
      undefined,
      {
        type: "message",
        eventId: "evt-6",
        text: "+7 999 111-22-99",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Мастер с таким номером не найден.",
      }),
    );
  });

  it("returns disabled master reply when telegram is unavailable", async () => {
    const result = await handleClientEvent(
      undefined,
      {
        type: "message",
        eventId: "evt-7",
        text: "+7 999 111-22-44",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "У этого мастера Telegram-канал ещё не настроен. Укажите другой номер.",
      }),
    );
  });

  it("creates pending booking and asks for confirmation", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      {
        type: "callback",
        eventId: "evt-8",
        data: "slot:select:slot_1",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.booking).toEqual(
      expect.objectContaining({
        kind: "awaiting_confirmation",
        slotId: "slot_1",
      }),
    );
    expect(result.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "booking.pending",
          slotId: "slot_1",
        }),
        expect.objectContaining({
          type: "reply",
          text: "Подтвердить выбранное время?",
        }),
      ]),
    );
  });

  it("confirms booking and clears pending state", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
        booking: {
          kind: "awaiting_confirmation",
          workspaceId: "ws_smoke",
          slotId: "slot_1",
          slotLabel: "19.03 14:00",
        },
      },
      {
        type: "callback",
        eventId: "evt-9",
        data: "booking:confirm:slot_1",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.booking).toEqual({ kind: "idle" });
    expect(result.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "booking.confirmed",
          slotId: "slot_1",
        }),
        expect.objectContaining({
          type: "reply",
          text: "Запись подтверждена: 19.03 14:00.",
        }),
      ]),
    );
  });

  it("forwards free-form text in ready state to crm", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      {
        type: "message",
        eventId: "evt-10",
        text: "Хочу записаться завтра",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toEqual([
      expect.objectContaining({
        type: "crm",
        intent: "client.message_forwarded",
        workspaceId: "ws_smoke",
        text: "Хочу записаться завтра",
      }),
    ]);
  });

  it("allows human handoff before master selection", async () => {
    const result = await handleClientEvent(
      undefined,
      {
        type: "callback",
        eventId: "evt-11",
        data: "client:human",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toEqual([
      expect.objectContaining({
        type: "crm",
        intent: "client.human_requested",
        text: "Хочу связаться с человеком",
      }),
    ]);
  });

  it("ignores duplicate event ids through external dedup store", async () => {
    const dedupStore = new InMemoryDedupStore();
    const first = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-dup", now },
      { ...globalContext, dedupStore },
    );
    const second = await handleClientEvent(
      first.state,
      { type: "start", eventId: "evt-dup", now },
      { ...globalContext, dedupStore },
    );

    expect(second.effects).toEqual([]);
  });

  it("supports per-bot dedup namespaces on the same store", async () => {
    const dedupStore = new InMemoryDedupStore();
    const first = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-same", now },
      {
        ...globalContext,
        dedupStore,
        dedupNamespace: "client:global",
      },
    );
    const second = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-same", now },
      {
        ...globalContext,
        dedupStore,
        dedupNamespace: "client:workspace:ws_smoke",
      },
    );

    expect(first.effects.length).toBeGreaterThan(0);
    expect(second.effects.length).toBeGreaterThan(0);
  });

  it("revalidates workspace route when fixed workspace config changes", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_old",
          workspaceName: "Old Workspace",
        },
      },
      {
        type: "message",
        eventId: "evt-11b",
        text: "Привет",
        now,
      },
      {
        mode: "workspace",
        dedupStore: new InMemoryDedupStore(),
        masters: [],
        slotsByWorkspace: {},
        fixedWorkspace: {
          workspaceId: "ws_new",
          workspaceName: "New Workspace",
        },
      },
    );

    expect(result.state.route).toEqual(
      expect.objectContaining({
        kind: "ready",
        workspaceId: "ws_new",
      }),
    );
    expect(result.effects).toEqual([
      expect.objectContaining({
        type: "crm",
        intent: "client.message_forwarded",
        workspaceId: "ws_new",
      }),
    ]);
  });

  it("falls back to in-session dedup when dedup store throws", async () => {
    const dedupStore = new ThrowingDedupStore();
    const first = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-throw", now },
      { ...globalContext, dedupStore },
    );
    const second = await handleClientEvent(
      first.state,
      { type: "start", eventId: "evt-throw", now },
      { ...globalContext, dedupStore },
    );

    expect(first.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Введите номер мастера"),
      }),
    );
    expect(second.effects).toEqual([]);
  });

  it("returns booking recovery when confirmation arrives without pending state", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      {
        type: "callback",
        eventId: "evt-12",
        data: "booking:confirm:slot_1",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Не удалось найти незавершённую запись. Откройте календарь заново.",
        buttons: [
          {
            text: "Открыть календарь",
            action: "client:open_calendar",
          },
        ],
      }),
    );
  });

  it("revalidates workspace route for callbacks and human handoff", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_old",
          workspaceName: "Old Workspace",
        },
      },
      {
        type: "callback",
        eventId: "evt-11c",
        data: "client:human",
        now,
      },
      {
        mode: "workspace",
        dedupStore: new InMemoryDedupStore(),
        masters: [],
        slotsByWorkspace: {},
        fixedWorkspace: {
          workspaceId: "ws_new",
          workspaceName: "New Workspace",
        },
      },
    );

    expect(result.state.route).toEqual(
      expect.objectContaining({
        kind: "ready",
        workspaceId: "ws_new",
      }),
    );
    expect(result.effects).toEqual([
      expect.objectContaining({
        type: "crm",
        intent: "client.human_requested",
        workspaceId: "ws_new",
      }),
    ]);
  });

  it("returns slot unavailable recovery when slot selection is stale", async () => {
    const result = await handleClientEvent(
      {
        ...createClientSession(),
        route: {
          kind: "ready",
          workspaceId: "ws_smoke",
          workspaceName: "Smoke Workspace",
          masterPhone: "79991112233",
        },
      },
      {
        type: "callback",
        eventId: "evt-13",
        data: "slot:select:missing-slot",
        now,
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Этот слот уже недоступен. Откройте календарь и выберите другое время.",
        buttons: [
          {
            text: "Открыть календарь",
            action: "client:open_calendar",
          },
        ],
      }),
    );
  });

  it("suppresses repeated prompt replies inside cooldown window", async () => {
    const first = await handleClientEvent(
      undefined,
      { type: "start", eventId: "evt-14", now },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    const second = await handleClientEvent(
      first.state,
      {
        type: "message",
        eventId: "evt-15",
        text: "Введите номер мастера",
        now: new Date(now.getTime() + 30_000),
      },
      {
        ...globalContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(second.effects).toEqual([]);
  });
});
