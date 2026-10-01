package gpt

import (
	"DarkCS/entity"
	"testing"
)

func TestBasketKeyIgnoresLineOrder(t *testing.T) {
	a := []entity.OrderProduct{{Code: "A", Quantity: 1}, {Code: "B", Quantity: 2}}
	b := []entity.OrderProduct{{Code: "B", Quantity: 2}, {Code: "A", Quantity: 1}}
	if basketKey(a) != basketKey(b) {
		t.Fatal("same products in another order must give the same key")
	}
	c := []entity.OrderProduct{{Code: "A", Quantity: 2}, {Code: "B", Quantity: 2}}
	if basketKey(a) == basketKey(c) {
		t.Fatal("different quantities must give different keys")
	}
}

func TestDuplicateOrderWindow(t *testing.T) {
	o := &Overseer{recentOrders: make(map[string]recentOrder)}
	if o.orderedRecently("u1", "A:1") {
		t.Fatal("nothing ordered yet")
	}
	o.rememberOrder("u1", "A:1")
	if !o.orderedRecently("u1", "A:1") {
		t.Fatal("repeat of the same order must be detected")
	}
	if o.orderedRecently("u1", "B:1") || o.orderedRecently("u2", "A:1") {
		t.Fatal("other baskets and users are not duplicates")
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("Привіт", 3); got != "При" {
		t.Errorf("got %q", got)
	}
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Errorf("got %q", got)
	}
}
