package data

import "sort"

// Maps returns the generated Gen-II map catalog in native-id order.
func Maps() []MapInfo {
	out := make([]MapInfo, 0, len(mapsByNative))
	for _, info := range mapsByNative {
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		return NativeMapID(out[i].Group, out[i].Number) < NativeMapID(out[j].Group, out[j].Number)
	})
	return out
}

// MapByName resolves a generated decomp map constant name.
func MapByName(name string) (MapInfo, bool) {
	for _, info := range mapsByNative {
		if info.Name == name {
			return info, true
		}
	}
	return MapInfo{}, false
}

// MapByNative resolves a cartridge-native (group, number) map identity to its
// catalog entry. The wide uint16 key is the same identity the native topology
// uses, so a run can translate a live map id to a semantic location without
// narrowing the Gen-II namespace.
func MapByNative(native uint16) (MapInfo, bool) {
	info, ok := mapsByNative[native]
	return info, ok
}
