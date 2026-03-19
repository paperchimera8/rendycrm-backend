import { describe, expect, it } from "vitest";

import {
  InMemoryDedupStore,
  createOperatorSession,
  handleOperatorEvent,
  type OperatorContext,
} from "../src/index.js";

const operatorContext: OperatorContext = {
  dedupStore: new InMemoryDedupStore(),
  linkBindings: [
    {
      code: "link-code",
      workspaceId: "ws_smoke",
      userId: "usr_1",
      chatId: "tg-chat-1",
    },
  ],
  workspaces: [
    {
      id: "ws_smoke",
      name: "Smoke Workspace",
      dashboard: {
        todayBookings: 4,
        newMessages: 2,
        revenue: 12000,
        freeSlots: 3,
        nextSlot: "Thu 19.03 14:00",
      },
      conversations: [
        {
          id: "dlg_1",
          title: "Анна",
          provider: "telegram",
          customerName: "Анна",
          customerPhone: "+7 999 111-22-33",
          customerId: "cust_1",
          status: "new",
          lastMessageText: "Хочу записаться",
          unreadCount: 1,
          slotOptions: [
            { id: "slot_1", label: "19.03 14:00" },
            { id: "slot_2", label: "19.03 15:00" },
          ],
        },
      ],
      weekSlots: [{ label: "Чт 19.03", slotCount: 3 }],
      settings: {
        autoReply: true,
        handoffEnabled: true,
        telegramChatLabel: "tg-chat-1",
        webhookUrl: "https://example.com/webhooks/telegram/operator",
        faqQuestions: ["Сколько стоит?", "Где вы находитесь?"],
      },
    },
  ],
};

class ThrowingDedupStore {
  async claim(): Promise<boolean> {
    throw new Error("dedup backend unavailable");
  }
}

describe("operator engine", () => {
  it("rejects commands when bot is not linked", async () => {
    const result = await handleOperatorEvent(
      undefined,
      { type: "message", eventId: "evt-1", text: "/dashboard" },
      operatorContext,
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Вы не привязаны. Откройте deep link из CRM или отправьте link code.",
      }),
    );
  });

  it("rejects unknown link codes", async () => {
    const result = await handleOperatorEvent(
      undefined,
      { type: "start", eventId: "evt-2", payload: "bad-code" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Не удалось привязать бота: link code не найден.",
      }),
    );
  });

  it("links on explicit start payload and shows main menu", async () => {
    const result = await handleOperatorEvent(
      undefined,
      { type: "start", eventId: "evt-3", payload: "link-code" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.binding).toEqual(
      expect.objectContaining({
        kind: "bound",
        workspaceId: "ws_smoke",
      }),
    );
    expect(result.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "operator.bound",
          workspaceId: "ws_smoke",
        }),
        expect.objectContaining({
          type: "reply",
          text: expect.stringContaining("Бот привязан к Smoke Workspace"),
        }),
      ]),
    );
  });

  it("accepts operator sessions restored from backend without recentEventIds", async () => {
    const result = await handleOperatorEvent(
      {
        binding: {
          kind: "bound" as const,
          workspaceId: "ws_smoke",
          userId: "usr_1",
          chatId: "tg-chat-1",
        },
        interaction: { kind: "idle" as const },
      },
      { type: "message", eventId: "evt-missing-recent-operator", text: "/dashboard" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.recentEventIds).toEqual([
      "evt-missing-recent-operator",
    ]);
    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Записей сегодня: 4"),
      }),
    );
  });

  it("shows dashboard for bound operator", async () => {
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const result = await handleOperatorEvent(
      session,
      { type: "message", eventId: "evt-4", text: "/dashboard" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("Записей сегодня: 4"),
      }),
    );
  });

  it("supports emoji command aliases", async () => {
    const result = await handleOperatorEvent(
      {
        ...createOperatorSession(),
        binding: {
          kind: "bound",
          workspaceId: "ws_smoke",
          userId: "usr_1",
          chatId: "tg-chat-1",
        },
      },
      { type: "message", eventId: "evt-5", text: "⚙️ Настройки" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("⚙️ Настройки"),
      }),
    );
  });

  it("starts reply flow and sends operator reply through crm effect", async () => {
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const entered = await handleOperatorEvent(
      session,
      { type: "callback", eventId: "evt-6", data: "reply:dlg_1" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );
    expect(entered.state.interaction).toEqual(
      expect.objectContaining({
        kind: "awaiting_reply",
        conversationId: "dlg_1",
      }),
    );

    const replied = await handleOperatorEvent(
      entered.state,
      {
        type: "message",
        eventId: "evt-7",
        text: "Добрый день, есть окно в 14:00",
      },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(replied.state.interaction).toEqual({ kind: "idle" });
    expect(replied.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "crm",
          intent: "operator.reply_sent",
          conversationId: "dlg_1",
        }),
        expect.objectContaining({
          type: "reply",
          text: "Ответ отправлен клиенту.",
        }),
      ]),
    );
  });

  it("starts slot pricing flow and validates numeric price", async () => {
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const entered = await handleOperatorEvent(
      session,
      { type: "callback", eventId: "evt-8", data: "pickslot:dlg_1:slot_1" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );
    expect(entered.state.interaction).toEqual(
      expect.objectContaining({
        kind: "awaiting_price",
        slotId: "slot_1",
      }),
    );

    const invalid = await handleOperatorEvent(
      entered.state,
      { type: "message", eventId: "evt-9", text: "abc" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );
    expect(invalid.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Введите цену числом, например 4500.",
      }),
    );
  });

  it("confirms booking after numeric price", async () => {
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
      interaction: {
        kind: "awaiting_price" as const,
        conversationId: "dlg_1",
        customerId: "cust_1",
        slotId: "slot_1",
        slotLabel: "19.03 14:00",
      },
    };

    const confirmed = await handleOperatorEvent(
      session,
      { type: "message", eventId: "evt-10", text: "4500" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(confirmed.state.interaction).toEqual({ kind: "idle" });
    expect(confirmed.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "booking.confirmed",
          slotId: "slot_1",
          amount: 4500,
        }),
        expect.objectContaining({
          type: "reply",
          text: "Запись подтверждена: 19.03 14:00 за 4500 ₽.",
        }),
      ]),
    );
  });

  it("updates auto reply toggle in session-visible settings", async () => {
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const toggled = await handleOperatorEvent(
      session,
      { type: "message", eventId: "evt-11", text: "/auto_off" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(toggled.state.autoReplyOverride).toBe(false);
    expect(toggled.effects).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: "settings.auto_reply_changed",
          enabled: false,
        }),
      ]),
    );
  });

  it("preserves mixed-case dialog ids in callbacks and slash commands", async () => {
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const result = await handleOperatorEvent(
      session,
      { type: "callback", eventId: "evt-12", data: "reply:Dlg_A" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
        workspaces: [
          {
            ...operatorContext.workspaces[0],
            conversations: [
              {
                ...operatorContext.workspaces[0].conversations[0],
                id: "Dlg_A",
                slotOptions: [{ id: "Slot_A", label: "19.03 16:00" }],
              },
            ],
          },
        ],
      },
    );

    expect(result.state.interaction).toEqual(
      expect.objectContaining({
        kind: "awaiting_reply",
        conversationId: "Dlg_A",
      }),
    );
  });

  it("clears stale interaction when operator navigates by callback", async () => {
    const result = await handleOperatorEvent(
      {
        ...createOperatorSession(),
        binding: {
          kind: "bound",
          workspaceId: "ws_smoke",
          userId: "usr_1",
          chatId: "tg-chat-1",
        },
        interaction: {
          kind: "awaiting_reply",
          conversationId: "dlg_1",
        },
      },
      { type: "callback", eventId: "evt-12b", data: "dialog:dlg_1" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.state.interaction).toEqual({ kind: "idle" });
  });

  it("ignores duplicate event ids through external dedup store", async () => {
    const dedupStore = new InMemoryDedupStore();
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const first = await handleOperatorEvent(
      session,
      { type: "message", eventId: "evt-dup", text: "/faq" },
      { ...operatorContext, dedupStore },
    );
    const second = await handleOperatorEvent(
      first.state,
      { type: "message", eventId: "evt-dup", text: "/faq" },
      { ...operatorContext, dedupStore },
    );

    expect(second.effects).toEqual([]);
  });

  it("falls back to in-session dedup when operator dedup store throws", async () => {
    const dedupStore = new ThrowingDedupStore();
    const session = {
      ...createOperatorSession(),
      binding: {
        kind: "bound" as const,
        workspaceId: "ws_smoke",
        userId: "usr_1",
        chatId: "tg-chat-1",
      },
    };

    const first = await handleOperatorEvent(
      session,
      { type: "message", eventId: "evt-throw", text: "/faq" },
      { ...operatorContext, dedupStore },
    );
    const second = await handleOperatorEvent(
      first.state,
      { type: "message", eventId: "evt-throw", text: "/faq" },
      { ...operatorContext, dedupStore },
    );

    expect(first.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: expect.stringContaining("FAQ"),
      }),
    );
    expect(second.effects).toEqual([]);
  });

  it("rejects stale conversation actions", async () => {
    const result = await handleOperatorEvent(
      {
        ...createOperatorSession(),
        binding: {
          kind: "bound",
          workspaceId: "ws_smoke",
          userId: "usr_1",
          chatId: "tg-chat-1",
        },
      },
      { type: "callback", eventId: "evt-13", data: "close:missing" },
      {
        ...operatorContext,
        dedupStore: new InMemoryDedupStore(),
      },
    );

    expect(result.effects).toContainEqual(
      expect.objectContaining({
        type: "reply",
        text: "Диалог не найден.",
      }),
    );
  });
});
