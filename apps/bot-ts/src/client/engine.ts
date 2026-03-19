import { normalizeRussianPhone } from "../shared/phone.js";
import {
  claimEvent,
  reply,
  type BotButton,
  type CrmEffect,
  type DedupStore,
  type SlotOption,
  type Transition,
} from "../shared/types.js";

const MASTER_PHONE_PROMPT_COOLDOWN_MS = 2 * 60 * 1000;
const WELCOME_TEXT =
  "Здравствуйте!\nЯ помогу связаться с нужным мастером и передам ваши сообщения в CRM.\n\nВведите номер мастера, к которому хотите записаться.";
const MASTER_PHONE_PROMPT_TEXT =
  "Введите номер мастера, к которому хотите записаться.";
const INVALID_PHONE_TEXT =
  "Не удалось распознать номер мастера. Введите полный номер, например +7 999 111-22-33.";
const MASTER_NOT_FOUND_TEXT = "Мастер с таким номером не найден.";
const TELEGRAM_DISABLED_TEXT =
  "У этого мастера Telegram-канал ещё не настроен. Укажите другой номер.";
const ADDRESS_TEXT =
  "Адрес уточнит оператор в этом чате. Если адрес уже есть в FAQ, добавьте его туда для автоответа.";
const BOOKING_RECOVERY_TEXT =
  "Не удалось найти незавершённую запись. Запросите свободные окна заново.";
const WORKSPACE_CONFIGURATION_ERROR =
  "Workspace bot настроен не полностью. Проверьте fixed workspace configuration.";

export interface ClientMasterDirectoryEntry {
  readonly workspaceId: string;
  readonly workspaceName: string;
  readonly masterPhone: string;
  readonly telegramEnabled: boolean;
}

export interface ClientFixedWorkspace {
  readonly workspaceId: string;
  readonly workspaceName: string;
  readonly masterPhone?: string;
}

export interface ClientContext {
  readonly mode: "global" | "workspace";
  readonly masters: readonly ClientMasterDirectoryEntry[];
  readonly slotsByWorkspace: Readonly<Record<string, readonly SlotOption[]>>;
  readonly fixedWorkspace?: ClientFixedWorkspace;
  readonly dedupStore?: DedupStore;
  readonly dedupNamespace?: string;
  readonly dedupTtlSeconds?: number;
  readonly promptCooldownMs?: number;
}

export type ClientRouteState =
  | { readonly kind: "awaiting_master_phone"; readonly promptedAt: string }
  | {
      readonly kind: "ready";
      readonly workspaceId: string;
      readonly workspaceName: string;
      readonly masterPhone?: string;
    };

export type ClientBookingState =
  | { readonly kind: "idle" }
  | {
      readonly kind: "awaiting_confirmation";
      readonly workspaceId: string;
      readonly slotId: string;
      readonly slotLabel: string;
    };

export interface ClientSession {
  readonly route: ClientRouteState;
  readonly booking: ClientBookingState;
  readonly recentEventIds: readonly string[];
}

export type ClientEvent =
  | {
      readonly type: "start";
      readonly eventId?: string;
      readonly payload?: string;
      readonly now?: Date;
    }
  | {
      readonly type: "message";
      readonly eventId?: string;
      readonly text: string;
      readonly now?: Date;
    }
  | {
      readonly type: "callback";
      readonly eventId?: string;
      readonly data: string;
      readonly now?: Date;
    };

const RETRY_PHONE_BUTTONS: readonly BotButton[] = [
  { text: "Повторить", action: "client:enter_master_phone" },
];

const RETRY_OR_HUMAN_BUTTONS: readonly BotButton[] = [
  { text: "Повторить", action: "client:enter_master_phone" },
  { text: "Связаться с человеком", action: "client:human" },
];

const BOOKING_RECOVERY_BUTTONS: readonly BotButton[] = [
  { text: "Свободные окна", action: "client:slots" },
];

const CLIENT_TEXT_ACTIONS = new Map<string, string>([
  ["записаться", "client:book"],
  ["свободные окна", "client:slots"],
  ["цены", "client:prices"],
  ["адрес", "client:address"],
  ["связаться с человеком", "client:human"],
  ["сменить мастера", "client:change_master"],
  ["ввести номер мастера", "client:enter_master_phone"],
  ["введите номер мастера", "client:enter_master_phone"],
  ["номер мастера", "client:enter_master_phone"],
  [MASTER_PHONE_PROMPT_TEXT.toLowerCase(), "client:enter_master_phone"],
  ["отмена", "booking:cancel"],
]);

export function createClientSession(): ClientSession {
  return {
    route: {
      kind: "awaiting_master_phone",
      promptedAt: new Date(0).toISOString(),
    },
    booking: { kind: "idle" },
    recentEventIds: [],
  };
}

export async function handleClientEvent(
  rawSession: ClientSession | undefined,
  event: ClientEvent,
  context: ClientContext,
): Promise<Transition<ClientSession>> {
  const currentSession = rawSession ?? createClientSession();
  const dedupResult = await claimEvent(
    currentSession,
    context.dedupNamespace ?? "client",
    event.eventId,
    context.dedupStore,
    context.dedupTtlSeconds,
  );
  if (!dedupResult.claimed) {
    return { state: dedupResult.state, effects: [] };
  }

  const session = dedupResult.state;
  const now = event.now ?? new Date();

  switch (event.type) {
    case "start":
      return handleClientStart(session, event.payload, now, context);
    case "message":
      return handleClientMessage(session, event.text, now, context);
    case "callback":
      return handleClientAction(session, event.data, now, context);
  }
}

function handleClientStart(
  session: ClientSession,
  payload: string | undefined,
  now: Date,
  context: ClientContext,
): Transition<ClientSession> {
  if (context.mode === "workspace") {
    return activateFixedWorkspace(session, context, true);
  }

  if (payload?.trim()) {
    return selectMasterByPhone(session, payload, now, context);
  }

  return promptForMasterPhone(session, now, true, context);
}

function handleClientMessage(
  session: ClientSession,
  text: string,
  now: Date,
  context: ClientContext,
): Transition<ClientSession> {
  const normalizedText = normalizeText(text);
  const action = CLIENT_TEXT_ACTIONS.get(normalizedText);
  if (action) {
    return handleClientAction(session, action, now, context);
  }

  if (context.mode === "workspace") {
    const readySessionResult = ensureWorkspaceRoute(session, context);
    if (!readySessionResult.ok) {
      return readySessionResult.transition;
    }

    if (text.trim()) {
      return forwardClientMessage(readySessionResult.session, readySessionResult.route, text);
    }
    return { state: readySessionResult.session, effects: [] };
  }

  if (session.route.kind === "ready") {
    if (!text.trim()) {
      return { state: session, effects: [] };
    }
    return forwardClientMessage(session, session.route, text);
  }

  return selectMasterByPhone(session, text, now, context);
}

function handleClientAction(
  session: ClientSession,
  action: string,
  now: Date,
  context: ClientContext,
): Transition<ClientSession> {
  if (action === "client:change_master" || action === "client:enter_master_phone") {
    if (context.mode === "workspace") {
      return activateFixedWorkspace(session, context, false);
    }
    return promptForMasterPhone(session, now, false, context);
  }

  if (action === "client:human") {
    if (context.mode === "workspace") {
      const workspaceResult = ensureWorkspaceRoute(session, context);
      if (!workspaceResult.ok) {
        return workspaceResult.transition;
      }

      return {
        state: workspaceResult.session,
        effects: [
          {
            type: "crm",
            intent: "client.human_requested",
            workspaceId: workspaceResult.route.workspaceId,
            text: "Хочу связаться с человеком",
          },
        ],
      };
    }

    const workspaceId =
      session.route.kind === "ready" ? session.route.workspaceId : undefined;
    return {
      state: session,
      effects: [
        {
          type: "crm",
          intent: "client.human_requested",
          ...(workspaceId ? { workspaceId } : {}),
          text: "Хочу связаться с человеком",
        },
      ],
    };
  }

  const readySessionResult = ensureReadyRoute(session, now, context);
  if (!readySessionResult.ok) {
    return readySessionResult.transition;
  }

  const readySession = readySessionResult.session;
  const route = readySessionResult.route;

  switch (action) {
    case "client:book":
      return showAvailableSlots(readySession, route, context, true);
    case "client:slots":
      return showAvailableSlots(readySession, route, context, false);
    case "client:prices":
      return {
        state: readySession,
        effects: [
          {
            type: "crm",
            intent: "client.prices_requested",
            workspaceId: route.workspaceId,
            text: "Сколько стоит?",
          },
        ],
      };
    case "client:address":
      return {
        state: readySession,
        effects: [reply(ADDRESS_TEXT, buildClientMenuButtons(context.mode))],
      };
    case "booking:cancel":
      return cancelBooking(readySession, route, context);
    default:
      break;
  }

  if (action.startsWith("slot:select:")) {
    return startBookingConfirmation(readySession, route, action, context);
  }

  if (action.startsWith("booking:confirm:")) {
    return confirmBooking(readySession, route, action, context);
  }

  return { state: readySession, effects: [] };
}

function promptForMasterPhone(
  session: ClientSession,
  now: Date,
  welcome: boolean,
  context: ClientContext,
): Transition<ClientSession> {
  const cooldownMs = context.promptCooldownMs ?? MASTER_PHONE_PROMPT_COOLDOWN_MS;
  if (
    session.route.kind === "awaiting_master_phone" &&
    now.getTime() - new Date(session.route.promptedAt).getTime() < cooldownMs
  ) {
    return { state: session, effects: [] };
  }

  const nextSession: ClientSession = {
    ...session,
    route: { kind: "awaiting_master_phone", promptedAt: now.toISOString() },
    booking: { kind: "idle" },
  };

  return {
    state: nextSession,
    effects: [
      reply(
        welcome ? WELCOME_TEXT : MASTER_PHONE_PROMPT_TEXT,
        [{ text: "Ввести номер мастера", action: "client:enter_master_phone" }],
      ),
    ],
  };
}

function selectMasterByPhone(
  session: ClientSession,
  rawPhone: string,
  now: Date,
  context: ClientContext,
): Transition<ClientSession> {
  const normalizedPhone = normalizeRussianPhone(rawPhone);
  if (!normalizedPhone) {
    return {
      state: {
        ...session,
        route: { kind: "awaiting_master_phone", promptedAt: now.toISOString() },
        booking: { kind: "idle" },
      },
      effects: [reply(INVALID_PHONE_TEXT, RETRY_PHONE_BUTTONS)],
    };
  }

  const master = context.masters.find(
    (candidate) => candidate.masterPhone === normalizedPhone,
  );
  if (!master) {
    return {
      state: {
        ...session,
        route: { kind: "awaiting_master_phone", promptedAt: now.toISOString() },
        booking: { kind: "idle" },
      },
      effects: [reply(MASTER_NOT_FOUND_TEXT, RETRY_OR_HUMAN_BUTTONS)],
    };
  }

  if (!master.telegramEnabled) {
    return {
      state: {
        ...session,
        route: { kind: "awaiting_master_phone", promptedAt: now.toISOString() },
        booking: { kind: "idle" },
      },
      effects: [reply(TELEGRAM_DISABLED_TEXT, RETRY_PHONE_BUTTONS)],
    };
  }

  const nextRoute: ClientRouteState = {
    kind: "ready",
    workspaceId: master.workspaceId,
    workspaceName: master.workspaceName,
    masterPhone: master.masterPhone,
  };
  const nextSession: ClientSession = {
    ...session,
    route: nextRoute,
    booking: { kind: "idle" },
  };

  return {
    state: nextSession,
    effects: [
      {
        type: "client.route_selected",
        workspaceId: master.workspaceId,
        workspaceName: master.workspaceName,
        masterPhone: master.masterPhone,
      },
      reply(
        `Мастер выбран: ${master.workspaceName}.\nТеперь можно написать сообщение, посмотреть слоты или записаться.`,
        buildClientMenuButtons(context.mode),
      ),
    ],
  };
}

function activateFixedWorkspace(
  session: ClientSession,
  context: ClientContext,
  welcome: boolean,
): Transition<ClientSession> {
  const fixedWorkspace = validateFixedWorkspace(context.fixedWorkspace);
  if (!fixedWorkspace) {
    return configurationError(session);
  }

  const nextRoute: ClientRouteState = fixedWorkspace.masterPhone
    ? {
        kind: "ready",
        workspaceId: fixedWorkspace.workspaceId,
        workspaceName: fixedWorkspace.workspaceName,
        masterPhone: fixedWorkspace.masterPhone,
      }
    : {
        kind: "ready",
        workspaceId: fixedWorkspace.workspaceId,
        workspaceName: fixedWorkspace.workspaceName,
      };

  const nextSession: ClientSession = {
    ...session,
    route: nextRoute,
    booking: { kind: "idle" },
  };

  return {
    state: nextSession,
    effects: [
      {
        type: "client.route_selected",
        workspaceId: fixedWorkspace.workspaceId,
        workspaceName: fixedWorkspace.workspaceName,
        ...(fixedWorkspace.masterPhone
          ? { masterPhone: fixedWorkspace.masterPhone }
          : {}),
      },
      reply(
        welcome
          ? `Здравствуйте!\nБот подключён к ${fixedWorkspace.workspaceName}.\nМожно написать сообщение, посмотреть слоты или записаться.`
          : `Бот уже привязан к ${fixedWorkspace.workspaceName}.`,
        buildClientMenuButtons(context.mode),
      ),
    ],
  };
}

function showAvailableSlots(
  session: ClientSession,
  route: Extract<ClientRouteState, { kind: "ready" }>,
  context: ClientContext,
  bookingMode: boolean,
): Transition<ClientSession> {
  const availableSlots = context.slotsByWorkspace[route.workspaceId] ?? [];
  const effects: (CrmEffect | ReturnType<typeof reply>)[] = [
    {
      type: "crm",
      intent: "client.slots_requested",
      workspaceId: route.workspaceId,
      text: bookingMode ? "Хочу записаться" : "Покажите свободные окна",
    },
  ];

  if (availableSlots.length > 0) {
    effects.push(
      reply(
        bookingMode ? "Выберите время для записи." : "Свободные окна:",
        availableSlots.slice(0, 8).map((slot) => ({
          text: slot.label,
          action: `slot:select:${slot.id}`,
        })),
      ),
    );
  }

  return { state: session, effects };
}

function startBookingConfirmation(
  session: ClientSession,
  route: Extract<ClientRouteState, { kind: "ready" }>,
  action: string,
  context: ClientContext,
): Transition<ClientSession> {
  const slotId = action.slice("slot:select:".length);
  const slot = (context.slotsByWorkspace[route.workspaceId] ?? []).find(
    (candidate) => candidate.id === slotId,
  );
  if (!slot) {
    return {
      state: session,
      effects: [
        reply(
          "Этот слот уже недоступен. Запросите свободные окна ещё раз.",
          BOOKING_RECOVERY_BUTTONS,
        ),
      ],
    };
  }

  const nextSession: ClientSession = {
    ...session,
    booking: {
      kind: "awaiting_confirmation",
      workspaceId: route.workspaceId,
      slotId: slot.id,
      slotLabel: slot.label,
    },
  };

  return {
    state: nextSession,
    effects: [
      {
        type: "booking.pending",
        workspaceId: route.workspaceId,
        slotId: slot.id,
        slotLabel: slot.label,
      },
      reply("Подтвердить выбранное время?", [
        { text: "Подтвердить", action: `booking:confirm:${slot.id}` },
        { text: "Отмена", action: "booking:cancel" },
      ]),
    ],
  };
}

function confirmBooking(
  session: ClientSession,
  route: Extract<ClientRouteState, { kind: "ready" }>,
  action: string,
  context: ClientContext,
): Transition<ClientSession> {
  const slotId = action.slice("booking:confirm:".length);
  if (
    session.booking.kind !== "awaiting_confirmation" ||
    session.booking.slotId !== slotId
  ) {
    return {
      state: { ...session, booking: { kind: "idle" } },
      effects: [reply(BOOKING_RECOVERY_TEXT, BOOKING_RECOVERY_BUTTONS)],
    };
  }

  const nextSession: ClientSession = {
    ...session,
    booking: { kind: "idle" },
  };

  return {
    state: nextSession,
    effects: [
      {
        type: "booking.confirmed",
        workspaceId: route.workspaceId,
        slotId: session.booking.slotId,
        slotLabel: session.booking.slotLabel,
      },
      reply(
        `Запись подтверждена: ${session.booking.slotLabel}.`,
        buildClientMenuButtons(context.mode),
      ),
    ],
  };
}

function cancelBooking(
  session: ClientSession,
  route: Extract<ClientRouteState, { kind: "ready" }>,
  context: ClientContext,
): Transition<ClientSession> {
  if (session.booking.kind !== "awaiting_confirmation") {
    return {
      state: session,
      effects: [reply("Действие отменено.", buildClientMenuButtons(context.mode))],
    };
  }

  const nextSession: ClientSession = {
    ...session,
    booking: { kind: "idle" },
  };

  return {
    state: nextSession,
    effects: [
      {
        type: "booking.cancelled",
        workspaceId: route.workspaceId,
        slotId: session.booking.slotId,
      },
      reply("Действие отменено.", buildClientMenuButtons(context.mode)),
    ],
  };
}

function forwardClientMessage(
  session: ClientSession,
  route: Extract<ClientRouteState, { kind: "ready" }>,
  text: string,
): Transition<ClientSession> {
  return {
    state: session,
    effects: [
      {
        type: "crm",
        intent: "client.message_forwarded",
        workspaceId: route.workspaceId,
        text,
      },
    ],
  };
}

function ensureReadyRoute(
  session: ClientSession,
  now: Date,
  context: ClientContext,
):
  | {
      readonly ok: true;
      readonly session: ClientSession;
      readonly route: Extract<ClientRouteState, { kind: "ready" }>;
    }
  | { readonly ok: false; readonly transition: Transition<ClientSession> } {
  if (context.mode === "workspace") {
    return ensureWorkspaceRoute(session, context);
  }

  if (session.route.kind === "ready") {
    return { ok: true, session, route: session.route };
  }

  return {
    ok: false,
    transition: promptForMasterPhone(session, now, true, context),
  };
}

function ensureWorkspaceRoute(
  session: ClientSession,
  context: ClientContext,
):
  | {
      readonly ok: true;
      readonly session: ClientSession;
      readonly route: Extract<ClientRouteState, { kind: "ready" }>;
    }
  | { readonly ok: false; readonly transition: Transition<ClientSession> } {
  const fixedWorkspace = validateFixedWorkspace(context.fixedWorkspace);
  if (!fixedWorkspace) {
    return { ok: false, transition: configurationError(session) };
  }

  const route: Extract<ClientRouteState, { kind: "ready" }> = fixedWorkspace.masterPhone
    ? {
        kind: "ready",
        workspaceId: fixedWorkspace.workspaceId,
        workspaceName: fixedWorkspace.workspaceName,
        masterPhone: fixedWorkspace.masterPhone,
      }
    : {
        kind: "ready",
        workspaceId: fixedWorkspace.workspaceId,
        workspaceName: fixedWorkspace.workspaceName,
      };

  if (
    session.route.kind === "ready" &&
    session.route.workspaceId === route.workspaceId &&
    session.route.workspaceName === route.workspaceName &&
    session.route.masterPhone === route.masterPhone
  ) {
    return { ok: true, session, route: session.route };
  }

  return {
    ok: true,
    session: {
      ...session,
      route,
      booking: { kind: "idle" },
    },
    route,
  };
}

function configurationError(session: ClientSession): Transition<ClientSession> {
  return {
    state: session,
    effects: [
      {
        type: "configuration.error",
        bot: "client",
        message: WORKSPACE_CONFIGURATION_ERROR,
      },
      reply(WORKSPACE_CONFIGURATION_ERROR),
    ],
  };
}

function validateFixedWorkspace(
  fixedWorkspace: ClientFixedWorkspace | undefined,
): ClientFixedWorkspace | null {
  if (!fixedWorkspace) {
    return null;
  }
  if (!fixedWorkspace.workspaceId.trim() || !fixedWorkspace.workspaceName.trim()) {
    return null;
  }
  return fixedWorkspace;
}

function buildClientMenuButtons(
  mode: ClientContext["mode"],
): readonly BotButton[] {
  const buttons: BotButton[] = [
    { text: "Записаться", action: "client:book" },
    { text: "Свободные окна", action: "client:slots" },
    { text: "Цены", action: "client:prices" },
    { text: "Адрес", action: "client:address" },
    { text: "Связаться с человеком", action: "client:human" },
  ];
  if (mode === "global") {
    buttons.push({ text: "Сменить мастера", action: "client:change_master" });
  }
  return buttons;
}

function normalizeText(text: string): string {
  return text.trim().replace(/\s+/g, " ").toLowerCase();
}
