package state

const (
	// wStatusFlags1 bit 6 is BIT_GAVE_SAFFRON_GUARDS_DRINK.
	saffronGuardsDrinkMask uint8 = 1 << 6

	// wElite4Flags lives at 0xd734. HallOfFameResetEventsAndSaveScript sets
	// BIT_UNUSED_BEAT_ELITE_4 (bit 0) immediately before resetting the Indigo
	// event range and saving. Despite the historical name, this is the only
	// durable RAM fact that survives the ending's event reset.
	elite4FlagsAddr     uint16 = 0xd734
	elite4CompletedMask uint8  = 1 << 0

	// Item ids from pokered/constants/item_constants.asm.
	secretKeyItemID  uint8 = 0x2b
	cardKeyItemID    uint8 = 0x30
	ssTicketItemID   uint8 = 0x3f
	silphScopeItemID uint8 = 0x48
	pokeFluteItemID  uint8 = 0x49
	hm01ItemID       uint8 = 0xc4
	hm03ItemID       uint8 = 0xc6
	hm04ItemID       uint8 = 0xc7
)

// These event ids stay private to the Red story decoder. Agent/planner code
// consumes StoryFacts instead of learning raw event-bit numbers. The values
// are checked against the vendored event_constants.asm in story_test.go and
// silph_story_test.go.
const (
	eventViridianGymOpen            Event = 0x028
	eventMansionSwitchOn            Event = 0x278
	eventBeatRoute22Rival2ndBattle  Event = 0x526
	eventPassedCascadeBadgeCheck    Event = 0x530
	eventPassedThunderBadgeCheck    Event = 0x531
	eventPassedRainbowBadgeCheck    Event = 0x532
	eventPassedSoulBadgeCheck       Event = 0x533
	eventPassedMarshBadgeCheck      Event = 0x534
	eventPassedVolcanoBadgeCheck    Event = 0x535
	eventPassedEarthBadgeCheck      Event = 0x536
	eventBeatSilphCoRival           Event = 0x740
	eventGotMasterBall              Event = 0x78d
	eventBeatSilphCoGiovanni        Event = 0x78f
	eventBeatLorelei                Event = 0x8e1
	eventAutowalkedIntoLoreleisRoom Event = 0x8e6
	eventBeatBruno                  Event = 0x8e9
	eventBeatAgatha                 Event = 0x8f1
	eventBeatLanceTrainer           Event = 0x8f9
	eventBeatLance                  Event = 0x8fe
)

// StoryFacts is Red's semantic progression projection. It deliberately names
// game concepts, not event ids, WRAM bits, or item bytes. Every field is
// deterministically re-derived from the current RAM/bag state.
type StoryFacts struct {
	MtMoonFossilAcquired       bool
	PokedexAcquired            bool
	SSTicketAcquired           bool
	HM01Acquired               bool
	SilphScopeAcquired         bool
	PokeFluteAcquired          bool
	FuchsiaProgressionComplete bool

	SaffronGateOpen            bool
	CardKeyOwned               bool
	SilphCoRivalDefeated       bool
	SilphCoCleared             bool
	MasterBallAwarded          bool
	SilphRescueComplete        bool
	MansionSwitchOn            bool
	SecretKeyOwned             bool
	ViridianGymOpen            bool
	Route22RivalResolved       bool
	Route23BadgeChecksPassed   int
	Route23BadgeChecksComplete bool
	LeagueChallengeStarted     bool
	LeagueLoreleiDefeated      bool
	LeagueBrunoDefeated        bool
	LeagueAgathaDefeated       bool
	LeagueLanceDefeated        bool
	LeagueChampionDefeated     bool
	MainStoryComplete          bool
}

var route23BadgeCheckEvents = [...]Event{
	eventPassedCascadeBadgeCheck,
	eventPassedThunderBadgeCheck,
	eventPassedRainbowBadgeCheck,
	eventPassedSoulBadgeCheck,
	eventPassedMarshBadgeCheck,
	eventPassedVolcanoBadgeCheck,
	eventPassedEarthBadgeCheck,
}

// DecodeStoryFacts derives planner-facing progression semantics from Red's
// authoritative RAM and decoded inventory. No run memory is involved, so the
// same checkpoint always reconstructs the same facts after resume.
func (a Addresses) DecodeStoryFacts(m *Mem, inv InventoryState) StoryFacts {
	progress := a.DecodeProgress(m)
	mainStoryComplete := m.U8(elite4FlagsAddr)&elite4CompletedMask != 0
	facts := StoryFacts{
		MtMoonFossilAcquired:       a.HasEvent(m, EventBeatMtMoonSuperNerd) && (a.HasEvent(m, EventGotDomeFossil) || a.HasEvent(m, EventGotHelixFossil)) && m.U8(a.MtMoonB2FCurScript) == 0,
		PokedexAcquired:            a.HasEvent(m, EventGotPokedex),
		SSTicketAcquired:           inventoryHasItem(inv, ssTicketItemID),
		HM01Acquired:               inventoryHasItem(inv, hm01ItemID),
		SilphScopeAcquired:         inventoryHasItem(inv, silphScopeItemID),
		PokeFluteAcquired:          inventoryHasItem(inv, pokeFluteItemID),
		FuchsiaProgressionComplete: progress.Has(BadgeSoul) && inventoryHasItem(inv, hm03ItemID) && inventoryHasItem(inv, hm04ItemID),
		SaffronGateOpen:            m.U8(a.StatusFlags1)&saffronGuardsDrinkMask != 0,
		CardKeyOwned:               inventoryHasItem(inv, cardKeyItemID),
		SilphCoRivalDefeated:       a.HasEvent(m, eventBeatSilphCoRival),
		SilphCoCleared:             a.HasEvent(m, eventBeatSilphCoGiovanni),
		MasterBallAwarded:          a.HasEvent(m, eventGotMasterBall),
		MansionSwitchOn:            a.HasEvent(m, eventMansionSwitchOn),
		SecretKeyOwned:             inventoryHasItem(inv, secretKeyItemID),
		ViridianGymOpen:            a.HasEvent(m, eventViridianGymOpen),
		Route22RivalResolved:       a.HasEvent(m, eventBeatRoute22Rival2ndBattle),
		LeagueChallengeStarted:     a.HasEvent(m, eventAutowalkedIntoLoreleisRoom),
		LeagueLoreleiDefeated:      a.HasEvent(m, eventBeatLorelei) || mainStoryComplete,
		LeagueBrunoDefeated:        a.HasEvent(m, eventBeatBruno) || mainStoryComplete,
		LeagueAgathaDefeated:       a.HasEvent(m, eventBeatAgatha) || mainStoryComplete,
		LeagueLanceDefeated:        a.HasEvent(m, eventBeatLance) || mainStoryComplete,
		LeagueChampionDefeated:     a.HasEvent(m, EventBeatChampionRival) || mainStoryComplete,
		MainStoryComplete:          mainStoryComplete,
	}
	facts.SilphRescueComplete = facts.SilphCoCleared && facts.MasterBallAwarded
	for _, event := range route23BadgeCheckEvents {
		if a.HasEvent(m, event) {
			facts.Route23BadgeChecksPassed++
		}
	}
	facts.Route23BadgeChecksComplete = facts.Route23BadgeChecksPassed == len(route23BadgeCheckEvents)
	if facts.LeagueLoreleiDefeated || facts.LeagueBrunoDefeated || facts.LeagueAgathaDefeated || facts.LeagueLanceDefeated || facts.LeagueChampionDefeated || facts.MainStoryComplete {
		facts.LeagueChallengeStarted = true
	}
	return facts
}

func inventoryHasItem(inv InventoryState, id uint8) bool {
	for _, item := range inv.Items {
		if item.ID == id && item.Quantity > 0 {
			return true
		}
	}
	return false
}
