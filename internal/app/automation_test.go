package app

import "testing"

func TestDetectIntent(t *testing.T) {
	cases := []struct {
		text string
		want AutomationIntent
	}{
		{text: "Хочу записаться на вечер", want: IntentBookingRequest},
		{text: "Какая цена с покрытием?", want: IntentPriceQuestion},
		{text: "Перенесите запись на завтра", want: IntentReschedule},
		{text: "Отмените, пожалуйста, визит", want: IntentCancel},
		{text: "Позовите человека", want: IntentHumanRequest},
		{text: "Есть ли окно после 20:00?", want: IntentBookingRequest},
	}
	for _, tc := range cases {
		if got := detectIntent(tc.text); got != tc.want {
			t.Fatalf("detectIntent(%q) = %s, want %s", tc.text, got, tc.want)
		}
	}
}

func TestMatchFAQ(t *testing.T) {
	items := []FAQItem{
		{ID: "faq_1", Question: "Сколько стоит маникюр с покрытием?", Answer: "5000 ₽"},
		{ID: "faq_2", Question: "Где вы находитесь?", Answer: "Москва, центр"},
	}
	match := matchFAQ("Подскажите, сколько стоит покрытие?", items)
	if match == nil {
		t.Fatal("expected faq match")
	}
	if match.ID != "faq_1" {
		t.Fatalf("expected faq_1, got %s", match.ID)
	}
}
