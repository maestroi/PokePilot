package sym

// Current-map warp data. Each wWarpEntries record is Y, X, destination warp
// ID, destination map ID. Red's elevator scripts rewrite the destination pair
// in place after a floor is selected.
const (
	NumberOfWarps uint16 = 0xD3AE
	WarpEntries   uint16 = 0xD3AF
)
