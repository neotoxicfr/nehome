package integ

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Immich : clé d'API en en-tête x-api-key. Files de tâches toutes les 30 s ;
// statistiques, stockage et version toutes les 5 min.
type immich struct {
	url, key string
	hc       *http.Client
}

func newImmich(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	key, err := secret("key")
	if err != nil {
		return nil, err
	}
	return &immich{url: strings.TrimRight(t.URL, "/") + "/api", key: key, hc: httpClient(t.InsecureTLS, false)}, nil
}

func (i *immich) Fields() []tile.Field {
	return []tile.Field{
		tile.F("photos", "Photos", tile.Stat, tile.OnTile),
		tile.F("videos", "Vidéos", tile.Stat, tile.OnTile),
		tile.F("usage", "Espace utilisé par la bibliothèque", tile.Stat, tile.OnTile),
		tile.F("disk", "Disque du serveur", tile.Meter, tile.OnTile),
		tile.F("users", "Utilisateurs", tile.Table, tile.InDetail),
		tile.F("jobs", "Tâches en cours", tile.Stat, tile.InDetail),
		tile.F("jobs_table", "Files de tâches", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (i *immich) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, i.hc, i.url+path, out, func(r *http.Request) { r.Header.Set("x-api-key", i.key) })
}

func (i *immich) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, 30*time.Second, func() {
		v := i.jobs(ctx)
		if ctx.Err() == nil && len(v) > 0 {
			e[0](v, nil)
		}
	})
	every(ctx, 5*time.Minute, func() {
		v, err := i.slow(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (i *immich) slow(ctx context.Context) (tile.Values, error) {
	var st struct {
		Photos, Videos, Usage float64
		UsageByUser           []struct {
			UserName       string
			Photos, Videos float64
			Usage          float64
			Quota          *float64 `json:"quotaSizeInBytes"`
		}
	}
	if err := i.get(ctx, "/server/statistics", &st); err != nil {
		return nil, err
	}
	v := tile.Values{
		"photos": {Text: tile.Num(st.Photos)},
		"videos": {Text: tile.Num(st.Videos)},
		"usage":  {Text: tile.Bytes(st.Usage)},
	}
	tbl := tile.Value{Head: []string{"Utilisateur", "Photos", "Vidéos", "Espace", "Quota"}}
	for _, u := range st.UsageByUser {
		quota, cl := "illimité", ""
		if u.Quota != nil && *u.Quota > 0 {
			p := u.Usage / *u.Quota * 100
			quota, cl = tile.Pct(p)+" de "+tile.Bytes(*u.Quota), tile.Level(p, 85, 95)
		}
		if len(tbl.Rows) < 15 {
			tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: u.UserName}, {Text: tile.Num(u.Photos)}, {Text: tile.Num(u.Videos)}, {Text: tile.Bytes(u.Usage)}, {Text: quota, Level: cl}})
		}
	}
	v["users"] = tbl

	var sto struct {
		DiskSizeRaw, DiskUseRaw, DiskAvailableRaw float64
		DiskUsagePercentage                       float64
	}
	if i.get(ctx, "/server/storage", &sto) == nil && sto.DiskSizeRaw > 0 {
		v["disk"] = tile.Value{Text: tile.Pct(sto.DiskUsagePercentage), Pct: sto.DiskUsagePercentage, Level: tile.Level(sto.DiskUsagePercentage, 85, 95),
			Sub: tile.Bytes(sto.DiskAvailableRaw) + " libres / " + tile.Bytes(sto.DiskSizeRaw)}
	}
	var ver struct{ Major, Minor, Patch int }
	if i.get(ctx, "/server/version", &ver) == nil {
		cur := fmt.Sprintf("v%d.%d.%d", ver.Major, ver.Minor, ver.Patch)
		vv := tile.Value{Text: cur}
		var chk struct{ ReleaseVersion string }
		if i.get(ctx, "/server/version-check", &chk) == nil && chk.ReleaseVersion != "" && chk.ReleaseVersion != cur {
			vv.Level, vv.Sub = "warn", chk.ReleaseVersion+" disponible"
		}
		v["version"] = vv
	}
	return v, nil
}

// jobs : somme des tâches actives, en attente et en échec de toutes les files.
func (i *immich) jobs(ctx context.Context) tile.Values {
	type counts struct{ Active, Waiting, Failed, Delayed, Paused float64 }
	jobs := map[string]counts{}
	var qs []struct {
		Name       string
		Statistics counts
	}
	if i.get(ctx, "/queues", &qs) == nil { // /jobs est déprécié
		for _, q := range qs {
			jobs[q.Name] = q.Statistics
		}
	} else {
		var old map[string]struct{ JobCounts counts }
		if i.get(ctx, "/jobs", &old) != nil {
			return nil
		}
		for n, j := range old {
			jobs[n] = j.JobCounts
		}
	}
	var act, wait, fail float64
	tbl := tile.Value{Head: []string{"File", "Actives", "En attente", "En échec"}}
	names := make([]string, 0, len(jobs))
	for n := range jobs {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		c := jobs[n]
		act += c.Active
		wait += c.Waiting
		fail += c.Failed
		if (c.Active+c.Waiting+c.Failed > 0) && len(tbl.Rows) < 15 {
			cl := ""
			if c.Failed > 0 {
				cl = "warn"
			}
			tbl.Rows = append(tbl.Rows, []tile.Cell{{Text: n}, {Text: tile.Num(c.Active)}, {Text: tile.Num(c.Waiting)}, {Text: tile.Num(c.Failed), Level: cl}})
		}
	}
	jv := tile.Value{Text: tile.Num(act), Sub: fmt.Sprintf("%s en attente · %s en échec", tile.Num(wait), tile.Num(fail))}
	if fail > 0 {
		jv.Level = "warn"
	}
	return tile.Values{"jobs": jv, "jobs_table": tbl}
}
