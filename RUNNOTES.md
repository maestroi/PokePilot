# RUNNOTES — S5b-2: collect Oak's parcel (Viridian Mart)

## What changed
- skill/errand.go (new): `GetParcel(m, romData, policy) error`. Travel to the
  mart; a failed Travel ending ON the mart (0x2A) is the normal entry (the
  greeting box blocks Travel), resumed with `Cutscene(2000, done = parcel
  event set)`. Asserts EVENT_GOT_OAKS_PARCEL set AND parcel in bag. Exports
  `ItemOaksParcel = 0x46`.
- skill/goto.go: places gains "viridian mart" 0x2A (2,5) — open floor in front
  of the counter (clerk (0,5), counter tile (1,5)); also where the entry
  cutscene force-walks the player (warp lands (3,7); walk left 1, up 2).
- red/state/progress.go: `EventGotOaksParcel = 57` (verified by replaying the
  const/const_skip counter over testdata/event_constants.asm, not hand-counted);
  pinned in event_constants_test.go.
- skill/errand_test.go (new): TestGetParcel from the post_starter fixture.

## How the parcel actually works (measured from the decomp)
- It comes from the MAP SCRIPT, not an NPC at (3,3) (that's a COOLTRAINER_M;
  the clerk is at (0,5)). Entry runs ViridianMartDefaultScript (greeting box,
  force-walk to (2,5)), then ViridianMartOaksParcelScript shows the parcel
  box and GiveItem's the frame the box OPENS (DisplayTextID is non-blocking),
  setting event 57. No Talk is issued.
- The greeting cutscene fires on EVERY entry (even after delivery), so
  Travel can never finish a leg into the mart; plan on Travel-then-Cutscene.

## Measured (POKEMON_RED_ROM=/home/maestro/Documents/projects/gomeboy/roms/pokemon_red.gb)
- go test ./skill/ -run TestGetParcel: PASS, 2.5s wall, 1 Route 1 battle, ends
  (2,5) on 0x2A controllable; bag = 5x poke ball + 1x OAK's PARCEL.
- Full gate `go build ./... && go vet ./... && go test ./... -count=1`: all ok, 0 skips.

## For the next task (deliver the parcel to Oak)
- EVENT_OAK_GOT_PARCEL = 56 (same counter replay). The mart swaps to
  ViridianMart_TextPointers2 when 56 is set — clerk text changes after
  delivery, a useful delivery-complete signal. The handover sets
  EVENT_GOT_POKEDEX (37).
- After GetParcel the player is at (2,5); the return leg to Oak's lab (0x0A)
  is mart -> 0x01 -> Route 1 grass -> 0x00. Route 1 encounters are
  battle-counter phase-dependent; do not assume a round trip's count. Pewter
  (0x0D/0x02) stays unreachable until the corridor story event (S5-2).
