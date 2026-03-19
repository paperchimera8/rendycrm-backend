export type TelegramUser = {
  id: number;
  is_bot?: boolean;
  first_name?: string;
  last_name?: string;
  username?: string;
};

export type TelegramChat = {
  id: number;
  type?: string;
};

export type TelegramMessage = {
  message_id: number;
  date?: number;
  text?: string;
  chat: TelegramChat;
  from?: TelegramUser;
};

export type TelegramCallbackQuery = {
  id: string;
  from: TelegramUser;
  data?: string;
  message?: TelegramMessage;
};

export type TelegramUpdate = {
  update_id: number;
  message?: TelegramMessage;
  callback_query?: TelegramCallbackQuery;
};

export type TelegramButton = {
  text: string;
  callback_data: string;
};

type TelegramApiResponse<T> = {
  ok: boolean;
  result?: T;
  description?: string;
};

function normalizeApiBaseUrl(value: string): string {
  return value.replace(/\/+$/, "");
}

export class TelegramBotApi {
  private readonly apiBaseUrl: string;

  constructor(
    private readonly token: string,
    apiBaseUrl: string,
  ) {
    this.apiBaseUrl = normalizeApiBaseUrl(apiBaseUrl);
  }

  async sendMessage(chatId: number | string, text: string, buttons: TelegramButton[] = []): Promise<void> {
    const payload: Record<string, unknown> = {
      chat_id: chatId,
      text,
    };

    if (buttons.length > 0) {
      payload.reply_markup = {
        inline_keyboard: buttons.map((button) => [
          {
            text: button.text,
            callback_data: button.callback_data,
          },
        ]),
      };
    }

    await this.call("sendMessage", payload);
  }

  async answerCallbackQuery(callbackQueryId: string, text = ""): Promise<void> {
    const payload: Record<string, unknown> = {
      callback_query_id: callbackQueryId,
    };
    if (text.trim() !== "") {
      payload.text = text;
    }
    await this.call("answerCallbackQuery", payload);
  }

  private async call<T>(method: string, payload: Record<string, unknown>): Promise<T> {
    const response = await fetch(`${this.apiBaseUrl}/bot${this.token}/${method}`, {
      method: "POST",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(10_000),
    });

    const body = (await response.json().catch(() => null)) as TelegramApiResponse<T> | null;
    if (!response.ok || !body?.ok) {
      const description = body?.description?.trim() || `Telegram API request failed with HTTP ${response.status}`;
      throw new Error(description);
    }
    return body.result as T;
  }
}
