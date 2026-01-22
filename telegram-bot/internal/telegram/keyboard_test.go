package telegram

import "testing"

func TestKeyboardBuilder_SingleRow(t *testing.T) {
	kb := NewKeyboard().
		Button("Btn1", "data1").
		Button("Btn2", "data2").
		Row().
		Build()

	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 2 {
		t.Errorf("expected 2 buttons, got %d", len(kb.InlineKeyboard[0]))
	}
	if kb.InlineKeyboard[0][0].Text != "Btn1" {
		t.Errorf("expected Btn1, got %s", kb.InlineKeyboard[0][0].Text)
	}
	if *kb.InlineKeyboard[0][0].CallbackData != "data1" {
		t.Errorf("expected data1, got %s", *kb.InlineKeyboard[0][0].CallbackData)
	}
}

func TestKeyboardBuilder_MultipleRows(t *testing.T) {
	kb := NewKeyboard().
		Button("A", "a").Row().
		Button("B", "b").Row().
		Build()

	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(kb.InlineKeyboard))
	}
}

func TestKeyboardBuilder_Columns(t *testing.T) {
	kb := NewKeyboard().
		Button("1", "1").
		Button("2", "2").
		Button("3", "3").
		Button("4", "4").
		Button("5", "5").
		Columns(2).
		Build()

	// 5 buttons with 2 columns = 3 rows (2+2+1)
	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 2 {
		t.Errorf("row 0: expected 2 buttons, got %d", len(kb.InlineKeyboard[0]))
	}
	if len(kb.InlineKeyboard[2]) != 1 {
		t.Errorf("row 2: expected 1 button, got %d", len(kb.InlineKeyboard[2]))
	}
}

func TestKeyboardBuilder_Empty(t *testing.T) {
	kb := NewKeyboard().Build()
	if len(kb.InlineKeyboard) != 0 {
		t.Errorf("expected empty keyboard, got %d rows", len(kb.InlineKeyboard))
	}
}
