export interface BotButton {
  readonly text: string;
  readonly action: string;
}

export interface ReplyEffect {
  readonly type: "reply";
  readonly text: string;
  readonly buttons: readonly BotButton[];
}

export type CrmIntent =
  | "client.message_forwarded"
  | "client.slots_requested"
  | "client.prices_requested"
  | "client.human_requested"
  | "operator.reply_sent"
  | "operator.take_dialog"
  | "operator.return_to_auto"
  | "operator.close_dialog";

export interface CrmEffect {
  readonly type: "crm";
  readonly intent: CrmIntent;
  readonly workspaceId?: string;
  readonly conversationId?: string;
  readonly text?: string;
}

export interface ClientRouteSelectedEffect {
  readonly type: "client.route_selected";
  readonly workspaceId: string;
  readonly workspaceName: string;
  readonly masterPhone?: string;
}

export interface BookingPendingEffect {
  readonly type: "booking.pending";
  readonly workspaceId: string;
  readonly slotId: string;
  readonly slotLabel: string;
}

export interface BookingConfirmedEffect {
  readonly type: "booking.confirmed";
  readonly workspaceId: string;
  readonly slotId: string;
  readonly slotLabel: string;
  readonly amount?: number;
  readonly conversationId?: string;
  readonly customerId?: string;
}

export interface BookingCancelledEffect {
  readonly type: "booking.cancelled";
  readonly workspaceId: string;
  readonly slotId: string;
}

export interface SettingsAutoReplyChangedEffect {
  readonly type: "settings.auto_reply_changed";
  readonly workspaceId: string;
  readonly enabled: boolean;
}

export interface OperatorBoundEffect {
  readonly type: "operator.bound";
  readonly workspaceId: string;
  readonly userId: string;
  readonly chatId: string;
}

export interface ConfigurationErrorEffect {
  readonly type: "configuration.error";
  readonly bot: "client" | "operator";
  readonly message: string;
}

export type BotEffect =
  | ReplyEffect
  | CrmEffect
  | ClientRouteSelectedEffect
  | BookingPendingEffect
  | BookingConfirmedEffect
  | BookingCancelledEffect
  | SettingsAutoReplyChangedEffect
  | OperatorBoundEffect
  | ConfigurationErrorEffect;

export interface Transition<State> {
  readonly state: State;
  readonly effects: readonly BotEffect[];
}

export interface SlotOption {
  readonly id: string;
  readonly label: string;
}

export interface RecentEventState {
  readonly recentEventIds: readonly string[];
}

export interface DedupStore {
  claim(key: string, ttlSeconds: number): Promise<boolean>;
}

const DEFAULT_RECENT_EVENT_LIMIT = 50;

export class InMemoryDedupStore implements DedupStore {
  private readonly claims = new Map<string, number>();

  async claim(key: string, ttlSeconds: number): Promise<boolean> {
    const now = Date.now();
    this.evictExpired(now);
    const expiresAt = this.claims.get(key);
    if (expiresAt !== undefined && expiresAt > now) {
      return false;
    }
    this.claims.set(key, now + ttlSeconds * 1000);
    return true;
  }

  private evictExpired(now: number): void {
    for (const [key, expiresAt] of this.claims.entries()) {
      if (expiresAt <= now) {
        this.claims.delete(key);
      }
    }
  }
}

export function reply(
  text: string,
  buttons: readonly BotButton[] = [],
): ReplyEffect {
  return { type: "reply", text, buttons };
}

export function appendRecentEventId(
  eventIds: readonly string[],
  eventId: string,
  limit = DEFAULT_RECENT_EVENT_LIMIT,
): readonly string[] {
  if (!eventId || eventIds.includes(eventId)) {
    return eventIds;
  }
  return [...eventIds, eventId].slice(-limit);
}

export async function claimEvent<State extends RecentEventState>(
  state: State,
  namespace: string,
  eventId: string | undefined,
  dedupStore?: DedupStore,
  ttlSeconds = 300,
): Promise<{ readonly claimed: boolean; readonly state: State }> {
  if (!eventId) {
    return { claimed: true, state };
  }

  if (dedupStore) {
    try {
      const claimed = await dedupStore.claim(`${namespace}:${eventId}`, ttlSeconds);
      if (!claimed) {
        return { claimed: false, state };
      }
    } catch {
      if (state.recentEventIds.includes(eventId)) {
        return { claimed: false, state };
      }
    }
  } else if (state.recentEventIds.includes(eventId)) {
    return { claimed: false, state };
  }

  return {
    claimed: true,
    state: {
      ...state,
      recentEventIds: appendRecentEventId(state.recentEventIds, eventId),
    },
  };
}
