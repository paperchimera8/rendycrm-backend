export const CLIENT_FUNCTIONAL_SPEC = {
  modes: ["global", "workspace"],
  entry: [
    "/start without payload shows greeting and phone prompt in global mode.",
    "/start <master_phone> tries to resolve the master immediately in global mode.",
    "/start in workspace mode opens the fixed workspace without asking for a phone.",
  ],
  selection: [
    "Prompt-like texts reopen phone entry instead of being parsed as a phone number.",
    "Unknown or disabled masters keep the route in awaiting_master_phone.",
    "Workspace mode fails fast if fixed workspace configuration is incomplete.",
  ],
  actions: [
    "Ready client menu contains booking, slots, prices, address, human handoff, and master change in global mode.",
    "Text buttons and callback buttons are treated the same way.",
    "Free-form text in ready state is forwarded into CRM.",
  ],
  booking: [
    "Slot selection creates a pending booking state.",
    "Confirmation clears pending state and emits a booking confirmation effect.",
    "Cancellation clears pending state and emits a cancellation effect.",
    "Missing booking state returns a recovery message instead of crashing the flow.",
  ],
  delivery: [
    "Duplicate event handling prefers an external dedup store.",
    "A small in-session recent-event buffer is kept only as a fallback.",
    "Repeated phone prompts are throttled to avoid duplicate Telegram messages.",
  ],
} as const;

export const OPERATOR_FUNCTIONAL_SPEC = {
  binding: [
    "Operator bot stays unavailable until a real link code resolves into an explicit binding.",
    "Binding requires a matching workspace; fake defaults are not allowed.",
  ],
  navigation: [
    "Main sections are dashboard, dialogs, slots, settings, and FAQ.",
    "Reply-keyboard labels and slash commands are normalized to the same command handlers.",
  ],
  dialogs: [
    "Dialog details expose reply, slot suggestion, take-dialog, return-to-auto, and close actions.",
    "Reply flow waits for the next operator message and then resets to idle.",
  ],
  booking: [
    "Slot offer flow asks for a price after slot selection.",
    "Invalid prices keep the flow active and request numeric input.",
    "Valid prices emit a booking confirmation effect with amount and slot metadata.",
  ],
  settings: [
    "Settings are readable in-bot.",
    "Quick auto-reply toggles update session-visible state and emit settings effects.",
  ],
  delivery: [
    "Duplicate event handling prefers an external dedup store.",
    "The bot resets multi-step state on /start or Отмена.",
  ],
} as const;
