package controller

import "testing"

func TestYellowMartStockPosition(t *testing.T) {
	stock := []uint8{0x04, 0x05, 0x0b, 0x0f}
	if got := yellowMartStockPosition(stock, 0x0b); got != 2 {
		t.Fatalf("yellowMartStockPosition = %d, want 2", got)
	}
	if got := yellowMartStockPosition(stock, 0x01); got != -1 {
		t.Fatalf("yellowMartStockPosition missing item = %d, want -1", got)
	}
}
