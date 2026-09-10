package sym

// TwoOptionMenuID is wTwoOptionMenuID. Its low 7 bits select the semantic
// two-option menu layout (YES/NO, NO/YES, HEAL/CANCEL, directional choices,
// etc.); bit 7 only requests that the second option starts selected.
// Address from pokered.sym / ram/wram.asm.
const TwoOptionMenuID uint16 = 0xD12C
