package integ

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Frigate NVR : GET /api/stats. Sans authentification sur le port interne
// 5000 (le port 8971 exige une connexion et n'est pas géré : pointer url sur
// le 5000). Une caméra est « en ligne » quand elle reçoit des images
// (camera_fps > 0). Relevé toutes les 10 s.
type frigate struct {
	url string
	hc  *http.Client
	fps *tile.Ring
}

func newFrigate(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	return &frigate{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false), fps: tile.NewRing(60)}, nil
}

func (f *frigate) Fields() []tile.Field {
	return []tile.Field{
		tile.F("cameras", "Caméras", tile.Stat, tile.OnTile),
		tile.F("detection_fps", "Détections par seconde", tile.Stat, tile.OnTile),
		tile.F("inference", "Inférence", tile.Stat, tile.OnTile),
		tile.F("storage", "Enregistrements", tile.Meter, tile.OnTile),
		tile.F("fps_hist", "Détections (historique)", tile.Spark, tile.InDetail),
		tile.F("camera_list", "Caméras (images par seconde)", tile.Table, tile.InDetail),
		tile.F("gpu", "GPU", tile.Bars, tile.InDetail),
		tile.F("uptime", "Disponibilité", tile.Stat, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (f *frigate) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 10*time.Second, func() {
		v, err := f.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func (f *frigate) poll(ctx context.Context) (tile.Values, error) {
	var r struct {
		Cameras map[string]struct {
			CameraFPS    float64 `json:"camera_fps"`
			ProcessFPS   float64 `json:"process_fps"`
			SkippedFPS   float64 `json:"skipped_fps"`
			DetectionFPS float64 `json:"detection_fps"`
		}
		Detectors map[string]struct {
			InferenceSpeed float64 `json:"inference_speed"`
		}
		DetectionFPS float64                              `json:"detection_fps"`
		GPUUsages    map[string]struct{ GPU, Mem string } `json:"gpu_usages"`
		Service      struct {
			Uptime  float64
			Version string
			Storage map[string]struct{ Total, Used, Free float64 }
		}
	}
	if err := getJSON(ctx, f.hc, f.url+"/api/stats", &r, nil); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(r.Cameras))
	for n := range r.Cameras {
		names = append(names, n)
	}
	sort.Strings(names)
	up := 0
	var rows [][]tile.Cell
	for _, n := range names {
		c := r.Cameras[n]
		st, lvl := "En ligne", "ok"
		if c.CameraFPS > 0 {
			up++
		} else {
			st, lvl = "Hors ligne", "crit"
		}
		if len(rows) < 15 {
			rows = append(rows, []tile.Cell{{Text: n}, {Text: st, Level: lvl}, {Text: tile.Num(c.CameraFPS)}, {Text: tile.Num(c.ProcessFPS)}, {Text: tile.Num(c.DetectionFPS)}, {Text: tile.Num(c.SkippedFPS)}})
		}
	}
	cv := tile.Value{Text: fmt.Sprintf("%d / %d en ligne", up, len(names)), Level: "ok"}
	if up < len(names) {
		cv.Level = "crit"
	}
	v := tile.Values{
		"cameras":       cv,
		"detection_fps": {Text: tile.Num(r.DetectionFPS)},
		"fps_hist":      {Points: f.fps.Push(r.DetectionFPS), Text: tile.Num(r.DetectionFPS) + "/s"},
		"camera_list":   {Head: []string{"Caméra", "État", "Capture", "Traitement", "Détection", "Perdues"}, Rows: rows},
	}
	// Détecteur le plus lent : c'est lui qui limite le débit de détection.
	var inf float64
	for _, d := range r.Detectors {
		inf = max(inf, d.InferenceSpeed)
	}
	if inf > 0 {
		v["inference"] = tile.Value{Text: tile.Num(inf) + " ms", Level: tile.Level(inf, 50, 100)}
	}
	if r.Service.Version != "" {
		v["version"] = tile.Value{Text: r.Service.Version}
	}
	if r.Service.Uptime > 0 {
		v["uptime"] = tile.Value{Text: tile.Dur(time.Duration(r.Service.Uptime) * time.Second)}
	}
	// Stockage : le point de montage des enregistrements, sinon le premier.
	var sk string
	for k := range r.Service.Storage {
		if strings.Contains(k, "recordings") || sk == "" {
			sk = k
		}
	}
	if st, ok := r.Service.Storage[sk]; ok && st.Total > 0 {
		pct := st.Used / st.Total * 100
		v["storage"] = tile.Value{Pct: pct, Text: tile.Pct(pct), Sub: tile.Bytes(st.Used*(1<<20)) + " / " + tile.Bytes(st.Total*(1<<20)), Level: tile.Level(pct, 85, 95)}
	}
	var bars []tile.Bar
	gn := make([]string, 0, len(r.GPUUsages))
	for n := range r.GPUUsages {
		gn = append(gn, n)
	}
	sort.Strings(gn)
	for _, n := range gn {
		var p float64
		if _, err := fmt.Sscanf(strings.TrimSpace(r.GPUUsages[n].GPU), "%f", &p); err == nil {
			bars = append(bars, tile.Bar{Label: n, Text: tile.Pct(p), Pct: p, Level: tile.Level(p, 80, 95)})
		}
	}
	if len(bars) > 0 {
		v["gpu"] = tile.Value{Bars: bars}
	}
	return v, nil
}
