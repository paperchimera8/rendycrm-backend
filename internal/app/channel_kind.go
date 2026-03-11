package app

func defaultChannelKind(provider ChannelProvider) ChannelKind {
	switch provider {
	case ChannelTelegram:
		return ChannelKindTelegramClient
	case ChannelWhatsApp:
		return ChannelKindWhatsAppTwilio
	default:
		return ChannelKind(provider)
	}
}
