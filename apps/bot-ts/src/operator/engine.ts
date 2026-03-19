import {
  claimEvent,
  reply,
  type BotButton,
  type DedupStore,
  type SlotOption,
  type Transition,
} from "../shared/types.js";

const OPERATOR_NOT_LINKED_TEXT =
  "Вы не привязаны. Откройте deep link из CRM или отправьте link code.";
const OPERATOR_LINK_FAILED_TEXT =
  "Не удалось привязать бота: link code не найден.";
const OPERATOR_WORKSPACE_MISSING_TEXT =
  "Workspace для оператора не найден.";

export interface OperatorLinkBinding {
  readonly code: string;
  readonly workspaceId: string;
  readonly userId: string;
  readonly chatId: string;
}

export interface OperatorDashboard {
  readonly todayBookings: number;
  readonly newMessages: number;
  readonly revenue: number;
  readonly freeSlots: number;
  readonly nextSlot: string;
}

export interface OperatorConversation {
  readonly id: string;
  readonly title: string;
  readonly provider: string;
  readonly customerName: string;
  readonly customerPhone?: string;
  readonly customerId?: string;
  readonly status: string;
  readonly lastMessageText: string;
  readonly unreadCount: number;
  readonly slotOptions: readonly SlotOption[];
}

export interface OperatorWeekSlots {
  readonly label: string;
  readonly slotCount: number;
}

export interface OperatorSettings {
  readonly autoReply: boolean;
  readonly handoffEnabled: boolean;
  readonly telegramChatLabel: string;
  readonly webhookUrl: string;
  readonly faqQuestions: readonly string[];
}

export interface OperatorWorkspace {
  readonly id: string;
  readonly name: string;
  readonly dashboard: OperatorDashboard;
  readonly conversations: readonly OperatorConversation[];
  readonly weekSlots: readonly OperatorWeekSlots[];
  readonly settings: OperatorSettings;
}

export interface OperatorContext {
  readonly linkBindings: readonly OperatorLinkBinding[];
  readonly workspaces: readonly OperatorWorkspace[];
  readonly dedupStore?: DedupStore;
  readonly dedupNamespace?: string;
  readonly dedupTtlSeconds?: number;
}

export type OperatorBinding =
  | { readonly kind: "unbound" }
  | {
      readonly kind: "bound";
      readonly workspaceId: string;
      readonly userId: string;
      readonly chatId: string;
    };

export type OperatorInteraction =
  | { readonly kind: "idle" }
  | { readonly kind: "awaiting_reply"; readonly conversationId: string }
  | {
      readonly kind: "awaiting_price";
      readonly conversationId: string;
      readonly customerId?: string;
      readonly slotId: string;
      readonly slotLabel: string;
    };

export interface OperatorSession {
  readonly binding: OperatorBinding;
  readonly interaction: OperatorInteraction;
  readonly autoReplyOverride?: boolean;
  readonly recentEventIds: readonly string[];
}

export type OperatorEvent =
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

const MAIN_MENU_BUTTONS: readonly BotButton[] = [
  { text: "Дашборд", action: "/dashboard" },
  { text: "Диалоги", action: "/dialogs" },
  { text: "Слоты", action: "/slots" },
  { text: "Настройки", action: "/settings" },
  { text: "FAQ", action: "/faq" },
];

export function createOperatorSession(): OperatorSession {
  return {
    binding: { kind: "unbound" },
    interaction: { kind: "idle" },
    recentEventIds: [],
  };
}

export async function handleOperatorEvent(
  rawSession: OperatorSession | undefined,
  event: OperatorEvent,
  context: OperatorContext,
): Promise<Transition<OperatorSession>> {
  const currentSession = rawSession ?? createOperatorSession();
  const dedupResult = await claimEvent(
    currentSession,
    context.dedupNamespace ?? "operator",
    event.eventId,
    context.dedupStore,
    context.dedupTtlSeconds,
  );
  if (!dedupResult.claimed) {
    return { state: dedupResult.state, effects: [] };
  }

  const session = dedupResult.state;
  if (event.type === "start" && event.payload?.trim()) {
    return bindOperator(session, event.payload.trim(), context);
  }

  const rawCommand =
    event.type === "callback"
      ? event.data
      : event.type === "message"
        ? event.text
        : "/start";

  if (session.binding.kind === "unbound") {
    return handleUnboundOperatorInput(session, rawCommand, context);
  }

  const normalizedCommand = normalizeOperatorCommand(rawCommand);
  if (normalizedCommand === "/start" || normalizedCommand === "отмена") {
    return {
      state: { ...session, interaction: { kind: "idle" } },
      effects: [reply("Доступны основные разделы.", MAIN_MENU_BUTTONS)],
    };
  }

  if (event.type === "message") {
    const interactionTransition = handleOperatorInteraction(
      session,
      event.text,
      context,
    );
    if (interactionTransition) {
      return interactionTransition;
    }
  }

  const commandSession =
    event.type === "callback" && session.interaction.kind !== "idle"
      ? { ...session, interaction: { kind: "idle" as const } }
      : session;

  return handleBoundOperatorCommand(commandSession, normalizedCommand, context);
}

function handleUnboundOperatorInput(
  session: OperatorSession,
  rawInput: string,
  context: OperatorContext,
): Transition<OperatorSession> {
  const linkCode = extractLinkCode(rawInput);
  if (!linkCode) {
    return { state: session, effects: [reply(OPERATOR_NOT_LINKED_TEXT)] };
  }
  return bindOperator(session, linkCode, context);
}

function bindOperator(
  session: OperatorSession,
  linkCode: string,
  context: OperatorContext,
): Transition<OperatorSession> {
  const binding = context.linkBindings.find(
    (candidate) => candidate.code === linkCode,
  );
  if (!binding) {
    return { state: session, effects: [reply(OPERATOR_LINK_FAILED_TEXT)] };
  }

  const workspace = context.workspaces.find(
    (candidate) => candidate.id === binding.workspaceId,
  );
  if (!workspace) {
    return {
      state: session,
      effects: [
        {
          type: "configuration.error",
          bot: "operator",
          message: OPERATOR_WORKSPACE_MISSING_TEXT,
        },
        reply(OPERATOR_WORKSPACE_MISSING_TEXT),
      ],
    };
  }

  const nextSession: OperatorSession = {
    ...session,
    binding: {
      kind: "bound",
      workspaceId: binding.workspaceId,
      userId: binding.userId,
      chatId: binding.chatId,
    },
    interaction: { kind: "idle" },
  };

  return {
    state: nextSession,
    effects: [
      {
        type: "operator.bound",
        workspaceId: binding.workspaceId,
        userId: binding.userId,
        chatId: binding.chatId,
      },
      reply(
        `Бот привязан к ${workspace.name}. Доступны основные разделы.`,
        MAIN_MENU_BUTTONS,
      ),
    ],
  };
}

function handleOperatorInteraction(
  session: OperatorSession,
  rawText: string,
  _context: OperatorContext,
): Transition<OperatorSession> | null {
  if (session.binding.kind !== "bound") {
    return null;
  }

  switch (session.interaction.kind) {
    case "idle":
      return null;
    case "awaiting_reply":
      return {
        state: { ...session, interaction: { kind: "idle" } },
        effects: [
          {
            type: "crm",
            intent: "operator.reply_sent",
            conversationId: session.interaction.conversationId,
            workspaceId: session.binding.workspaceId,
            text: rawText,
          },
          reply("Ответ отправлен клиенту.", MAIN_MENU_BUTTONS),
        ],
      };
    case "awaiting_price": {
      const amount = Number.parseInt(rawText.trim(), 10);
      if (!Number.isInteger(amount) || amount <= 0) {
        return {
          state: session,
          effects: [reply("Введите цену числом, например 4500.")],
        };
      }

      return {
        state: { ...session, interaction: { kind: "idle" } },
        effects: [
          {
            type: "booking.confirmed",
            workspaceId: session.binding.workspaceId,
            slotId: session.interaction.slotId,
            slotLabel: session.interaction.slotLabel,
            amount,
            conversationId: session.interaction.conversationId,
            ...(session.interaction.customerId
              ? { customerId: session.interaction.customerId }
              : {}),
          },
          reply(
            `Запись подтверждена: ${session.interaction.slotLabel} за ${amount} ₽.`,
            MAIN_MENU_BUTTONS,
          ),
        ],
      };
    }
  }
}

function handleBoundOperatorCommand(
  session: OperatorSession,
  command: string,
  context: OperatorContext,
): Transition<OperatorSession> {
  const binding = session.binding;
  if (binding.kind !== "bound") {
    return { state: session, effects: [reply(OPERATOR_NOT_LINKED_TEXT)] };
  }

  const workspace = context.workspaces.find(
    (candidate) => candidate.id === binding.workspaceId,
  );
  if (!workspace) {
    return {
      state: session,
      effects: [
        {
          type: "configuration.error",
          bot: "operator",
          message: OPERATOR_WORKSPACE_MISSING_TEXT,
        },
        reply(OPERATOR_WORKSPACE_MISSING_TEXT),
      ],
    };
  }

  switch (true) {
    case command === "/dashboard":
      return {
        state: session,
        effects: [formatDashboard(workspace, session.autoReplyOverride)],
      };
    case command === "/dialogs":
      return { state: session, effects: [formatDialogs(workspace.conversations)] };
    case command === "/slots":
      return { state: session, effects: [formatWeekSlots(workspace)] };
    case command === "/settings":
      return {
        state: session,
        effects: [formatSettings(workspace, session.autoReplyOverride)],
      };
    case command === "/faq":
      return { state: session, effects: [formatFaq(workspace.settings.faqQuestions)] };
    case command === "/auto_on":
      return {
        state: { ...session, autoReplyOverride: true },
        effects: [
          {
            type: "settings.auto_reply_changed",
            workspaceId: workspace.id,
            enabled: true,
          },
          reply("Автоответ включён.", MAIN_MENU_BUTTONS),
        ],
      };
    case command === "/auto_off":
      return {
        state: { ...session, autoReplyOverride: false },
        effects: [
          {
            type: "settings.auto_reply_changed",
            workspaceId: workspace.id,
            enabled: false,
          },
          reply("Автоответ выключен.", MAIN_MENU_BUTTONS),
        ],
      };
    default:
      break;
  }

  if (command.startsWith("/dialog ")) {
    return openDialog(session, workspace, command.slice("/dialog ".length).trim());
  }
  if (command.startsWith("dialog:")) {
    return openDialog(session, workspace, command.slice("dialog:".length));
  }
  if (command.startsWith("reply:")) {
    return startReplyFlow(session, workspace, command.slice("reply:".length));
  }
  if (command.startsWith("slots:")) {
    return showConversationSlots(session, workspace, command.slice("slots:".length));
  }
  if (command.startsWith("pickslot:")) {
    return startPriceFlow(session, workspace, command);
  }
  if (command.startsWith("take:")) {
    return runConversationCommand(
      session,
      workspace,
      command.slice("take:".length),
      "operator.take_dialog",
      "Диалог взят оператором.",
    );
  }
  if (command.startsWith("auto:")) {
    return runConversationCommand(
      session,
      workspace,
      command.slice("auto:".length),
      "operator.return_to_auto",
      "Диалог возвращён в автоответ.",
    );
  }
  if (command.startsWith("close:")) {
    return runConversationCommand(
      session,
      workspace,
      command.slice("close:".length),
      "operator.close_dialog",
      "Диалог закрыт.",
    );
  }

  return {
    state: session,
    effects: [reply("Доступны основные разделы.", MAIN_MENU_BUTTONS)],
  };
}

function openDialog(
  session: OperatorSession,
  workspace: OperatorWorkspace,
  conversationId: string,
): Transition<OperatorSession> {
  const conversation = findConversation(workspace, conversationId);
  if (!conversation) {
    return { state: session, effects: [reply("Диалог не найден.")] };
  }
  return { state: session, effects: [formatDialogDetails(conversation)] };
}

function startReplyFlow(
  session: OperatorSession,
  workspace: OperatorWorkspace,
  conversationId: string,
): Transition<OperatorSession> {
  const conversation = findConversation(workspace, conversationId);
  if (!conversation) {
    return { state: session, effects: [reply("Диалог не найден.")] };
  }
  return {
    state: {
      ...session,
      interaction: { kind: "awaiting_reply", conversationId },
    },
    effects: [reply("Введите ответ клиенту одним сообщением.")],
  };
}

function showConversationSlots(
  session: OperatorSession,
  workspace: OperatorWorkspace,
  conversationId: string,
): Transition<OperatorSession> {
  const conversation = findConversation(workspace, conversationId);
  if (!conversation) {
    return { state: session, effects: [reply("Диалог не найден.")] };
  }
  if (conversation.slotOptions.length === 0) {
    return { state: session, effects: [reply("Свободных вариантов нет.")] };
  }
  return {
    state: session,
    effects: [
      reply(
        "Выберите слот для подтверждения записи.",
        conversation.slotOptions.map((slot) => ({
          text: slot.label,
          action: `pickslot:${conversation.id}:${slot.id}`,
        })),
      ),
    ],
  };
}

function startPriceFlow(
  session: OperatorSession,
  workspace: OperatorWorkspace,
  command: string,
): Transition<OperatorSession> {
  const parts = command.split(":");
  if (parts.length !== 3) {
    return { state: session, effects: [reply("Слот не найден.")] };
  }
  const conversationId = parts[1];
  const slotId = parts[2];
  if (!conversationId || !slotId) {
    return { state: session, effects: [reply("Слот не найден.")] };
  }

  const conversation = findConversation(workspace, conversationId);
  const slot = conversation?.slotOptions.find((candidate) => candidate.id === slotId);
  if (!conversation || !slot) {
    return { state: session, effects: [reply("Слот не найден.")] };
  }

  return {
    state: {
      ...session,
      interaction: conversation.customerId
        ? {
            kind: "awaiting_price",
            conversationId: conversation.id,
            customerId: conversation.customerId,
            slotId: slot.id,
            slotLabel: slot.label,
          }
        : {
            kind: "awaiting_price",
            conversationId: conversation.id,
            slotId: slot.id,
            slotLabel: slot.label,
          },
    },
    effects: [reply("Введите цену для подтверждения записи, например 4500.")],
  };
}

function runConversationCommand(
  session: OperatorSession,
  workspace: OperatorWorkspace,
  conversationId: string,
  intent: "operator.take_dialog" | "operator.return_to_auto" | "operator.close_dialog",
  successText: string,
): Transition<OperatorSession> {
  const conversation = findConversation(workspace, conversationId);
  if (!conversation) {
    return { state: session, effects: [reply("Диалог не найден.")] };
  }

  return {
    state: session,
    effects: [
      {
        type: "crm",
        intent,
        conversationId: conversation.id,
        workspaceId: workspace.id,
        text: successText,
      },
      reply(successText, MAIN_MENU_BUTTONS),
    ],
  };
}

function formatDashboard(
  workspace: OperatorWorkspace,
  autoReplyOverride: boolean | undefined,
) {
  const autoReplyEnabled = autoReplyOverride ?? workspace.settings.autoReply;
  return reply(
    `📊 ${workspace.name}\n\nЗаписей сегодня: ${workspace.dashboard.todayBookings}\nНовых обращений: ${workspace.dashboard.newMessages}\nДоход: ${workspace.dashboard.revenue} ₽\nСвободных окон: ${workspace.dashboard.freeSlots}\nБлижайший слот: ${workspace.dashboard.nextSlot}\nАвтоответ: ${formatBoolean(autoReplyEnabled)}`,
    MAIN_MENU_BUTTONS,
  );
}

function formatDialogs(
  conversations: readonly OperatorConversation[],
) {
  const activeConversations = conversations.filter(
    (conversation) => conversation.status !== "closed",
  );
  if (activeConversations.length === 0) {
    return reply("Новых обращений нет.", MAIN_MENU_BUTTONS);
  }

  return reply(
    `💬 Обращения\n\n${activeConversations
      .slice(0, 8)
      .map(
        (conversation) =>
          `${conversation.title} [${conversation.provider}] (${conversation.unreadCount}) - ${conversation.lastMessageText}`,
      )
      .join("\n")}`,
    activeConversations.slice(0, 8).map((conversation) => ({
      text: conversation.title,
      action: `dialog:${conversation.id}`,
    })),
  );
}

function formatWeekSlots(workspace: OperatorWorkspace) {
  const lines =
    workspace.weekSlots.length > 0
      ? workspace.weekSlots.map(
          (slot) => `${slot.label}: ${slot.slotCount} слотов`,
        )
      : ["Свободных слотов нет."];
  return reply(`🕒 ${workspace.name}\n\n${lines.join("\n")}`, MAIN_MENU_BUTTONS);
}

function formatSettings(
  workspace: OperatorWorkspace,
  autoReplyOverride: boolean | undefined,
) {
  const autoReplyEnabled = autoReplyOverride ?? workspace.settings.autoReply;
  return reply(
    `⚙️ Настройки\n\nАвтоответ: ${formatBoolean(autoReplyEnabled)}\nHandoff: ${formatBoolean(workspace.settings.handoffEnabled)}\nTelegram chat: ${workspace.settings.telegramChatLabel}\nWebhook: ${workspace.settings.webhookUrl}`,
    [
      { text: "Включить автоответ", action: "/auto_on" },
      { text: "Выключить автоответ", action: "/auto_off" },
      { text: "FAQ", action: "/faq" },
    ],
  );
}

function formatFaq(faqQuestions: readonly string[]) {
  if (faqQuestions.length === 0) {
    return reply("FAQ пока пуст.", MAIN_MENU_BUTTONS);
  }
  return reply(
    `❓ FAQ\n\n${faqQuestions
      .map((question, index) => `${index + 1}. ${question}`)
      .join("\n")}`,
    MAIN_MENU_BUTTONS,
  );
}

function formatDialogDetails(conversation: OperatorConversation) {
  return reply(
    `👤 ${conversation.customerName}\nКанал: ${conversation.provider}\nТелефон: ${conversation.customerPhone ?? "-"}\nСтатус: ${conversation.status}\n\nПоследнее сообщение:\n${conversation.lastMessageText}`,
    [
      { text: "Ответить", action: `reply:${conversation.id}` },
      { text: "Предложить слот", action: `slots:${conversation.id}` },
      { text: "Взять диалог", action: `take:${conversation.id}` },
      { text: "Вернуть в автоответ", action: `auto:${conversation.id}` },
      { text: "Закрыть", action: `close:${conversation.id}` },
    ],
  );
}

function findConversation(
  workspace: OperatorWorkspace,
  conversationId: string,
): OperatorConversation | undefined {
  return workspace.conversations.find(
    (candidate) => candidate.id === conversationId,
  );
}

function extractLinkCode(rawInput: string): string | null {
  const trimmed = rawInput.trim();
  if (!trimmed) {
    return null;
  }
  if (trimmed.startsWith("/start ")) {
    const code = trimmed.slice("/start ".length).trim();
    return code || null;
  }
  if (trimmed.startsWith("/")) {
    return null;
  }
  return trimmed;
}

function normalizeOperatorCommand(rawInput: string): string {
  const trimmed = rawInput.trim().replace(/\s+/g, " ");
  const normalized = trimmed.toLowerCase();
  switch (normalized) {
    case "📊 дашборд":
    case "дашборд":
      return "/dashboard";
    case "💬 диалоги":
    case "🔥 новые":
    case "диалоги":
      return "/dialogs";
    case "🕒 слоты":
    case "слоты":
      return "/slots";
    case "⚙️ настройки":
    case "настройки":
      return "/settings";
    case "faq":
    case "❓ faq":
      return "/faq";
    default:
      break;
  }

  if (trimmed.startsWith("/")) {
    const separatorIndex = trimmed.indexOf(" ");
    if (separatorIndex < 0) {
      return trimmed.toLowerCase();
    }
    return `${trimmed.slice(0, separatorIndex).toLowerCase()}${trimmed.slice(separatorIndex)}`;
  }

  return trimmed;
}

function formatBoolean(value: boolean): string {
  return value ? "включен" : "выключен";
}
