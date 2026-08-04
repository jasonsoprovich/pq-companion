package mapgen

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// Panel is one labelled rendering in a comparison sheet.
type Panel struct {
	Label    string
	Segments []Segment
	Stroke   string
}

// RenderComparison writes a side-by-side SVG of the same zone extracted several
// ways, all on identical bounds so the panels are directly comparable.
//
// This exists because the classifier's metrics say where it is uncertain but not
// whether it is right. Cartography has to be checked by eye: a mirrored
// transform, an over-simplified silhouette and an empty terrain boundary all
// produce numbers that look reasonable.
func RenderComparison(path, title string, panels []Panel, panelW int) error {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range panels {
		for _, s := range p.Segments {
			for _, v := range [2]Vec3{s.A, s.B} {
				minX, minY = math.Min(minX, v.X), math.Min(minY, v.Y)
				maxX, maxY = math.Max(maxX, v.X), math.Max(maxY, v.Y)
			}
		}
	}
	if math.IsInf(minX, 1) {
		minX, minY, maxX, maxY = 0, 0, 1, 1
	}
	span := math.Max(maxX-minX, maxY-minY)
	if span <= 0 {
		span = 1
	}

	const pad, header = 12.0, 26.0
	scale := (float64(panelW) - 2*pad) / span
	panelH := float64(panelW) + header
	totalW := panelW * len(panels)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%.0f" `+
		`style="background:#0b1220;font-family:ui-monospace,monospace">`, totalW, panelH)
	fmt.Fprintf(&b, `<text x="8" y="16" fill="#94a3b8" font-size="12">%s</text>`, title)

	for i, p := range panels {
		ox := float64(i * panelW)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="%s" font-size="11">%s (%d)</text>`,
			ox+8, header-4, p.Stroke, p.Label, len(p.Segments))
		if i > 0 {
			fmt.Fprintf(&b, `<line x1="%.0f" y1="0" x2="%.0f" y2="%.0f" stroke="#1e293b"/>`,
				ox, ox, panelH)
		}
		fmt.Fprintf(&b, `<g stroke="%s" stroke-width="0.9" fill="none" stroke-linecap="round">`,
			p.Stroke)
		for _, s := range p.Segments {
			// North-up, east-right: screen X and Y both grow with their map
			// component (see ZoneMap.tsx for the derivation from game geography).
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				ox+pad+(s.A.X-minX)*scale, header+pad+(s.A.Y-minY)*scale,
				ox+pad+(s.B.X-minX)*scale, header+pad+(s.B.Y-minY)*scale)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// AllTechniques extracts a zone every way, for comparison sheets.
func AllTechniques(z *Zone, c Classification) []Panel {
	floor := z.BoundaryEdges()
	boundaryPlusWalls := append(floor, z.FreestandingWallEdges(floor)...)
	return []Panel{
		{
			Label:    "boundary",
			Stroke:   "#7dd3fc",
			Segments: SimplifyCollinear(Chain(boundaryPlusWalls), 0.03),
		},
		{
			Label:  "contours",
			Stroke: "#5eead4",
			Segments: SimplifyRDP(
				Chain(z.Contours(contourInterval(c.ZSpan))), contourRDPEpsilon),
		},
		{
			Label:    "silhouette",
			Stroke:   "#a78bfa",
			Segments: z.Silhouette(),
		},
	}
}
