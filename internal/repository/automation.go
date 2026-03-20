package repository

import (
	"fmt"
	"slices"
	"strings"
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

type automationDecision struct {
	Intent      AutomationIntent
	Response    string
	Status      ConversationStatus
	ShouldReply bool
	Buttons     []string
}

var (
	intentHumanKeywords        = []string{"человек", "оператор", "мастер", "позовите", "свяжите", "свяжись", "менеджер"}
	intentBookingKeywords      = []string{"запис", "бронь", "заброни", "подтверд", "окно", "время", "когда можно", "свободно"}
	intentAvailabilityKeywords = []string{"свобод", "окно", "время", "доступно", "после", "до", "когда"}
	intentPriceKeywords        = []string{"цена", "стоим", "сколько", "прайс", "руб"}
	intentRescheduleKeywords   = []string{"перен", "другое время", "смест", "позже", "раньше"}
	intentCancelKeywords       = []string{"отмен", "не смогу", "уберите запись", "удалите запись"}
)

func detectIntent(text string) AutomationIntent {
	normalized := strings.ToLower(strings.TrimSpace(text))
	switch {
	case containsAny(normalized, intentHumanKeywords):
		return IntentHumanRequest
	case containsAny(normalized, intentCancelKeywords):
		return IntentCancel
	case containsAny(normalized, intentRescheduleKeywords):
		return IntentReschedule
	case containsAny(normalized, intentPriceKeywords):
		return IntentPriceQuestion
	case containsAny(normalized, intentBookingKeywords):
		return IntentBookingRequest
	case containsAny(normalized, intentAvailabilityKeywords):
		return IntentAvailabilityQuestion
	default:
		return IntentOther
	}
}

func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func matchFAQ(text string, faqItems []FAQItem) *FAQItem {
	normalized := normalizeForMatch(text)
	bestScore := 0
	var best *FAQItem
	for i := range faqItems {
		score := overlapScore(normalized, normalizeForMatch(faqItems[i].Question))
		if score > bestScore {
			bestScore = score
			best = &faqItems[i]
		}
	}
	if bestScore < 2 {
		return nil
	}
	return best
}

func normalizeForMatch(text string) []string {
	cleaned := strings.ToLower(text)
	replacer := strings.NewReplacer(
		".", " ",
		",", " ",
		"?", " ",
		"!", " ",
		":", " ",
		";", " ",
		"-", " ",
		"\n", " ",
		"\t", " ",
	)
	cleaned = replacer.Replace(cleaned)
	parts := strings.Fields(cleaned)
	seen := make(map[string]struct{}, len(parts))
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 3 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		tokens = append(tokens, part)
	}
	slices.Sort(tokens)
	return tokens
}

func overlapScore(left, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	score := 0
	for _, l := range left {
		if slices.Contains(right, l) {
			score++
		}
	}
	return score
}

func formatSlotsForBot(slots []DailySlot) (string, []string) {
	if len(slots) == 0 {
		return "Пока не вижу свободных окон в ближайшие дни. Передаю диалог оператору.", nil
	}
	lines := []string{"Подходящие слоты:"}
	buttons := make([]string, 0, min(3, len(slots)))
	for i, slot := range slots {
		if i >= 3 {
			break
		}
		label := formatBotSlotLabel(slot)
		lines = append(lines, label)
		buttons = append(buttons, label)
	}
	return strings.Join(lines, "\n"), buttons
}

func formatBotSlotLabel(slot DailySlot) string {
	start := slot.StartsAt.In(time.Local)
	end := slot.EndsAt.In(time.Local)
	if start.Format("02.01") == end.Format("02.01") {
		return fmt.Sprintf("%s-%s", start.Format("02.01 15:04"), end.Format("15:04"))
	}
	return fmt.Sprintf("%s-%s", start.Format("02.01 15:04"), end.Format("02.01 15:04"))
}

func countRecentBotMessages(messages []Message, window time.Duration) int {
	cutoff := time.Now().UTC().Add(-window)
	count := 0
	for _, message := range messages {
		if message.SenderType == MessageSenderBot && message.CreatedAt.After(cutoff) {
			count++
		}
	}
	return count
}
