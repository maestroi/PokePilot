// Farm mode for pokepilot: a loop around the existing run, driven by specs
// leased from the wall (docs/plans/2026-08-26-farm-design.md §4). The game
// stays a plain process; this file is the only PokePilot change.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maestroi/pokepilot/agent"
	"github.com/maestroi/pokepilot/emu"
	"github.com/maestroi/pokepilot/farm"
	"github.com/maestroi/pokepilot/red/state"
	"github.com/maestroi/pokepilot/skill"
)

const (
	// heartbeatInterval is the cadence of an in-flight run's heartbeats:
	// on the order of a second, not every frame.
	heartbeatInterval = time.Second
	// heartbeatDeadline bounds each wall call from the heartbeat goroutine.
	// It must stay short because runOne joins that goroutine before dumping.
	heartbeatDeadline = 2 * time.Second
	// farmHTTPTimeout bounds the Lease and Finish calls.
	farmHTTPTimeout = 2 * time.Second
	// farmIdleSleep is how long a worker with no spec ready waits before
	// leasing again; idle workers keep leasing.
	farmIdleSleep = time.Second
	// farmErrorSleep backs off after a failed lease, finish, or bad spec.
	farmErrorSleep = 2 * time.Second
)

// applySpec maps a leased spec onto the same values the CLI flags set. The
// spec is authoritative, not a sparse merge over flags: Planner, Starter and
// Dest pass through exactly, and zero Seed / zero FPS mean exactly that
// (replay bit-identically / run flat out), never "use the default". Only
// MaxRounds and MaxFrames treat zero as unset and fall back to the llm
// guardrails.
func applySpec(s farm.Spec) (planner, starter, dest string, fps, maxRounds, maxFrames int) {
	planner, starter, dest = s.Planner, s.Starter, s.Dest
	fps = s.FPS
	maxRounds, maxFrames = s.MaxRounds, s.MaxFrames
	if maxRounds == 0 {
		maxRounds = llmMaxRounds
	}
	if maxFrames == 0 {
		maxFrames = llmMaxFrames
	}
	return
}

// seedBurn is the idle frames a nonzero seed burns after boot; 0 replays
// bit-identically and burns nothing. The wall picks the seed; the runner
// never invents one when leased.
func seedBurn(seed int64) int {
	if seed == 0 {
		return 0
	}
	return rand.New(rand.NewPCG(uint64(seed), 0)).IntN(600) // up to ten seconds of game time
}

// starterFromName is the same mapping runScripted uses, as a lookup.
func starterFromName(name string) (skill.Starter, bool) {
	switch name {
	case "charmander":
		return skill.StarterCharmander, true
	case "squirtle":
		return skill.StarterSquirtle, true
	case "bulbasaur":
		return skill.StarterBulbasaur, true
	}
	return 0, false
}

// heartbeatSnap is the plain, mutex-protected status the heartbeat goroutine
// reads while the stepping goroutine writes it. It holds no emulator or state
// reference: heartbeatLoop never touches the machine.
type heartbeatSnap struct {
	mu sync.Mutex
	hb farm.Heartbeat
}

func (s *heartbeatSnap) store(hb farm.Heartbeat) {
	s.mu.Lock()
	s.hb = hb
	s.mu.Unlock()
}

// storeStatus writes the live position/trace without touching the last
// plan or the latest tally. sampleHeartbeat uses this so a tick cannot
// blank the question the planner published while the model was still
// answering, or the stats it pushed between samples.
func (s *heartbeatSnap) storeStatus(hb farm.Heartbeat) {
	s.mu.Lock()
	hb.Question = s.hb.Question
	hb.Decision = s.hb.Decision
	hb.Stats = s.hb.Stats
	s.hb = hb
	s.mu.Unlock()
}

// storePlan writes the latest offered menu and chosen objective. The
// planner calls this from the stepping goroutine — including just
// before a blocking model POST, so heartbeats keep showing the
// question while the stepper is stuck in HTTP.
func (s *heartbeatSnap) storePlan(question, decision string) {
	s.mu.Lock()
	s.hb.Question = question
	s.hb.Decision = decision
	s.mu.Unlock()
}

// storeStats writes the latest planner tally. The copy is taken here —
// value plus a fresh Choices slice — so the snap never aliases the live
// tally that record() keeps mutating on the stepping goroutine.
func (s *heartbeatSnap) storeStats(st farm.LLMStats) {
	st.Choices = append([]farm.ChoiceCount(nil), st.Choices...)
	s.mu.Lock()
	s.hb.Stats = &st
	s.mu.Unlock()
}

func (s *heartbeatSnap) load() farm.Heartbeat {
	s.mu.Lock()
	h := s.hb
	s.mu.Unlock()
	return h
}

// heartbeatLoop pushes one Heartbeat per tick until stop is closed, and
// joins promptly after: every wall call carries a finite deadline
// (heartbeatDeadline), so a wedged handler cannot hold the loop hostage. It
// performs no emulator or red/state work — snap supplies the plain snapshot
// captured on the stepping goroutine. A cancel reply closes cancel at most
// once, even if several replies ask for it.
func heartbeatLoop(client *farm.Client, runID string, snap func() farm.Heartbeat, cancel chan struct{}, stop <-chan struct{}, tick time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var once sync.Once
		for {
			select {
			case <-stop:
				return
			default:
			}
			ctx, cancelCtx := context.WithTimeout(context.Background(), heartbeatDeadline)
			reply, err := client.Heartbeat(ctx, snap())
			cancelCtx()
			if err == nil && reply.Cancel {
				once.Do(func() { close(cancel) })
			}
			select {
			case <-stop:
				return
			case <-time.After(tick):
			}
		}
	}()
	return done
}

// runFarm is the farm loop: lease a spec, validate it before gameplay, run
// it exactly as main.go runs from flags, report why it stopped, and lease
// again. bootState is the SaveState taken right after BootToOverworld; every
// leased run restores it before applying its seed, so each run starts from
// the same overworld frame.
//
// The emulator is single-goroutine: everything that steps or reads it runs
// on this goroutine. The heartbeat goroutine sees only the plain snapshot.
func runFarm(m *emu.Emu, client *farm.Client, bootState []byte, watchPort int) {
	tracer := newDialogueTracer()
	snap := &heartbeatSnap{}
	var mem state.Mem               // hoisted: every sample reuses this buffer
	addrs := workerAddrs(watchPort) // fixed for the container's lifetime

	for {
		pingWorker(client, addrs)
		spec, err := leaseSpec(client)
		if err != nil {
			log.Printf("farm: lease: %v; retrying in %s", err, farmErrorSleep)
			time.Sleep(farmErrorSleep)
			continue
		}
		if spec == nil {
			// 204: no spec ready yet. Idle workers keep leasing.
			time.Sleep(farmIdleSleep)
			continue
		}
		if spec.RunID == "" {
			log.Printf("farm: lease returned a spec with no run_id; ignoring")
			time.Sleep(farmErrorSleep)
			continue
		}

		planner, starter, dest, fps, maxRounds, maxFrames := applySpec(*spec)
		if err := validateSpec(planner, starter, dest); err != nil {
			log.Printf("farm: %s: %v", spec.RunID, err)
			finishRun(m, client, *spec, "error", err.Error())
			time.Sleep(farmErrorSleep)
			continue
		}

		runOne(m, client, *spec, planner, starter, dest, spec.Goal, fps, maxRounds, maxFrames, bootState, tracer, snap, &mem, addrs)
	}
}

// leaseSpec asks the wall for the next spec under a bounded deadline. A nil
// spec (204) means none is ready yet.
func leaseSpec(client *farm.Client) (*farm.Spec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), farmHTTPTimeout)
	defer cancel()
	return client.Lease(ctx)
}

// pingWorker advertises this worker's watch addresses to the wall while it
// is between runs, so the grid shows idle capacity. It is a presence beacon,
// not a dependency: failures are silent here because the lease call right
// after reports the same outage loudly.
func pingWorker(client *farm.Client, addrs []string) {
	if len(addrs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), farmHTTPTimeout)
	defer cancel()
	_ = client.Ping(ctx, addrs)
}

// validateSpec rejects a bad spec before it spends a run: the planner must
// be known, and a scripted spec must name a starter and destination we can
// resolve. An llm spec may name a starter (empty is Squirtle); dest is unused.
func validateSpec(planner, starter, dest string) error {
	switch planner {
	case "scripted":
		if _, ok := starterFromName(starter); !ok {
			return fmt.Errorf("unknown starter %q: want charmander, squirtle or bulbasaur", starter)
		}
		if _, ok := skill.Place(dest); !ok {
			return fmt.Errorf("unknown destination %q", dest)
		}
	case "llm":
		if starter != "" {
			if _, ok := starterFromName(starter); !ok {
				return fmt.Errorf("unknown starter %q: want charmander, squirtle or bulbasaur", starter)
			}
		}
	default:
		return fmt.Errorf("unknown planner %q: want scripted or llm", planner)
	}
	return nil
}

// runOne runs one leased spec end-to-end and always finishes it. The
// heartbeat starts before gameplay and is stopped and joined before the
// dump, so no heartbeat arrives after Finish.
func runOne(m *emu.Emu, client *farm.Client, spec farm.Spec, planner, starter, dest, goal string, fps, maxRounds, maxFrames int, bootState []byte, tracer *dialogueTracer, snap *heartbeatSnap, mem *state.Mem, addrs []string) {
	// A new lease must not inherit the previous run's plan: the snap is
	// reused for the worker's lifetime.
	snap.store(farm.Heartbeat{RunID: spec.RunID})
	if err := m.LoadState(bootState); err != nil {
		log.Printf("farm: %s: load state: %v", spec.RunID, err)
		finishRun(m, client, spec, "error", fmt.Sprintf("load state: %v", err))
		return
	}

	// The seed is applied exactly once per lease, after the restore: idle
	// frames shift the cycle count and with it every encounter that follows.
	seed := spec.Seed
	burn := seedBurn(seed)
	if burn > 0 {
		m.StepFrames(burn)
		fmt.Printf("seed %d: burned %d idle frames, so this run's luck differs\n", seed, burn)
	}

	m.Pace(fps)
	m.TraceHeader(runHeader(planner, starter, dest, seed, burn))

	// Compose the sample callback on this (stepping) goroutine: the dialogue
	// tracer plus the heartbeat snapshot, sharing one hoisted Mem buffer.
	m.OnSample(func(m *emu.Emu) {
		tracer.sample(m)
		sampleHeartbeat(m, spec.RunID, snap, mem, addrs)
	})
	sampleHeartbeat(m, spec.RunID, snap, mem, addrs) // synchronous initial sample

	cancel := make(chan struct{})
	stop := make(chan struct{})
	hbDone := heartbeatLoop(client, spec.RunID, snap.load, cancel, stop, heartbeatInterval)

	var reason, detail string
	switch planner {
	case "scripted":
		reason, detail = runFarmScripted(m, starter, dest)
	case "llm":
		reason, detail = runFarmLLM(m, starter, goal, maxRounds, maxFrames, cancel, snap)
	}

	// Stop and join the heartbeat before TraceTail/SaveState/Finish.
	close(stop)
	<-hbDone

	finishRun(m, client, spec, reason, detail)
}

// sampleHeartbeat captures the plain snapshot the heartbeat goroutine will
// send. It runs on the stepping goroutine (via the composed OnSample
// callback or the initial call), so it may read emulator memory; mem is
// hoisted and reused rather than allocated per sample.
func sampleHeartbeat(m *emu.Emu, runID string, snap *heartbeatSnap, mem *state.Mem, addrs []string) {
	g := state.Read(m, mem)
	hb := farm.Heartbeat{
		RunID:       runID,
		Frame:       m.FrameCount(),
		Map:         g.Player.MapID,
		X:           g.Player.X,
		Y:           g.Player.Y,
		WorkerAddrs: addrs,
	}
	if tail := m.TraceTail(1); len(tail) > 0 {
		hb.Trace = tail[len(tail)-1]
	}
	snap.storeStatus(hb)
}

// workerAddrs lists every non-loopback local address as "host:port", so the
// wall can reach this runner's watch server from whichever swarm network
// interface it is on. The container's interfaces are fixed for the process
// lifetime, so the result is computed once and reused by every heartbeat.
func workerAddrs(port int) []string {
	if port <= 0 {
		return nil
	}
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			out = append(out, net.JoinHostPort(ipnet.IP.String(), strconv.Itoa(port)))
		}
	}
	return out
}

// runFarmScripted mirrors runScripted: take the starter, walk to the
// destination. It returns the finish reason instead of keeping the server
// alive, because the wall decides what happens next.
func runFarmScripted(m *emu.Emu, starter, dest string) (string, string) {
	which, _ := starterFromName(starter) // validated before gameplay

	fmt.Printf("getting the %s starter (this includes the rival battle)...\n", starter)
	if err := skill.GetStarter(m, m.ROM(), which, skill.StatAwareMove(m.ROM())); err != nil {
		return "error", fmt.Sprintf("get starter: %v", err)
	}

	target, ok := skill.Place(dest)
	if !ok {
		return "error", fmt.Sprintf("unknown destination %q", dest)
	}
	fmt.Printf("walking to %q (map %02x, %d,%d)...\n", dest, target.Map, target.X, target.Y)
	start := time.Now()
	if err := skill.GoTo(m, m.ROM(), target); err != nil {
		return "error", fmt.Sprintf("GoTo: %v", err)
	}
	fmt.Printf("arrived at %q after %s\n", dest, time.Since(start).Round(time.Millisecond))
	return "done", ""
}

// runFarmLLM mirrors runLLM's diagnostics and objective list; the only
// differences are that the budget comes from the spec and cancel is the
// wall's cooperative stop.
func runFarmLLM(m *emu.Emu, starter, goal string, maxRounds, maxFrames int, cancel <-chan struct{}, snap *heartbeatSnap) (string, string) {
	// The starter is the farm's controlled variable, so the harness TAKES it
	// before handing control to the model — the same reason badgerun does
	// (a model that knows Pokemon always picks Squirtle otherwise). From
	// here on the model decides everything, and the menu is no longer built
	// here at all: agent.Run rebuilds it every round from the current
	// observation (agent.Offer), which is strictly better than a static list
	// of every place in the ROM.
	if err := skill.GetStarter(m, m.ROM(), farmStarterFor(starter), skill.StatAwareMove(m.ROM())); err != nil {
		return "error", fmt.Sprintf("get starter %s: %v", starter, err)
	}
	fmt.Println("planner: llm — the model picks from a menu rebuilt every round")

	logw := &agentTraceLog{w: os.Stdout, note: m.TraceNote}
	planner := agent.NewLLMPlanner()
	planner.Goal = goal
	planner.Log = logw // one line per model call, above its round line
	// The same tally the local watch page shows (runStats): a farm worker's
	// page is this page, so a wandering leased run is visible on it too.
	stats := newStatsPlanner(planner, m.TraceStats, snap)
	res := agent.Run(m, m.ROM(), reportingPlanner{inner: stats, snap: snap}, agent.Budget{
		MaxRounds: maxRounds,
		MaxFrames: maxFrames,
		Log:       logw,
		Cancel:    cancel,
	})

	fmt.Printf("\nrun stopped: %s after %d round(s)\n", stopName(res.Stop), res.Rounds)
	for i, o := range res.Completed {
		fmt.Printf("  completed %d: %s\n", i+1, o)
	}
	detail := ""
	if res.Err != nil {
		fmt.Printf("  error: %v\n", res.Err)
		detail = res.Err.Error()
	}
	return stopName(res.Stop), detail
}

// reportingPlanner publishes the offered menu onto the heartbeat snap
// before the inner planner is asked, and the chosen objective after it
// answers. That is what keeps the watch pane current during a multi-second
// model POST, when the stepper is blocked and sampleHeartbeat is not
// running.
type reportingPlanner struct {
	inner agent.Planner
	snap  *heartbeatSnap
}

func (p reportingPlanner) Next(obs agent.Observation, offered []agent.Objective) (agent.Objective, error) {
	return p.ask(obs, offered, "")
}

func (p reportingPlanner) NextFeedback(obs agent.Observation, offered []agent.Objective, feedback string) (agent.Objective, error) {
	return p.ask(obs, offered, feedback)
}

func (p reportingPlanner) ask(obs agent.Observation, offered []agent.Objective, feedback string) (agent.Objective, error) {
	q := planQuestion(offered)
	if p.snap != nil {
		p.snap.storePlan(q, "")
	}
	var (
		obj agent.Objective
		err error
	)
	if feedback != "" {
		obj, err = p.inner.(agent.FeedbackPlanner).NextFeedback(obs, offered, feedback)
	} else {
		obj, err = p.inner.Next(obs, offered)
	}
	if err == nil && p.snap != nil {
		p.snap.storePlan(q, obj.String())
	}
	return obj, err
}

func planQuestion(offered []agent.Objective) string {
	var b strings.Builder
	for i, o := range offered {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d: %s", i+1, o)
	}
	return b.String()
}

// finishRun sends the Finish dump for one accepted run. Every accepted
// nonempty RunID gets an attempt, including runs that died in validation or
// LoadState before they stepped a frame. It is the last call before the next
// lease, and it happens after the heartbeat has been joined.
func finishRun(m *emu.Emu, client *farm.Client, spec farm.Spec, reason, detail string) {
	report := farm.FinishReport{
		RunID:     spec.RunID,
		Attempt:   spec.Attempt,
		Reason:    reason,
		Detail:    detail,
		TraceTail: m.TraceTail(20),
	}
	if save, err := m.SaveState(); err == nil {
		report.SaveState = save
	} else {
		log.Printf("farm: %s: save state: %v", spec.RunID, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), farmHTTPTimeout)
	err := client.Finish(ctx, report)
	cancel()
	if err != nil {
		log.Printf("farm: %s: finish: %v", spec.RunID, err)
		return
	}
	fmt.Printf("run %s finished: %s\n", spec.RunID, reason)
}

// farmStarterFor maps a spec's starter name onto the typed skill.Starter the
// objective layer now carries. The conversion lives at the CLI/spec boundary
// rather than inside agent: since S6-7 an Objective's Starter is typed, so
// agent no longer parses names. Unknown and empty keep the historic default
// (Squirtle) so an older spec that omits the field behaves as it always did.
func farmStarterFor(name string) skill.Starter {
	switch name {
	case "charmander":
		return skill.StarterCharmander
	case "bulbasaur":
		return skill.StarterBulbasaur
	default:
		return skill.StarterSquirtle
	}
}
