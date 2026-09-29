package compositor

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/png"
	"math"
	"strings"
	"sync"

	protocol "github.com/maestroi/pokepilot/renderstate"
	webassets "github.com/maestroi/pokepilot/web"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	SemanticVideoWidth  = 640
	SemanticVideoHeight = 360
)

var ErrUnsupportedSemanticScene = errors.New("semantic scene is unsupported")

type SemanticRenderOptions struct {
	Width  int
	Height int
	AtMS   int64
}

// SemanticRenderer is the deterministic headless counterpart to the browser's
// ModernSceneRenderer. It consumes the same RenderState contract and the exact
// public Tiny Town theme manifest/assets embedded from web/.
type SemanticRenderer struct {
	theme semanticTheme

	assetMu     sync.RWMutex
	assets      map[string]image.Image
	failedAsset map[string]struct{}
}

func NewPublicSemanticRenderer() (*SemanticRenderer, error) {
	theme, err := loadPublicSemanticTheme()
	if err != nil {
		return nil, err
	}
	return &SemanticRenderer{
		theme:       theme,
		assets:      make(map[string]image.Image),
		failedAsset: make(map[string]struct{}),
	}, nil
}

func (r *SemanticRenderer) Version() string {
	if r == nil {
		return fmt.Sprintf("semantic-renderstate-v%d-%s-v%d-headless-v1", protocol.SchemaVersion, SemanticThemeID, semanticThemeVersion)
	}
	return fmt.Sprintf("semantic-renderstate-v%d-%s-v%d-headless-v1", protocol.SchemaVersion, r.theme.ID, r.theme.Version)
}

type SemanticSceneKind string

const (
	SemanticSceneOverworld SemanticSceneKind = "overworld"
	SemanticSceneDialogue  SemanticSceneKind = "dialogue"
	SemanticSceneMenu      SemanticSceneKind = "menu"
	SemanticSceneBattle    SemanticSceneKind = "battle"
)

func SemanticScene(state protocol.RenderState) SemanticSceneKind {
	if canSemanticBattle(state) {
		return SemanticSceneBattle
	}
	if canSemanticDialogue(state) {
		return SemanticSceneDialogue
	}
	if canSemanticMenu(state) {
		return SemanticSceneMenu
	}
	if canSemanticOverworld(state) {
		return SemanticSceneOverworld
	}
	return ""
}

func (r *SemanticRenderer) Supports(state protocol.RenderState) bool {
	return r != nil && SemanticScene(state) != ""
}

func hasSemanticCapability(state protocol.RenderState, capability protocol.Capability) bool {
	return state.HasCapability(capability)
}

func hasSemanticOverworldSurface(state protocol.RenderState) bool {
	if state.SchemaVersion != protocol.SchemaVersion || state.Map == nil || state.Player == nil {
		return false
	}
	if !hasSemanticCapability(state, protocol.CapabilityMap) ||
		!hasSemanticCapability(state, protocol.CapabilityPlayer) ||
		!hasSemanticCapability(state, protocol.CapabilityLayers) {
		return false
	}
	if state.Map.Width <= 0 || state.Map.Height <= 0 {
		return false
	}
	for _, layer := range state.Layers {
		if layer.Kind == protocol.LayerTerrain &&
			layer.Width == state.Map.Width &&
			layer.Height == state.Map.Height &&
			len(layer.Cells) == layer.Width*layer.Height {
			return true
		}
	}
	return false
}

func canSemanticOverworld(state protocol.RenderState) bool {
	return state.Scene == protocol.SceneOverworld && hasSemanticOverworldSurface(state)
}

func canSemanticDialogue(state protocol.RenderState) bool {
	return state.SchemaVersion == protocol.SchemaVersion &&
		state.Scene == protocol.SceneDialogue &&
		hasSemanticCapability(state, protocol.CapabilityDialogue) &&
		state.Dialogue != nil &&
		strings.TrimSpace(state.Dialogue.Text) != ""
}

func canSemanticMenu(state protocol.RenderState) bool {
	if state.SchemaVersion != protocol.SchemaVersion || state.Scene != protocol.SceneMenu ||
		!hasSemanticCapability(state, protocol.CapabilityMenu) || state.Menu == nil {
		return false
	}
	return strings.TrimSpace(state.Menu.Title) != "" || len(state.Menu.Entries) > 0
}

func canSemanticBattle(state protocol.RenderState) bool {
	return state.SchemaVersion == protocol.SchemaVersion &&
		state.Scene == protocol.SceneBattle &&
		hasSemanticCapability(state, protocol.CapabilityBattle) &&
		state.Battle != nil &&
		len(state.Battle.Actors) >= 2
}

func (r *SemanticRenderer) RenderTimeline(timeline protocol.ReplayTimeline, atMS int64, options SemanticRenderOptions) (*image.RGBA, error) {
	if err := timeline.Validate(); err != nil {
		return nil, err
	}
	state, ok := timeline.StateAtMS(atMS)
	if !ok {
		return nil, ErrUnsupportedSemanticScene
	}
	options.AtMS = atMS
	return r.Render(state, options)
}

func (r *SemanticRenderer) Render(state protocol.RenderState, options SemanticRenderOptions) (*image.RGBA, error) {
	if r == nil {
		return nil, ErrUnsupportedSemanticScene
	}
	if err := protocol.Validate(state); err != nil {
		return nil, err
	}
	kind := SemanticScene(state)
	if kind == "" {
		return nil, ErrUnsupportedSemanticScene
	}
	width := options.Width
	height := options.Height
	if width <= 0 {
		width = OutputWidth
	}
	if height <= 0 {
		height = OutputHeight
	}
	if width > 4096 || height > 4096 {
		return nil, fmt.Errorf("semantic viewport %dx%d exceeds renderer limit", width, height)
	}
	if options.AtMS < 0 {
		options.AtMS = 0
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	switch kind {
	case SemanticSceneOverworld:
		r.drawOverworld(dst, state, options.AtMS)
	case SemanticSceneDialogue:
		r.drawDialogue(dst, state, options.AtMS)
	case SemanticSceneMenu:
		r.drawMenu(dst, state, options.AtMS)
	case SemanticSceneBattle:
		r.drawBattle(dst, state)
	default:
		return nil, ErrUnsupportedSemanticScene
	}
	return dst, nil
}

type semanticViewport struct {
	startX int
	startY int
	endX   int
	endY   int
	offsetX float64
	offsetY float64
	tileSize int
}

type semanticPoint struct {
	x float64
	y float64
}

func authoritativeSemanticPosition(actor protocol.ActorState) semanticPoint {
	if actor.Movement != nil && actor.Movement.Progress > 0 && actor.Movement.Progress <= 1 {
		p := actor.Movement.Progress
		return semanticPoint{
			x: float64(actor.Movement.From.X) + float64(actor.Movement.To.X-actor.Movement.From.X)*p,
			y: float64(actor.Movement.From.Y) + float64(actor.Movement.To.Y-actor.Movement.From.Y)*p,
		}
	}
	return semanticPoint{x: float64(actor.Position.X), y: float64(actor.Position.Y)}
}

func semanticCameraViewport(state protocol.RenderState, width, height, preferredTileSize int, focus semanticPoint) semanticViewport {
	mapWidth := maxInt(1, state.Map.Width)
	mapHeight := maxInt(1, state.Map.Height)
	tileSize := clampInt(preferredTileSize, 16, 48)
	viewTilesX := float64(width) / float64(tileSize)
	viewTilesY := float64(height) / float64(tileSize)
	maxCameraX := math.Max(0, float64(mapWidth)-viewTilesX)
	maxCameraY := math.Max(0, float64(mapHeight)-viewTilesY)
	cameraX := math.Max(0, math.Min(maxCameraX, focus.x+0.5-viewTilesX/2))
	cameraY := math.Max(0, math.Min(maxCameraY, focus.y+0.5-viewTilesY/2))
	startX := maxInt(0, int(math.Floor(cameraX))-1)
	startY := maxInt(0, int(math.Floor(cameraY))-1)
	endX := minInt(mapWidth, int(math.Ceil(cameraX+viewTilesX))+1)
	endY := minInt(mapHeight, int(math.Ceil(cameraY+viewTilesY))+1)
	smallOffsetX := 0.0
	smallOffsetY := 0.0
	if float64(mapWidth) <= viewTilesX {
		smallOffsetX = math.Max(0, (float64(width)-float64(mapWidth*tileSize))/2)
	}
	if float64(mapHeight) <= viewTilesY {
		smallOffsetY = math.Max(0, (float64(height)-float64(mapHeight*tileSize))/2)
	}
	return semanticViewport{
		startX: startX, startY: startY, endX: endX, endY: endY,
		offsetX: smallOffsetX - (cameraX-float64(startX))*float64(tileSize),
		offsetY: smallOffsetY - (cameraY-float64(startY))*float64(tileSize),
		tileSize: tileSize,
	}
}

func (r *SemanticRenderer) drawOverworld(dst *image.RGBA, state protocol.RenderState, atMS int64) {
	fillRect(dst, dst.Bounds(), parseSemanticColor(r.theme.Effects.Background, color.RGBA{R: 20, G: 34, B: 40, A: 255}))
	if state.Map == nil || state.Player == nil {
		return
	}
	preferred := r.theme.TileSize
	if dst.Bounds().Dx() < 700 {
		preferred = minInt(preferred, 28)
	}
	viewport := semanticCameraViewport(state, dst.Bounds().Dx(), dst.Bounds().Dy(), preferred, authoritativeSemanticPosition(*state.Player))

	for _, layer := range state.Layers {
		if layer.Kind != protocol.LayerTerrain {
			continue
		}
		r.drawSemanticLayer(dst, layer, viewport, atMS, false)
	}
	for _, layer := range state.Layers {
		if layer.Kind == protocol.LayerTerrain {
			continue
		}
		r.drawSemanticLayer(dst, layer, viewport, atMS, true)
	}
	for _, actor := range state.Entities {
		r.drawSemanticActor(dst, actor, viewport, false)
	}
	r.drawSemanticActor(dst, *state.Player, viewport, true)
	r.drawVignette(dst)
}

func (r *SemanticRenderer) drawSemanticLayer(dst *image.RGBA, layer protocol.TileLayer, viewport semanticViewport, atMS int64, objectLayer bool) {
	for worldY := viewport.startY; worldY < viewport.endY; worldY++ {
		for worldX := viewport.startX; worldX < viewport.endX; worldX++ {
			cell, ok := semanticLayerCell(layer, worldX, worldY)
			if !ok {
				continue
			}
			kind := string(cell.Kind)
			if objectLayer && (kind == "" || cell.Kind == protocol.TileUnknown) {
				continue
			}
			left := int(math.Round(viewport.offsetX + float64(worldX-viewport.startX)*float64(viewport.tileSize)))
			top := int(math.Round(viewport.offsetY + float64(worldY-viewport.startY)*float64(viewport.tileSize)))
			assetKey := ""
			if objectLayer {
				assetKey = kind
			} else {
				assetKey = semanticTerrainAssetKey(r.theme.Assets.Tiles, layer, worldX, worldY)
			}
			assetRef := ""
			if objectLayer {
				assetRef = r.theme.Assets.Objects[assetKey]
			} else {
				assetRef = r.theme.Assets.Tiles[assetKey]
			}
			r.drawSemanticTile(dst, cell, left, top, viewport.tileSize, atMS, objectLayer, assetRef)
		}
	}
}

func semanticLayerCell(layer protocol.TileLayer, worldX, worldY int) (protocol.TileCell, bool) {
	x := worldX - layer.Origin.X
	y := worldY - layer.Origin.Y
	if x < 0 || y < 0 || x >= layer.Width || y >= layer.Height {
		return protocol.TileCell{}, false
	}
	index := y*layer.Width + x
	if index < 0 || index >= len(layer.Cells) {
		return protocol.TileCell{}, false
	}
	return layer.Cells[index], true
}

func semanticTerrainAssetKey(assets map[string]string, layer protocol.TileLayer, x, y int) string {
	cell, ok := semanticLayerCell(layer, x, y)
	if !ok {
		return ""
	}
	kind := string(cell.Kind)
	if cell.Variant != "" {
		if _, ok := assets[kind+"."+cell.Variant]; ok {
			return kind + "." + cell.Variant
		}
	}
	if _, ok := assets[kind+".center"]; ok {
		north, _ := semanticLayerCell(layer, x, y-1)
		south, _ := semanticLayerCell(layer, x, y+1)
		west, _ := semanticLayerCell(layer, x-1, y)
		east, _ := semanticLayerCell(layer, x+1, y)
		vertical := "middle"
		if north.Kind != cell.Kind {
			vertical = "top"
		} else if south.Kind != cell.Kind {
			vertical = "bottom"
		}
		horizontal := "center"
		if west.Kind != cell.Kind {
			horizontal = "left"
		} else if east.Kind != cell.Kind {
			horizontal = "right"
		}
		key := kind + "." + vertical + "-" + horizontal
		if _, ok := assets[key]; ok {
			return key
		}
		return kind + ".center"
	}
	if _, ok := assets[kind]; ok {
		return kind
	}
	return ""
}

func (r *SemanticRenderer) drawSemanticTile(dst *image.RGBA, cell protocol.TileCell, x, y, size int, atMS int64, objectLayer bool, assetReference string) {
	kind := string(cell.Kind)
	if kind == "" {
		kind = "unknown"
	}
	style := r.theme.tileStyle(kind, objectLayer)
	fill := parseSemanticColor(style.Fill, color.RGBA{R: 24, G: 43, B: 58, A: 255})
	if !objectLayer {
		fillRect(dst, image.Rect(x, y, x+size+1, y+size+1), fill)
	}
	if assetReference != "" && r.drawSemanticAsset(dst, assetReference, image.Rect(x, y, x+size, y+size)) {
		return
	}
	detail := parseSemanticColor(style.Detail, fill)
	accent := parseSemanticColor(style.Accent, detail)
	pattern := style.Pattern
	if pattern == "" {
		pattern = kind
	}
	switch pattern {
	case "path":
		for i := 0; i < 4; i++ {
			px := x + int(float64(size)*(0.18+math.Mod(float64(i)*0.31, 0.7)))
			py := y + int(float64(size)*(0.20+math.Mod(float64(i)*0.43, 0.64)))
			w := maxInt(1, int(float64(size)*0.045))
			h := maxInt(1, int(float64(size)*0.035))
			fillRect(dst, image.Rect(px, py, px+w, py+h), detail)
		}
	case "floor":
		drawSemanticRect(dst, image.Rect(x+size*8/100, y+size*8/100, x+size*92/100, y+size*92/100), detail, maxInt(1, size/28))
	case "grass":
		for i := 0; i < 3; i++ {
			px := x + int(float64(size)*(0.22+float64(i)*0.28))
			baseY := y + size*72/100
			drawSemanticLine(dst, px, baseY, px-size*8/100, y+size*50/100, detail, maxInt(1, size/18))
			drawSemanticLine(dst, px, baseY, px+size*8/100, y+size*47/100, detail, maxInt(1, size/18))
		}
	case "water":
		period := maxInt(1, r.theme.Animation.WaterPeriodMS)
		phase := math.Mod(float64(atMS)/float64(period), 1)
		for row := -1; row < 4; row++ {
			yy := y + int((float64(row)+phase)*float64(size)/3)
			drawSemanticLine(dst, x+size*12/100, yy, x+size*46/100, yy, detail, maxInt(1, size/20))
			drawSemanticLine(dst, x+size*62/100, yy+size*8/100, x+size*88/100, yy+size*8/100, detail, maxInt(1, size/20))
		}
	case "tree":
		fillRect(dst, image.Rect(x+size*38/100, y+size*55/100, x+size*62/100, y+size*93/100), accent)
		fillSemanticCircle(dst, x+size/2, y+size*38/100, size*34/100, detail)
	case "ledge":
		fillRect(dst, image.Rect(x, y+size*72/100, x+size, y+size*84/100), detail)
	case "wall":
		drawSemanticLine(dst, x+size*12/100, y+size*34/100, x+size*88/100, y+size*34/100, detail, maxInt(1, size/22))
		drawSemanticLine(dst, x+size*12/100, y+size*68/100, x+size*88/100, y+size*68/100, detail, maxInt(1, size/22))
	case "door":
		fillRect(dst, image.Rect(x+size*22/100, y+size*12/100, x+size*78/100, y+size*94/100), detail)
		fillSemanticCircle(dst, x+size*66/100, y+size*55/100, maxInt(2, size*5/100), accent)
	case "warp":
		fillRect(dst, image.Rect(x+size/4, y+size/4, x+size*3/4, y+size*3/4), detail)
	case "sign":
		fillRect(dst, image.Rect(x+size*18/100, y+size*20/100, x+size*82/100, y+size*60/100), detail)
		fillRect(dst, image.Rect(x+size*45/100, y+size*58/100, x+size*55/100, y+size*92/100), accent)
	case "unknown":
		drawSemanticLine(dst, x, y, x+size, y+size, detail, 1)
		drawSemanticLine(dst, x+size, y, x, y+size, detail, 1)
	}
}

func (r *SemanticRenderer) drawSemanticAsset(dst *image.RGBA, reference string, target image.Rectangle) bool {
	ref, ok := parseSemanticAssetRef(reference)
	if !ok {
		return false
	}
	img := r.semanticAssetImage(ref.Reference)
	if img == nil {
		return false
	}
	source := img.Bounds()
	if ref.TileSize > 0 {
		source = image.Rect(
			ref.Column*ref.TileSize,
			ref.Row*ref.TileSize,
			(ref.Column+1)*ref.TileSize,
			(ref.Row+1)*ref.TileSize,
		)
		if !source.In(img.Bounds()) {
			return false
		}
	}
	repeat := maxInt(1, ref.Repeat)
	cellW := maxInt(1, target.Dx()/repeat)
	cellH := maxInt(1, target.Dy()/repeat)
	for row := 0; row < repeat; row++ {
		for col := 0; col < repeat; col++ {
			rect := image.Rect(
				target.Min.X+col*cellW,
				target.Min.Y+row*cellH,
				target.Min.X+(col+1)*cellW,
				target.Min.Y+(row+1)*cellH,
			)
			xdraw.NearestNeighbor.Scale(dst, rect, img, source, stddraw.Over, nil)
		}
	}
	return true
}

func (r *SemanticRenderer) semanticAssetImage(reference string) image.Image {
	ref, ok := parseSemanticAssetRef(reference)
	if !ok {
		return nil
	}
	key := ref.Path
	r.assetMu.RLock()
	img := r.assets[key]
	_, failed := r.failedAsset[key]
	r.assetMu.RUnlock()
	if img != nil || failed {
		return img
	}
	data, err := webassets.PublicThemeAsset(reference)
	if err != nil {
		r.assetMu.Lock()
		r.failedAsset[key] = struct{}{}
		r.assetMu.Unlock()
		return nil
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	r.assetMu.Lock()
	defer r.assetMu.Unlock()
	if err != nil {
		r.failedAsset[key] = struct{}{}
		return nil
	}
	r.assets[key] = decoded
	return decoded
}

func (r *SemanticRenderer) drawSemanticActor(dst *image.RGBA, actor protocol.ActorState, viewport semanticViewport, player bool) {
	position := authoritativeSemanticPosition(actor)
	x := int(math.Round(viewport.offsetX + (position.x-float64(viewport.startX))*float64(viewport.tileSize)))
	y := int(math.Round(viewport.offsetY + (position.y-float64(viewport.startY))*float64(viewport.tileSize)))
	size := viewport.tileSize
	if x+size < 0 || y+size < 0 || x >= dst.Bounds().Dx() || y >= dst.Bounds().Dy() {
		return
	}
	shadow := parseSemanticColor(r.theme.Effects.Shadow, color.RGBA{A: 55})
	fillSemanticEllipse(dst, x+size/2, y+size*82/100, size*30/100, maxInt(2, size*11/100), shadow)

	style := r.theme.actorStyle(string(actor.Kind), player)
	fill := parseSemanticColor(style.Fill, color.RGBA{R: 244, G: 247, B: 255, A: 255})
	stroke := parseSemanticColor(style.Stroke, color.RGBA{R: 24, G: 37, B: 42, A: 255})
	radius := size * 32 / 100
	fillSemanticCircle(dst, x+size/2, y+size/2, radius, fill)
	strokeSemanticCircle(dst, x+size/2, y+size/2, radius, stroke, maxInt(2, size*8/100))
	drawFacingMarker(dst, x+size/2, y+size/2, maxInt(3, size/7), actor.Facing, stroke)
}

func (r *SemanticRenderer) drawVignette(dst *image.RGBA) {
	vignette := parseSemanticColor(r.theme.Effects.Vignette, color.RGBA{A: 0})
	if vignette.A == 0 {
		return
	}
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	depth := maxInt(8, minInt(w, h)/14)
	for i := 0; i < depth; i++ {
		alpha := uint8(float64(vignette.A) * (1 - float64(i)/float64(depth)))
		c := vignette
		c.A = alpha / 5
		drawSemanticRect(dst, image.Rect(i, i, w-i, h-i), c, 1)
	}
}

func (r *SemanticRenderer) drawDialogue(dst *image.RGBA, state protocol.RenderState, atMS int64) {
	if hasSemanticOverworldSurface(state) {
		r.drawOverworld(dst, state, atMS)
	} else {
		fillRect(dst, dst.Bounds(), color.RGBA{R: 80, G: 128, B: 64, A: 255})
	}
	panel := parseSemanticColor(r.theme.UI.Panel, color.RGBA{R: 25, G: 39, B: 57, A: 230})
	textColor := parseSemanticColor(r.theme.UI.Text, color.RGBA{R: 247, G: 234, B: 213, A: 255})
	accent := parseSemanticColor(r.theme.UI.Accent, color.RGBA{R: 242, G: 189, B: 119, A: 255})
	margin := maxInt(12, dst.Bounds().Dx()/40)
	panelHeight := maxInt(115, dst.Bounds().Dy()/3)
	box := image.Rect(margin, dst.Bounds().Dy()-panelHeight-margin, dst.Bounds().Dx()-margin, dst.Bounds().Dy()-margin)
	drawSemanticPanel(dst, box, panel)
	y := box.Min.Y + 20
	if speaker := strings.TrimSpace(state.Dialogue.Speaker); speaker != "" {
		drawSemanticText(dst, box.Min.X+24, y, 1, accent, strings.ToUpper(speaker))
		y += 25
	}
	lines := wrapSemanticText(state.Dialogue.Text, maxInt(12, (box.Dx()-48)/14))
	for i, line := range lines {
		if i >= 4 {
			break
		}
		drawSemanticText(dst, box.Min.X+24, y, 2, textColor, line)
		y += 30
	}
	drawFacingMarker(dst, box.Max.X-28, box.Max.Y-22, 7, protocol.DirectionDown, textColor)
}

func (r *SemanticRenderer) drawMenu(dst *image.RGBA, state protocol.RenderState, atMS int64) {
	if hasSemanticOverworldSurface(state) {
		r.drawOverworld(dst, state, atMS)
	} else {
		fillRect(dst, dst.Bounds(), parseSemanticColor(r.theme.Effects.Background, color.RGBA{R: 20, G: 34, B: 40, A: 255}))
	}
	overlay := color.RGBA{A: 45}
	stddraw.Draw(dst, dst.Bounds(), image.NewUniform(overlay), image.Point{}, stddraw.Over)
	panel := parseSemanticColor(r.theme.UI.Panel, color.RGBA{R: 25, G: 39, B: 57, A: 230})
	textColor := parseSemanticColor(r.theme.UI.Text, color.RGBA{R: 247, G: 234, B: 213, A: 255})
	width := minInt(360, maxInt(220, dst.Bounds().Dx()*44/100))
	x := dst.Bounds().Dx() - width - 24
	y := 24
	entries := state.Menu.Entries
	height := 72 + maxInt(1, len(entries))*28
	box := image.Rect(x, y, x+width, minInt(dst.Bounds().Dy()-24, y+height))
	drawSemanticPanel(dst, box, panel)
	title := strings.TrimSpace(state.Menu.Title)
	if title == "" {
		title = "Menu"
	}
	drawSemanticText(dst, box.Min.X+18, box.Min.Y+18, 1, textColor, strings.ToUpper(title))
	selected := 0
	if state.Menu.Cursor != nil {
		selected = *state.Menu.Cursor
	}
	if len(entries) > 0 {
		count := fmt.Sprintf("%d/%d", minInt(len(entries), selected+1), len(entries))
		drawSemanticText(dst, box.Max.X-18-len(count)*7, box.Min.Y+18, 1, textColor, count)
	}
	drawSemanticLine(dst, box.Min.X+14, box.Min.Y+35, box.Max.X-14, box.Min.Y+35, color.RGBA{R: 32, G: 32, B: 32, A: 255}, 2)
	rowY := box.Min.Y + 52
	for i, entry := range entries {
		if rowY+20 >= box.Max.Y {
			break
		}
		c := textColor
		if entry.Disabled {
			c.A = 110
		}
		if i == selected {
			drawFacingMarker(dst, box.Min.X+22, rowY+6, 5, protocol.DirectionRight, c)
		}
		label := strings.TrimSpace(entry.Label)
		if label == "" {
			label = fmt.Sprintf("Option %d", i+1)
		}
		drawSemanticText(dst, box.Min.X+38, rowY, 1, c, label)
		rowY += 28
	}
	if len(entries) == 0 {
		drawSemanticText(dst, box.Min.X+22, rowY, 1, textColor, "Choose an option")
	}
}

func (r *SemanticRenderer) drawBattle(dst *image.RGBA, state protocol.RenderState) {
	drawBattleGradient(dst, r.theme.battlePaint("background", "#b5dab1"), parseSemanticColor(r.theme.Effects.Background, color.RGBA{R: 65, G: 94, B: 116, A: 255}))
	panel := parseSemanticColor(r.theme.battlePaint("panel", r.theme.UI.Panel), color.RGBA{R: 25, G: 39, B: 57, A: 230})
	textColor := parseSemanticColor(r.theme.UI.Text, color.RGBA{R: 247, G: 234, B: 213, A: 255})
	actors := state.Battle.Actors
	player := actors[0]
	opponent := actors[1]
	for _, actor := range actors {
		switch strings.ToLower(actor.Role) {
		case "player":
			player = actor
		case "opponent":
			opponent = actor
		}
	}
	margin := maxInt(16, dst.Bounds().Dx()/40)
	panelW := minInt(285, dst.Bounds().Dx()*43/100)
	panelH := 88
	opPanel := image.Rect(margin, margin, margin+panelW, margin+panelH)
	drawSemanticBattleActorPanel(dst, opPanel, opponent, panel, textColor, r.theme, false)

	tokenSize := minInt(136, dst.Bounds().Dy()/3)
	r.drawBattleToken(dst, dst.Bounds().Dx()-margin-tokenSize/2, margin+tokenSize/2, tokenSize/2, opponent, false)

	bottomY := dst.Bounds().Dy() - margin
	playerTokenSize := minInt(148, dst.Bounds().Dy()*38/100)
	r.drawBattleToken(dst, margin+playerTokenSize/2, bottomY-playerTokenSize/2, playerTokenSize/2, player, true)
	playerPanel := image.Rect(margin+playerTokenSize+10, bottomY-100, minInt(dst.Bounds().Dx()/2+80, margin+playerTokenSize+10+panelW), bottomY-10)
	drawSemanticBattleActorPanel(dst, playerPanel, player, panel, textColor, r.theme, true)

	movesX := maxInt(dst.Bounds().Dx()/2+18, playerPanel.Max.X+12)
	movesBox := image.Rect(movesX, dst.Bounds().Dy()-margin-150, dst.Bounds().Dx()-margin, dst.Bounds().Dy()-margin)
	drawSemanticPanel(dst, movesBox, panel)
	header := strings.ToUpper(strings.TrimSpace(state.Battle.Kind))
	if header == "" {
		header = "BATTLE"
	}
	phase := strings.ToUpper(strings.TrimSpace(state.Battle.Phase))
	if phase == "" {
		phase = "FIGHT"
	}
	drawSemanticText(dst, movesBox.Min.X+14, movesBox.Min.Y+15, 1, textColor, header)
	drawSemanticText(dst, movesBox.Max.X-14-len(phase)*7, movesBox.Min.Y+15, 1, textColor, phase)
	if len(state.Battle.Moves) == 0 {
		drawSemanticText(dst, movesBox.Min.X+22, movesBox.Min.Y+65, 1, textColor, "Waiting for battle actions...")
		return
	}
	gridTop := movesBox.Min.Y + 34
	cellW := maxInt(1, movesBox.Dx()/2)
	cellH := maxInt(1, (movesBox.Max.Y-gridTop)/2)
	for i, move := range state.Battle.Moves {
		if i >= 4 {
			break
		}
		col := i % 2
		row := i / 2
		cell := image.Rect(movesBox.Min.X+col*cellW, gridTop+row*cellH, movesBox.Min.X+(col+1)*cellW, gridTop+(row+1)*cellH)
		drawSemanticRect(dst, cell, color.RGBA{R: 32, G: 32, B: 32, A: 255}, 1)
		c := textColor
		if move.Disabled {
			c.A = 110
		}
		name := strings.ToUpper(firstNonEmpty(move.Name, move.ID, "MOVE"))
		drawSemanticText(dst, cell.Min.X+8, cell.Min.Y+9, 1, c, clipSemanticText(name, maxInt(4, (cell.Dx()-16)/7)))
		pp := fmt.Sprintf("PP %d", move.PP)
		if move.MaxPP > 0 {
			pp = fmt.Sprintf("PP %d/%d", move.PP, move.MaxPP)
		}
		drawSemanticText(dst, cell.Min.X+8, cell.Min.Y+31, 1, c, pp)
	}
}

func (r *SemanticRenderer) drawBattleToken(dst *image.RGBA, cx, cy, radius int, actor protocol.BattleActor, player bool) {
	accentKey := "opponentAccent"
	if player {
		accentKey = "playerAccent"
	}
	stroke := parseSemanticColor(r.theme.battlePaint(accentKey, "#f2bd77"), color.RGBA{R: 242, G: 189, B: 119, A: 255})
	fill := color.RGBA{R: 255, G: 255, B: 255, A: 28}
	fillSemanticCircle(dst, cx, cy, radius, fill)
	strokeSemanticCircle(dst, cx, cy, radius, stroke, 4)
	label := strings.TrimSpace(actor.Name)
	token := "?"
	if label != "" {
		token = strings.ToUpper(label[:1])
	}
	c := stroke
	if actor.Defeated {
		c.A = 110
	}
	drawSemanticText(dst, cx-radius/3, cy-radius/3, maxInt(2, radius/35), c, token)
}

func drawSemanticBattleActorPanel(dst *image.RGBA, rect image.Rectangle, actor protocol.BattleActor, panel, textColor color.RGBA, theme semanticTheme, player bool) {
	drawSemanticPanel(dst, rect, panel)
	name := strings.ToUpper(firstNonEmpty(actor.Name, "Unknown Pokemon"))
	drawSemanticText(dst, rect.Min.X+12, rect.Min.Y+12, 1, textColor, clipSemanticText(name, maxInt(5, (rect.Dx()-75)/7)))
	if actor.Level > 0 {
		level := fmt.Sprintf("LV%d", actor.Level)
		drawSemanticText(dst, rect.Max.X-12-len(level)*7, rect.Min.Y+12, 1, textColor, level)
	}
	bar := image.Rect(rect.Min.X+44, rect.Min.Y+39, rect.Max.X-14, rect.Min.Y+49)
	fillRect(dst, bar, color.RGBA{R: 32, G: 32, B: 32, A: 255})
	pct := 0
	if actor.MaxHP > 0 {
		pct = clampInt(int(math.Round(float64(actor.HP)*100/float64(actor.MaxHP))), 0, 100)
	}
	hpKey := "hpHealthy"
	if pct <= 20 {
		hpKey = "hpDanger"
	} else if pct <= 50 {
		hpKey = "hpWarn"
	}
	hp := parseSemanticColor(theme.battlePaint(hpKey, "#64be67"), color.RGBA{R: 100, G: 190, B: 103, A: 255})
	fillRect(dst, image.Rect(bar.Min.X+2, bar.Min.Y+2, bar.Min.X+2+(bar.Dx()-4)*pct/100, bar.Max.Y-2), hp)
	drawSemanticText(dst, rect.Min.X+12, rect.Min.Y+38, 1, textColor, "HP")
	status := strings.TrimSpace(actor.Status)
	if player {
		hpText := fmt.Sprintf("%d / %d", actor.HP, actor.MaxHP)
		drawSemanticText(dst, rect.Max.X-12-len(hpText)*7, rect.Min.Y+60, 1, textColor, hpText)
	}
	if status != "" {
		drawSemanticText(dst, rect.Min.X+12, rect.Min.Y+60, 1, textColor, status)
	}
}

func drawBattleGradient(dst *image.RGBA, paint string, fallback color.RGBA) {
	colors := semanticGradientColors(paint)
	if len(colors) == 0 {
		fillRect(dst, dst.Bounds(), fallback)
		return
	}
	if len(colors) == 1 {
		fillRect(dst, dst.Bounds(), colors[0])
		return
	}
	h := dst.Bounds().Dy()
	for y := 0; y < h; y++ {
		p := float64(y) / float64(maxInt(1, h-1))
		segment := p * float64(len(colors)-1)
		index := minInt(len(colors)-2, int(math.Floor(segment)))
		local := segment - float64(index)
		c := blendSemanticColor(colors[index], colors[index+1], local)
		fillRect(dst, image.Rect(0, y, dst.Bounds().Dx(), y+1), c)
	}
}

func semanticGradientColors(value string) []color.RGBA {
	var out []color.RGBA
	for i := 0; i+7 <= len(value); i++ {
		if value[i] != '#' {
			continue
		}
		candidate := value[i : i+7]
		c := parseSemanticColor(candidate, color.RGBA{})
		if c.A == 255 {
			out = append(out, c)
			i += 6
		}
	}
	return out
}

func blendSemanticColor(a, b color.RGBA, p float64) color.RGBA {
	p = math.Max(0, math.Min(1, p))
	mix := func(x, y uint8) uint8 { return uint8(float64(x)+(float64(y)-float64(x))*p+0.5) }
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}

func drawSemanticPanel(dst *image.RGBA, rect image.Rectangle, panel color.RGBA) {
	fillRect(dst, rect, panel)
	drawSemanticRect(dst, rect, color.RGBA{R: 32, G: 32, B: 32, A: 255}, 3)
	inner := rect.Inset(5)
	drawSemanticRect(dst, inner, color.RGBA{R: 184, G: 184, B: 152, A: 255}, 2)
}

func drawSemanticRect(dst *image.RGBA, rect image.Rectangle, c color.RGBA, width int) {
	if width < 1 {
		width = 1
	}
	for i := 0; i < width; i++ {
		r := rect.Inset(i)
		if r.Empty() {
			break
		}
		fillRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), c)
		fillRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), c)
		fillRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), c)
		fillRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), c)
	}
}

func drawSemanticLine(dst *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, width int) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	radius := maxInt(0, width/2)
	for {
		fillRect(dst, image.Rect(x0-radius, y0-radius, x0+radius+1, y0+radius+1), c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func fillSemanticCircle(dst *image.RGBA, cx, cy, radius int, c color.RGBA) {
	if radius <= 0 {
		return
	}
	r2 := radius * radius
	for y := -radius; y <= radius; y++ {
		x := int(math.Sqrt(float64(maxInt(0, r2-y*y))))
		fillRect(dst, image.Rect(cx-x, cy+y, cx+x+1, cy+y+1), c)
	}
}

func strokeSemanticCircle(dst *image.RGBA, cx, cy, radius int, c color.RGBA, width int) {
	for i := 0; i < maxInt(1, width); i++ {
		r := radius - i
		if r <= 0 {
			break
		}
		for degree := 0; degree < 360; degree += 2 {
			radian := float64(degree) * math.Pi / 180
			x := cx + int(math.Round(math.Cos(radian)*float64(r)))
			y := cy + int(math.Round(math.Sin(radian)*float64(r)))
			fillRect(dst, image.Rect(x, y, x+2, y+2), c)
		}
	}
}

func fillSemanticEllipse(dst *image.RGBA, cx, cy, rx, ry int, c color.RGBA) {
	if rx <= 0 || ry <= 0 {
		return
	}
	for y := -ry; y <= ry; y++ {
		p := 1 - float64(y*y)/float64(ry*ry)
		if p < 0 {
			continue
		}
		x := int(math.Sqrt(p) * float64(rx))
		fillRect(dst, image.Rect(cx-x, cy+y, cx+x+1, cy+y+1), c)
	}
}

func drawFacingMarker(dst *image.RGBA, cx, cy, size int, facing protocol.Direction, c color.RGBA) {
	switch facing {
	case protocol.DirectionUp:
		fillSemanticTriangle(dst, image.Pt(cx, cy-size), image.Pt(cx-size, cy+size), image.Pt(cx+size, cy+size), c)
	case protocol.DirectionLeft:
		fillSemanticTriangle(dst, image.Pt(cx-size, cy), image.Pt(cx+size, cy-size), image.Pt(cx+size, cy+size), c)
	case protocol.DirectionRight:
		fillSemanticTriangle(dst, image.Pt(cx+size, cy), image.Pt(cx-size, cy-size), image.Pt(cx-size, cy+size), c)
	default:
		fillSemanticTriangle(dst, image.Pt(cx, cy+size), image.Pt(cx-size, cy-size), image.Pt(cx+size, cy-size), c)
	}
}

func fillSemanticTriangle(dst *image.RGBA, a, b, c image.Point, fill color.RGBA) {
	minY := minInt(a.Y, minInt(b.Y, c.Y))
	maxY := maxInt(a.Y, maxInt(b.Y, c.Y))
	points := []image.Point{a, b, c}
	for y := minY; y <= maxY; y++ {
		var xs []int
		for i := 0; i < 3; i++ {
			p1 := points[i]
			p2 := points[(i+1)%3]
			if p1.Y == p2.Y || y < minInt(p1.Y, p2.Y) || y > maxInt(p1.Y, p2.Y) {
				continue
			}
			x := p1.X + (y-p1.Y)*(p2.X-p1.X)/(p2.Y-p1.Y)
			xs = append(xs, x)
		}
		if len(xs) >= 2 {
			if xs[0] > xs[1] {
				xs[0], xs[1] = xs[1], xs[0]
			}
			fillRect(dst, image.Rect(xs[0], y, xs[1]+1, y+1), fill)
		}
	}
}

func drawSemanticText(dst *image.RGBA, x, y, scale int, c color.RGBA, value string) {
	value = semanticASCII(value)
	if value == "" {
		return
	}
	scale = maxInt(1, scale)
	width := maxInt(1, len(value)*7)
	tmp := image.NewRGBA(image.Rect(0, 0, width, 14))
	d := font.Drawer{
		Dst:  tmp,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(0, 11),
	}
	d.DrawString(value)
	target := image.Rect(x, y, x+tmp.Bounds().Dx()*scale, y+tmp.Bounds().Dy()*scale)
	xdraw.NearestNeighbor.Scale(dst, target, tmp, tmp.Bounds(), stddraw.Over, nil)
}

func semanticASCII(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
		} else if r == '\n' || r == '\t' {
			b.WriteByte(' ')
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

func wrapSemanticText(value string, width int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return []string{"..."}
	}
	width = maxInt(8, width)
	var lines []string
	var line string
	for _, word := range strings.Fields(value) {
		if len(word) > width {
			word = clipSemanticText(word, width)
		}
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) <= width {
			line += " " + word
			continue
		}
		lines = append(lines, line)
		line = word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func clipSemanticText(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

// RenderClassicRGB scales an authoritative framebuffer into a 16:9 semantic
// video surface. Semantic mode uses it as a per-frame compatibility fallback
// whenever the recorded state cannot be represented by ModernSceneRenderer.
func RenderClassicRGB(rgb []byte, sourceWidth, sourceHeight int, width, height int) (*image.RGBA, error) {
	if sourceWidth <= 0 || sourceHeight <= 0 || len(rgb) != sourceWidth*sourceHeight*3 {
		return nil, fmt.Errorf("classic RGB frame has invalid dimensions/data")
	}
	if width <= 0 {
		width = SemanticVideoWidth
	}
	if height <= 0 {
		height = SemanticVideoHeight
	}
	src := image.NewRGBA(image.Rect(0, 0, sourceWidth, sourceHeight))
	for y := 0; y < sourceHeight; y++ {
		for x := 0; x < sourceWidth; x++ {
			i := (y*sourceWidth + x) * 3
			src.SetRGBA(x, y, color.RGBA{R: rgb[i], G: rgb[i+1], B: rgb[i+2], A: 255})
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	fillRect(dst, dst.Bounds(), color.RGBA{A: 255})
	scale := math.Min(float64(width)/float64(sourceWidth), float64(height)/float64(sourceHeight))
	drawW := maxInt(1, int(math.Floor(float64(sourceWidth)*scale)))
	drawH := maxInt(1, int(math.Floor(float64(sourceHeight)*scale)))
	x := (width - drawW) / 2
	y := (height - drawH) / 2
	xdraw.NearestNeighbor.Scale(dst, image.Rect(x, y, x+drawW, y+drawH), src, src.Bounds(), stddraw.Src, nil)
	return dst, nil
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
