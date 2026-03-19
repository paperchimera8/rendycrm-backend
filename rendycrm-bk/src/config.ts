import { existsSync, readFileSync } from "node:fs";

export type Config = {
  port: number;
  authSecret: string;
  adminEmail: string;
  adminPassword: string;
  corsAllowedOrigins: string[];
  corsAllowCredentials: boolean;
  telegramApiBaseUrl: string;
  clientWebhookSecret: string;
  operatorWebhookSecret: string;
  clientBotToken: string;
  operatorBotToken: string;
  clientWelcomeMessage: string;
  clientSlotsMessage: string;
  clientPriceMessage: string;
  clientAddressMessage: string;
  clientHandoffMessage: string;
  operatorWelcomeMessage: string;
};

const defaultAuthSecret = "change-me-now";
const defaultAdminPassword = "password";

function envOrDefault(name: string, fallback: string): string {
  const value = process.env[name]?.trim();
  return value === undefined || value === "" ? fallback : value;
}

function loadDotEnv(path: string): void {
  if (!existsSync(path)) {
    return;
  }
  const content = readFileSync(path, "utf8");
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    const separatorIndex = line.indexOf("=");
    if (separatorIndex <= 0) {
      continue;
    }
    const key = line.slice(0, separatorIndex).trim();
    const value = line.slice(separatorIndex + 1).trim();
    if (!key || process.env[key] !== undefined) {
      continue;
    }
    process.env[key] = value;
  }
}

function envOrDefaultBoolean(name: string, fallback: boolean): boolean {
  const value = process.env[name]?.trim().toLowerCase();
  if (!value) {
    return fallback;
  }
  if (["1", "true", "yes", "on"].includes(value)) {
    return true;
  }
  if (["0", "false", "no", "off"].includes(value)) {
    return false;
  }
  return fallback;
}

function envOrDefaultPort(name: string, fallback: number): number {
  const value = process.env[name]?.trim();
  if (!value) {
    return fallback;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function splitCsv(raw: string): string[] {
  return raw
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function loadConfig(): Config {
  loadDotEnv(".env");
  const config: Config = {
    port: envOrDefaultPort("PORT", 3000),
    authSecret: envOrDefault("AUTH_SECRET", defaultAuthSecret),
    adminEmail: envOrDefault("ADMIN_EMAIL", "operator@rendycrm.local"),
    adminPassword: envOrDefault("ADMIN_PASSWORD", defaultAdminPassword),
    corsAllowedOrigins: splitCsv(envOrDefault("CORS_ALLOWED_ORIGINS", "*")),
    corsAllowCredentials: envOrDefaultBoolean("CORS_ALLOW_CREDENTIALS", false),
    telegramApiBaseUrl: envOrDefault("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
    clientWebhookSecret: process.env.TELEGRAM_CLIENT_WEBHOOK_SECRET?.trim() ?? "",
    operatorWebhookSecret: process.env.TELEGRAM_OPERATOR_WEBHOOK_SECRET?.trim() ?? "",
    clientBotToken: process.env.TELEGRAM_CLIENT_BOT_TOKEN?.trim() ?? "",
    operatorBotToken: process.env.TELEGRAM_OPERATOR_BOT_TOKEN?.trim() ?? "",
    clientWelcomeMessage: envOrDefault(
      "CLIENT_WELCOME_MESSAGE",
      "Здравствуйте! Помогу записаться или быстро отвечу на вопросы.",
    ),
    clientSlotsMessage: envOrDefault(
      "CLIENT_SLOTS_MESSAGE",
      "Свободные окна уточняются. Напишите желаемую дату или время, и я помогу с записью.",
    ),
    clientPriceMessage: envOrDefault(
      "CLIENT_PRICE_MESSAGE",
      "Стоимость уточняется у мастера. Напишите, какая услуга вас интересует, и я передам запрос.",
    ),
    clientAddressMessage: envOrDefault(
      "CLIENT_ADDRESS_MESSAGE",
      "Адрес: уточните адрес салона здесь или замените это сообщение в .env на актуальное.",
    ),
    clientHandoffMessage: envOrDefault(
      "CLIENT_HANDOFF_MESSAGE",
      "Передаю ваш запрос человеку. Он ответит в этом чате.",
    ),
    operatorWelcomeMessage: envOrDefault(
      "OPERATOR_WELCOME_MESSAGE",
      "Operator bot на связи. Доступны команды: /dashboard, /dialogs, /slots, /settings.",
    ),
  };
  validateConfig(config);
  return config;
}

function validateConfig(config: Config): void {
  const issues: string[] = [];

  if (process.env.NODE_ENV === "production") {
    if (config.authSecret === defaultAuthSecret) {
      issues.push("AUTH_SECRET must not use the default value in production");
    }
    if (config.adminPassword === defaultAdminPassword) {
      issues.push("ADMIN_PASSWORD must not use the default value in production");
    }
    if (config.corsAllowCredentials && config.corsAllowedOrigins.includes("*")) {
      issues.push("CORS_ALLOWED_ORIGINS cannot contain '*' when CORS_ALLOW_CREDENTIALS=true in production");
    }
  }

  if (config.clientBotToken !== "" && config.clientWebhookSecret === "") {
    issues.push("TELEGRAM_CLIENT_WEBHOOK_SECRET is required when TELEGRAM_CLIENT_BOT_TOKEN is configured");
  }
  if (config.operatorBotToken !== "" && config.operatorWebhookSecret === "") {
    issues.push("TELEGRAM_OPERATOR_WEBHOOK_SECRET is required when TELEGRAM_OPERATOR_BOT_TOKEN is configured");
  }

  if (issues.length > 0) {
    throw new Error(`invalid runtime configuration: ${issues.join("; ")}`);
  }
}
