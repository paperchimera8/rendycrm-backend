import { createServer } from "node:http";
import type { NextFunction, Request, Response } from "express";
import express from "express";

import { issueToken, parseToken } from "./auth.js";
import { loadConfig } from "./config.js";
import { TelegramBotApi, type TelegramButton, type TelegramUpdate } from "./telegram.js";

type AuthenticatedRequest = Request & {
  email?: string;
};

type BotReply = {
  text: string;
  buttons?: TelegramButton[];
  callbackNotice?: string;
};

class ExpiringDeduplicator {
  private readonly items = new Map<string, number>();
  private readonly ttlMs: number;

  constructor(ttlMs: number) {
    this.ttlMs = ttlMs;
    const timer = setInterval(() => this.cleanup(), Math.min(ttlMs, 60_000));
    timer.unref();
  }

  claim(key: string): boolean {
    const now = Date.now();
    this.cleanup(now);
    const expiresAt = this.items.get(key);
    if (expiresAt !== undefined && expiresAt > now) {
      return false;
    }
    this.items.set(key, now + this.ttlMs);
    return true;
  }

  private cleanup(now = Date.now()): void {
    for (const [key, expiresAt] of this.items.entries()) {
      if (expiresAt <= now) {
        this.items.delete(key);
      }
    }
  }
}

const config = loadConfig();
const app = express();
const deduplicator = new ExpiringDeduplicator(2 * 60_000);

app.disable("x-powered-by");
app.use(requestLogger);
app.use(corsMiddleware);
app.use(express.json({ limit: "1mb" }));

app.get("/health", (_req, res) => {
  res.json({ status: "ok" });
});

app.head("/health", (_req, res) => {
  res.status(200).end();
});

app.post("/auth/login", (req, res) => {
  const email = typeof req.body?.email === "string" ? req.body.email.trim() : "";
  const password = typeof req.body?.password === "string" ? req.body.password : "";
  if (!email || !password) {
    res.status(400).json({ error: "invalid payload" });
    return;
  }
  if (email.toLowerCase() !== config.adminEmail.toLowerCase() || password !== config.adminPassword) {
    res.status(401).json({ error: "invalid credentials" });
    return;
  }

  const { token, expiresAt } = issueToken(config.authSecret, config.adminEmail, 24 * 60 * 60 * 1000);
  res.json({
    token,
    expiresAt: expiresAt.toISOString(),
    user: {
      id: "usr_1",
      email: config.adminEmail,
      name: "Main Master",
      role: "admin",
    },
  });
});

app.get("/auth/me", authMiddleware, (req: AuthenticatedRequest, res) => {
  res.json({
    user: {
      id: "usr_1",
      email: req.email,
      name: "Main Master",
      role: "admin",
    },
    workspace: {
      id: "ws_main",
      name: "Rendy CRM",
    },
  });
});

app.post("/webhooks/telegram/client/:channelAccountId/:secret", asyncHandler(async (req, res) => {
  const update = parseTelegramUpdate(req.body);
  if (botRuntimeProxyModeEnabled()) {
    if (!authorizeBotRuntimeProxyRequest(req, res)) {
      return;
    }
    const dedupKey = `client:${req.params.channelAccountId}:${update.update_id}`;
    const handled = deduplicator.claim(dedupKey);
    res.json({ ok: true, handled, accepted: true });
    if (!handled) {
      return;
    }
    void forwardClientUpdate(update, req.params.channelAccountId, req.params.secret).catch((error: unknown) => {
      console.error("client bot runtime forward failed", error);
    });
    return;
  }
  if (config.clientWebhookSecret && req.params.secret !== config.clientWebhookSecret) {
    res.status(401).json({ error: "invalid webhook secret" });
    return;
  }
  const bot = requireBot(config.clientBotToken, "TELEGRAM_CLIENT_BOT_TOKEN");
  const handled = await handleClientUpdate(bot, update, req.params.channelAccountId);
  res.json({ ok: true, handled });
}));

app.post("/webhooks/telegram/operator", asyncHandler(async (req, res) => {
  const secret = req.header("X-Telegram-Bot-Api-Secret-Token")?.trim() ?? "";
  const update = parseTelegramUpdate(req.body);
  if (botRuntimeProxyModeEnabled()) {
    if (!authorizeBotRuntimeProxyRequest(req, res)) {
      return;
    }
    const dedupKey = `operator:${secret}:${update.update_id}`;
    const handled = deduplicator.claim(dedupKey);
    res.json({ ok: true, handled, accepted: true });
    if (!handled) {
      return;
    }
    void forwardOperatorUpdate(update, secret).catch((error: unknown) => {
      console.error("operator bot runtime forward failed", error);
    });
    return;
  }
  if (config.operatorWebhookSecret && secret !== config.operatorWebhookSecret) {
    res.status(401).json({ error: "invalid operator webhook secret" });
    return;
  }
  const bot = requireBot(config.operatorBotToken, "TELEGRAM_OPERATOR_BOT_TOKEN");
  const handled = await handleOperatorUpdate(bot, update);
  res.json({ ok: true, handled });
}));

app.use((error: unknown, _req: Request, res: Response, _next: NextFunction) => {
  const message =
    error instanceof SyntaxError
      ? "invalid payload"
      : error instanceof Error && error.message.trim() !== ""
        ? error.message
        : "internal server error";
  const status = error instanceof SyntaxError ? 400 : 500;
  if (status === 500) {
    console.error(error);
  }
  res.status(status).json({ error: message });
});

const server = createServer(app);
server.listen(config.port, () => {
  console.log(`backend listening on :${config.port}`);
});

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.on(signal, () => {
    server.close((error) => {
      if (error) {
        console.error(error);
        process.exit(1);
        return;
      }
      process.exit(0);
    });
  });
}

function authMiddleware(req: AuthenticatedRequest, res: Response, next: NextFunction): void {
  const auth = req.header("Authorization")?.trim() ?? "";
  if (auth === "") {
    res.status(401).json({ error: "missing token" });
    return;
  }
  if (!auth.startsWith("Bearer ")) {
    res.status(401).json({ error: "invalid auth header" });
    return;
  }
  const token = auth.slice("Bearer ".length).trim();
  if (token === "") {
    res.status(401).json({ error: "invalid auth header" });
    return;
  }

  try {
    const claims = parseToken(config.authSecret, token);
    req.email = claims.email;
    next();
  } catch {
    res.status(401).json({ error: "invalid token" });
  }
}

function requestLogger(req: Request, res: Response, next: NextFunction): void {
  const startedAt = Date.now();
  res.on("finish", () => {
    console.log(`${req.method} ${req.originalUrl} ${res.statusCode} ${Date.now() - startedAt}ms`);
  });
  next();
}

function corsMiddleware(req: Request, res: Response, next: NextFunction): void {
  const origin = req.header("Origin")?.trim() ?? "";
  const allowedOrigin = resolveAllowedOrigin(origin);
  if (allowedOrigin) {
    res.setHeader("Access-Control-Allow-Origin", allowedOrigin);
    res.setHeader("Vary", "Origin");
  }
  if (config.corsAllowCredentials && allowedOrigin && allowedOrigin !== "*") {
    res.setHeader("Access-Control-Allow-Credentials", "true");
  }
  res.setHeader("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS");
  res.setHeader(
    "Access-Control-Allow-Headers",
    req.header("Access-Control-Request-Headers")?.trim() ||
      "Authorization, Content-Type, X-Telegram-Bot-Api-Secret-Token",
  );
  res.setHeader("Access-Control-Max-Age", "600");

  if (req.method === "OPTIONS") {
    res.status(204).end();
    return;
  }
  next();
}

function resolveAllowedOrigin(origin: string): string {
  const allowAll =
    config.corsAllowedOrigins.length === 0 ||
    (config.corsAllowedOrigins.length === 1 && config.corsAllowedOrigins[0] === "*");
  if (allowAll) {
    return config.corsAllowCredentials && origin ? origin : "*";
  }
  if (!origin) {
    return "";
  }

  try {
    const originUrl = new URL(origin);
    for (const pattern of config.corsAllowedOrigins) {
      if (pattern === origin) {
        return origin;
      }
      const match = pattern.match(/^([a-z]+):\/\/\*\.(.+)$/i);
      if (!match) {
        continue;
      }
      const [, scheme, suffix] = match;
      if (originUrl.protocol !== `${scheme}:`) {
        continue;
      }
      const host = originUrl.hostname;
      if (host === suffix || host.endsWith(`.${suffix}`)) {
        return origin;
      }
    }
  } catch {
    return "";
  }
  return "";
}

function asyncHandler(
  handler: (req: Request, res: Response, next: NextFunction) => Promise<void>,
): (req: Request, res: Response, next: NextFunction) => void {
  return (req, res, next) => {
    void handler(req, res, next).catch(next);
  };
}

function parseTelegramUpdate(value: unknown): TelegramUpdate {
  if (!value || typeof value !== "object") {
    throw new Error("invalid telegram update");
  }
  const update = value as Partial<TelegramUpdate>;
  if (typeof update.update_id !== "number" || !Number.isFinite(update.update_id)) {
    throw new Error("invalid telegram update");
  }
  return update as TelegramUpdate;
}

function requireBot(token: string, envName: string): TelegramBotApi {
  if (!token) {
    throw new Error(`${envName} is not configured`);
  }
  return new TelegramBotApi(token, config.telegramApiBaseUrl);
}

function botRuntimeProxyModeEnabled(): boolean {
  return config.goApiBaseUrl.trim() !== "";
}

function authorizeBotRuntimeProxyRequest(req: Request, res: Response): boolean {
  if (!botRuntimeProxyModeEnabled()) {
    return true;
  }
  const provided = req.header("X-Bot-Runtime-Secret")?.trim() ?? "";
  if (provided === "" || provided !== config.botRuntimeInternalSecret) {
    res.status(401).json({ error: "invalid bot runtime secret" });
    return false;
  }
  return true;
}

async function forwardClientUpdate(
  update: TelegramUpdate,
  channelAccountId: string,
  secret: string,
): Promise<void> {
  await postGoJSON(
    `/internal/bot-runtime/telegram/client/${encodeURIComponent(channelAccountId)}/${encodeURIComponent(secret)}`,
    update,
  );
}

async function forwardOperatorUpdate(update: TelegramUpdate, operatorSecret: string): Promise<void> {
  await postGoJSON("/internal/bot-runtime/telegram/operator", update, {
    "X-Telegram-Bot-Api-Secret-Token": operatorSecret,
  });
}

async function postGoJSON(
  path: string,
  payload: unknown,
  extraHeaders: Record<string, string> = {},
): Promise<void> {
  const response = await fetch(`${config.goApiBaseUrl}${path}`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "X-Bot-Runtime-Secret": config.botRuntimeInternalSecret,
      ...extraHeaders,
    },
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(20_000),
  });
  if (!response.ok) {
    const body = await response.text().catch(() => "");
    const suffix = body.trim() !== "" ? `: ${body.trim()}` : "";
    throw new Error(`go bot runtime request failed with HTTP ${response.status}${suffix}`);
  }
}

async function handleClientUpdate(bot: TelegramBotApi, update: TelegramUpdate, workspace: string): Promise<boolean> {
  if (!deduplicator.claim(`client:${update.update_id}`)) {
    return false;
  }

  if (update.callback_query?.message?.chat?.id && typeof update.callback_query.data === "string") {
    const chatId = update.callback_query.message.chat.id;
    const reply = resolveClientCallback(update.callback_query.data, workspace);
    await bot.answerCallbackQuery(update.callback_query.id, reply.callbackNotice ?? "Принято");
    await bot.sendMessage(chatId, reply.text, reply.buttons ?? clientMenuButtons());
    return true;
  }

  if (update.message?.chat?.id && typeof update.message.text === "string") {
    const chatId = update.message.chat.id;
    const reply = resolveClientText(update.message.text, workspace);
    await bot.sendMessage(chatId, reply.text, reply.buttons ?? clientMenuButtons());
    return true;
  }

  return false;
}

async function handleOperatorUpdate(bot: TelegramBotApi, update: TelegramUpdate): Promise<boolean> {
  if (!deduplicator.claim(`operator:${update.update_id}`)) {
    return false;
  }

  if (update.callback_query?.message?.chat?.id && typeof update.callback_query.data === "string") {
    const chatId = update.callback_query.message.chat.id;
    const reply = resolveOperatorCommand(update.callback_query.data);
    await bot.answerCallbackQuery(update.callback_query.id, reply.callbackNotice ?? "Открыто");
    await bot.sendMessage(chatId, reply.text, reply.buttons ?? operatorMenuButtons());
    return true;
  }

  if (update.message?.chat?.id && typeof update.message.text === "string") {
    const chatId = update.message.chat.id;
    const reply = resolveOperatorCommand(update.message.text);
    await bot.sendMessage(chatId, reply.text, reply.buttons ?? operatorMenuButtons());
    return true;
  }

  return false;
}

function resolveClientText(rawText: string, workspace: string): BotReply {
  const text = rawText.trim();
  const normalized = text.toLowerCase();
  if (text === "/start" || text === "/help") {
    return {
      text: formatWorkspaceAwareMessage(config.clientWelcomeMessage, workspace),
      buttons: clientMenuButtons(),
    };
  }
  if (includesAny(normalized, ["запис", "окно", "слот", "свобод"])) {
    return resolveClientCallback("client:slots", workspace);
  }
  if (includesAny(normalized, ["цена", "стоим", "прайс", "сколько"])) {
    return resolveClientCallback("client:prices", workspace);
  }
  if (includesAny(normalized, ["адрес", "где", "локац"])) {
    return resolveClientCallback("client:address", workspace);
  }
  if (includesAny(normalized, ["оператор", "человек", "менеджер"])) {
    return resolveClientCallback("client:human", workspace);
  }
  return {
    text: `Сообщение получено: "${text}". Выберите действие ниже или уточните вопрос.`,
    buttons: clientMenuButtons(),
  };
}

function resolveClientCallback(command: string, workspace: string): BotReply {
  switch (command.trim()) {
    case "client:book":
      return {
        text: `${formatWorkspaceAwareMessage(config.clientSlotsMessage, workspace)}\n\nЕсли готовы, напишите удобный день и время.`,
        buttons: clientMenuButtons(),
        callbackNotice: "Показываю варианты",
      };
    case "client:slots":
      return {
        text: formatWorkspaceAwareMessage(config.clientSlotsMessage, workspace),
        buttons: clientMenuButtons(),
        callbackNotice: "Смотрю окна",
      };
    case "client:prices":
      return {
        text: formatWorkspaceAwareMessage(config.clientPriceMessage, workspace),
        buttons: clientMenuButtons(),
        callbackNotice: "Отправляю цены",
      };
    case "client:address":
      return {
        text: formatWorkspaceAwareMessage(config.clientAddressMessage, workspace),
        buttons: clientMenuButtons(),
        callbackNotice: "Отправляю адрес",
      };
    case "client:human":
      return {
        text: formatWorkspaceAwareMessage(config.clientHandoffMessage, workspace),
        buttons: clientMenuButtons(),
        callbackNotice: "Передаю оператору",
      };
    default:
      return {
        text: formatWorkspaceAwareMessage(config.clientWelcomeMessage, workspace),
        buttons: clientMenuButtons(),
      };
  }
}

function resolveOperatorCommand(rawText: string): BotReply {
  switch (rawText.trim()) {
    case "/start":
      return {
        text: config.operatorWelcomeMessage,
        buttons: operatorMenuButtons(),
      };
    case "/dashboard":
      return {
        text: "Dashboard доступен. Подключите CRM-данные, если нужен реальный список метрик.",
        buttons: operatorMenuButtons(),
        callbackNotice: "Открываю dashboard",
      };
    case "/dialogs":
      return {
        text: "Dialogs доступны. Подключите CRM-данные, если нужен реальный список диалогов.",
        buttons: operatorMenuButtons(),
        callbackNotice: "Открываю диалоги",
      };
    case "/slots":
      return {
        text: "Slots доступны. Подключите расписание, если нужны реальные свободные окна.",
        buttons: operatorMenuButtons(),
        callbackNotice: "Открываю слоты",
      };
    case "/settings":
      return {
        text: "Settings доступны. Здесь можно держать ссылки на конфиг и состояние интеграций.",
        buttons: operatorMenuButtons(),
        callbackNotice: "Открываю настройки",
      };
    default:
      return {
        text: "Команда принята. Доступны: /dashboard, /dialogs, /slots, /settings.",
        buttons: operatorMenuButtons(),
      };
  }
}

function clientMenuButtons(): TelegramButton[] {
  return [
    { text: "Записаться", callback_data: "client:book" },
    { text: "Свободные окна", callback_data: "client:slots" },
    { text: "Цены", callback_data: "client:prices" },
    { text: "Адрес", callback_data: "client:address" },
    { text: "Связаться с человеком", callback_data: "client:human" },
  ];
}

function operatorMenuButtons(): TelegramButton[] {
  return [
    { text: "Дашборд", callback_data: "/dashboard" },
    { text: "Диалоги", callback_data: "/dialogs" },
    { text: "Слоты", callback_data: "/slots" },
    { text: "Настройки", callback_data: "/settings" },
  ];
}

function includesAny(value: string, probes: string[]): boolean {
  return probes.some((probe) => value.includes(probe));
}

function formatWorkspaceAwareMessage(message: string, workspace: string): string {
  const trimmedWorkspace = workspace.trim();
  if (!trimmedWorkspace) {
    return message;
  }
  return message.replaceAll("{workspace}", trimmedWorkspace);
}
