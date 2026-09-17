package state

// DecodeCoins returns the Coin Case balance from Red's two-byte packed BCD
// counter. AddBCD saturates overflow to 9999, which the Game Corner clerk uses
// when a final 50-coin purchase would cross the four-digit limit.
func (a Addresses) DecodeCoins(mem *Mem) int {
	if mem == nil {
		return 0
	}
	return bcdByte(mem.U8(a.PlayerCoins))*100 + bcdByte(mem.U8(a.PlayerCoins+1))
}

func bcdByte(v uint8) int {
	return int(v>>4)*10 + int(v&0x0f)
}
