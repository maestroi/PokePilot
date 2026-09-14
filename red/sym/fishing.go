package sym

// RodResponse is wRodResponse, the fishing animation's scratch result:
// 0 = no bite, 1 = bite, 2 = no fish on this map. The address is part of
// WRAM's union scratch area and is only authoritative while rod use resolves.
const RodResponse uint16 = 0xCD3D
