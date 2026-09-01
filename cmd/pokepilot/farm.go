// Farm mode for pokepilot: a loop around the existing run, driven by specs
// leased from the wall (docs/archive/2026-08-26-farm-design.md §4). The game
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
	// heartbeatTrailMax is roughly one minute of positions at the normal
	// heartbeat cadence. The trail is reset whenever the map changes.
	heartbeatTrailMax = 64
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
	hb.Raw = s.hb.Raw
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

// storeRaw publishes the verbatim model exchange. A prompt starts a fresh
// entry (start true) and the reply appends to it, so the panel shows the
// prompt while the POST is still blocked and grows the reply when it lands.
//
// ponytail: clipped, not paged. Raw rides every heartbeat, and an
// observation JSON plus a 16-line menu is a few KB; a run whose prompt
// outgrows the clip needs a per-round artifact, not a bigger heartbeat.
func (s *heartbeatSnap) storeRaw(text string, start bool) {
	s.mu.Lock()
	if start {
		s.hb.Raw = clip(text, maxRawPrompt)
	} else {
		s.hb.Raw += "\n" + clip(text, maxRawReply)
	}
	s.mu.Unlock()
}

const (
	maxRawPrompt = 6000
	maxRawReply  = 2000
)

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n… clipped"
}

// rawWriter adapts storeRaw to the io.Writer the planner's PromptLog and
// ReplyLog want. Each Fprintf there is one Write, so no buffering.
type rawWriter struct {
	snap  *heartbeatSnap
	start bool
}

func (w rawWriter) Write(p []byte) (int, error) {
	w.snap.storeRaw(string(p), w.start)
	return len(p), nil
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

// heartbeatTrail owns the recent map-local position samples. It lives on the
// stepping goroutine, so no locking is needed; heartbeatSnap receives copies.
type heartbeatTrail struct {
	mapID uint8
	set   bool
	pts   [][2]uint8
}

func (t *heartbeatTrail) add(mapID, x, y uint8) [][2]uint8 {
	if !t.set || t.mapID != mapID {
		t.mapID = mapID
		t.set = true
		t.pts = t.pts[:0]
	}
	p := [2]uint8{x, y}
	if len(t.pts) == 0 || t.pts[len(t.pts)-1] != p {
		if len(t.pts) == heartbeatTrailMax {
			copy(t.pts, t.pts[1:])
			t.pts[len(t.pts)-1] = p
		} else {
			t.pts = append(t.pts, p)
		}
	}
	return append([][2]uint8(nil), t.pts...)
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
func runFarm(m *emu.Emu, client *farm.Client, bootState []byte, watchPort int, checkpointDir string) {
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
			finishRun(m, client, *spec, "error", err.Error(), 0, "", nil, nil)
			time.Sleep(farmErrorSleep)
			continue
		}

		runOne(m, client, *spec, planner, starter, dest, spec.Goal, fps, maxRounds, maxFrames, bootState, tracer, snap, &mem, addrs, checkpointDir)
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
func runOne(m *emu.Emu, client *farm.Client, spec farm.Spec, planner, starter, dest, goal string, fps, maxRounds, maxFrames int, bootState []byte, tracer *dialogueTracer, snap *heartbeatSnap, mem *state.Mem, addrs []string, checkpointDir string) {
	// A new lease must not inherit the previous run's plan: the snap is
	// reused for the worker's lifetime.
	snap.store(farm.Heartbeat{RunID: spec.RunID})
	if err := m.LoadState(bootState); err != nil {
		log.Printf("farm: %s: load state: %v", spec.RunID, err)
		finishRun(m, client, spec, "error", fmt.Sprintf("load state: %v", err), 0, "", nil, nil)
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

	// A caller-supplied -checkpoint-dir wins; only an "llm" planner with none
	// given gets an ephemeral one, so the flag threaded from main.go is never
	// silently discarded in favor of a temp dir nobody asked for.
	if checkpointDir == "" && planner == "llm" {
		dir, err := os.MkdirTemp("", "pokefarm-checkpoints-")
		if err != nil {
			log.Printf("farm: %s: checkpoint dir: %v", spec.RunID, err)
			finishRun(m, client, spec, "error", fmt.Sprintf("checkpoint dir: %v", err), burn, "", nil, nil)
			return
		}
		checkpointDir = dir
	}

	// Compose the sample callback on this (stepping) goroutine: the dialogue
	// tracer plus the heartbeat snapshot, sharing one hoisted Mem buffer.
	trail := &heartbeatTrail{}
	m.OnSample(func(m *emu.Emu) {
		tracer.sample(m)
		sampleHeartbeat(m, spec.RunID, snap, mem, addrs, trail)
	})
	sampleHeartbeat(m, spec.RunID, snap, mem, addrs, trail) // synchronous initial sample

	var samples chan periodicSample
	var stopUploader chan struct{}
	var uploaderDone <-chan struct{}
	if checkpointDir != "" {
		samples = make(chan periodicSample, 1)
		stopUploader = make(chan struct{})
		uploaderDone = runCheckpointUploader(client, spec.RunID, spec.Attempt, checkpointDir, samples, stopUploader)
		// agent.Run replaces OnSample with its dialogue tape, so the
		// flight recorder lives on OnFrame, which the stepper always
		// runs and the uploader never sees.
		m.OnFrame(func(em *emu.Emu) {
			maybeCapturePeriodic(em, snap, samples)
		})
	}

	cancel := make(chan struct{})
	stop := make(chan struct{})
	hbDone := heartbeatLoop(client, spec.RunID, snap.load, cancel, stop, heartbeatInterval)

	var reason, detail string
	var progEarly, progFinal *farm.Progress
	switch planner {
	case "scripted":
		reason, detail, progEarly, progFinal = runFarmScripted(m, starter, dest)
	case "llm":
		reason, detail, progEarly, progFinal = runFarmLLM(m, starter, goal, maxRounds, maxFrames, cancel, snap, checkpointDir)
	}

	// Stop and join the heartbeat before TraceTail/SaveState/Finish.
	close(stop)
	<-hbDone
	m.OnFrame(nil)
	if stopUploader != nil {
		close(stopUploader)
		<-uploaderDone
	}

	finishRun(m, client, spec, reason, detail, burn, checkpointDir, progEarly, progFinal)
}

// sampleHeartbeat captures the plain snapshot the heartbeat goroutine will
// send. It runs on the stepping goroutine (via the composed OnSample
// callback or the initial call), so it may read emulator memory; mem is
// hoisted and reused rather than allocated per sample.
func sampleHeartbeat(m *emu.Emu, runID string, snap *heartbeatSnap, mem *state.Mem, addrs []string, trail *heartbeatTrail) {
	g := state.Read(m, mem)
	hb := farm.Heartbeat{
		RunID:       runID,
		Frame:       m.FrameCount(),
		Map:         g.Player.MapID,
		X:           g.Player.X,
		Y:           g.Player.Y,
		WorkerAddrs: addrs,
		Trail:       trail.add(g.Player.MapID, g.Player.X, g.Player.Y),
	}
	for _, sp := range state.DecodeSprites(mem) {
		// Coordinates outside uint8 are invalid map positions; hidden sprites
		// have already been filtered by DecodeSprites.
		if sp.X < 0 || sp.Y < 0 || sp.X > 255 || sp.Y > 255 {
			continue
		}
		hb.Sprites = append(hb.Sprites, farm.MapSprite{
			X: uint8(sp.X), Y: uint8(sp.Y), PictureID: sp.PictureID, Slot: uint8(sp.Slot),
		})
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
			host, _, err := net.SplitHostPort(a.String())
			if err != nil {
				host = strings.TrimSuffix(strings.TrimPrefix(a.String(), "["), "]")
				if i := strings.LastIndex(host, "%"); i >= 0 {
					host = host[:i]
				}
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			out = append(out, net.JoinHostPort(host, strconv.Itoa(port)))
		}
	}
	return out
}
