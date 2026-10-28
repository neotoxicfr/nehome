package integ

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Nextcloud (appli serverinfo, compte admin + mot de passe d'appli en basic
// auth ; le jeton NC-Token n'est pas celui-là). L'appel par défaut saute les
// applis et la mise à jour (0,1 s) : 60 s. Le contrôle des mises à jour coûte
// ~2 s : une fois par heure, désactivable par l'option skipApps: "true".
type nextcloud struct {
	url, user, pass string
	skipApps        bool
	hc              *http.Client
}

type ncInfo struct {
	Nextcloud struct {
		System struct {
			Version     string    `json:"version"`
			Local       string    `json:"memcache.local"`
			Distributed string    `json:"memcache.distributed"`
			Locking     string    `json:"memcache.locking"`
			Free        float64   `json:"freespace"`
			Load        []float64 `json:"cpuload"`
			CPUs        int       `json:"cpunum"`
			MemTotal    float64   `json:"mem_total"` // kio
			MemFree     float64   `json:"mem_free"`
			Apps        *struct {
				Installed int             `json:"num_installed"`
				Updates   json.RawMessage `json:"app_updates"` // [] si vide, {"appli":"version"} sinon
			} `json:"apps"`
			Update *struct {
				Available bool `json:"available"`
			} `json:"update"`
		} `json:"system"`
		Storage struct {
			Users       int     `json:"num_users"`
			Disabled    int     `json:"num_disabled_users"`
			Files       int     `json:"num_files"`
			Storages    int     `json:"num_storages"`
			Appdata     float64 `json:"size_appdata_storage"`
			AppdataFile int     `json:"num_files_appdata"`
		} `json:"storage"`
		Shares struct {
			Total      int `json:"num_shares"`
			User       int `json:"num_shares_user"`
			Groups     int `json:"num_shares_groups"`
			Link       int `json:"num_shares_link"`
			LinkNoPass int `json:"num_shares_link_no_password"`
			Mail       int `json:"num_shares_mail"`
			FedSent    int `json:"num_fed_shares_sent"`
			FedRecv    int `json:"num_fed_shares_received"`
		} `json:"shares"`
	} `json:"nextcloud"`
	Server struct {
		Webserver string `json:"webserver"`
		PHP       struct {
			Version string `json:"version"`
			Opcache struct {
				Enabled bool `json:"opcache_enabled"`
				Full    bool `json:"cache_full"`
				Mem     struct {
					Used   float64 `json:"used_memory"`
					Free   float64 `json:"free_memory"`
					Wasted float64 `json:"wasted_memory"`
				} `json:"memory_usage"`
				Stats struct {
					Scripts int     `json:"num_cached_scripts"`
					HitRate float64 `json:"opcache_hit_rate"`
				} `json:"opcache_statistics"`
			} `json:"opcache"`
			APCu *struct {
				Cache struct {
					Hits    float64 `json:"num_hits"`
					Misses  float64 `json:"num_misses"`
					Entries int     `json:"num_entries"`
				} `json:"cache"`
				SMA struct {
					Segs  float64 `json:"num_seg"`
					Size  float64 `json:"seg_size"`
					Avail float64 `json:"avail_mem"`
				} `json:"sma"`
			} `json:"apcu"`
		} `json:"php"`
		FPM *struct {
			Active      int `json:"active-processes"`
			Total       int `json:"total-processes"`
			Accepted    int `json:"accepted-conn"`
			MaxChildren int `json:"max-children-reached"`
			Slow        int `json:"slow-requests"`
			Queue       int `json:"listen-queue"`
		} `json:"fpm"`
		Database struct {
			Type    string  `json:"type"`
			Version string  `json:"version"`
			Size    float64 `json:"size"`
		} `json:"database"`
	} `json:"server"`
	Active struct {
		M5    int `json:"last5minutes"`
		H1    int `json:"last1hour"`
		H24   int `json:"last24hours"`
		D7    int `json:"last7days"`
		Month int `json:"last1month"`
		Year  int `json:"lastyear"`
	} `json:"activeUsers"`
}

func newNextcloud(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	n := &nextcloud{url: strings.TrimRight(t.URL, "/"), hc: httpClient(t.InsecureTLS, false), skipApps: t.Options["skipApps"] == "true"}
	// L'identifiant n'a rien de secret : option, « admin » par défaut.
	n.user = t.Options["user"]
	if n.user == "" {
		n.user = "admin"
	}
	var err error
	if n.pass, err = secret("pass"); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *nextcloud) Fields() []tile.Field {
	return []tile.Field{
		tile.F("active", "Utilisateurs actifs (5 min)", tile.Stat, tile.OnTile),
		tile.F("files", "Fichiers", tile.Stat, tile.OnTile),
		tile.F("free", "Espace libre", tile.Stat, tile.OnTile),
		tile.F("updates", "Mises à jour", tile.Stat, tile.OnTile),
		tile.F("active_long", "Utilisateurs actifs (long terme)", tile.Stat, tile.InDetail),
		tile.F("users", "Comptes", tile.Stat, tile.InDetail),
		tile.F("shares", "Partages", tile.Stat, tile.InDetail),
		tile.F("version", "Nextcloud", tile.Stat, tile.InDetail),
		tile.F("php", "PHP", tile.Stat, tile.InDetail),
		tile.F("db", "Base de données", tile.Stat, tile.InDetail),
		tile.F("opcache", "OPcache", tile.Meter, tile.InDetail),
		tile.F("apcu", "APCu", tile.Meter, tile.InDetail),
		tile.F("caches", "Caches configurés", tile.Stat, tile.InDetail),
		tile.F("fpm", "PHP-FPM", tile.Stat, tile.InDetail),
		tile.F("load", "Charge CPU", tile.Stat, tile.InDetail),
		tile.F("ram", "Mémoire (vue par PHP)", tile.Meter, tile.InDetail),
		tile.F("storages", "Stockages", tile.Stat, tile.InDetail),
		tile.F("appdata", "Données des applis", tile.Stat, tile.InDetail),
		tile.F("app_updates", "Applis à mettre à jour", tile.Table, tile.InDetail),
	}
}

func (n *nextcloud) get(ctx context.Context, query string) (*ncInfo, error) {
	var r struct {
		OCS struct {
			Data ncInfo `json:"data"`
		} `json:"ocs"`
	}
	err := getJSON(ctx, n.hc, n.url+"/ocs/v2.php/apps/serverinfo/api/v1/info?format=json"+query, &r, func(req *http.Request) {
		req.SetBasicAuth(n.user, n.pass)
		req.Header.Set("OCS-APIRequest", "true")
	})
	if err != nil {
		return nil, err
	}
	return &r.OCS.Data, nil
}

func (n *nextcloud) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	if !n.skipApps {
		go every(ctx, time.Hour, func() {
			// Requête secondaire : un échec ne masque pas la tuile.
			if i, err := n.get(ctx, "&skipApps=false&skipUpdate=false"); err == nil && ctx.Err() == nil {
				e[0](ncUpdates(i), nil)
			}
		})
	}
	every(ctx, time.Minute, func() {
		i, err := n.get(ctx, "")
		if ctx.Err() == nil {
			if err != nil {
				e[1](nil, err)
			} else {
				e[1](ncValues(i), nil)
			}
		}
	})
}

func ncPlural(n int, one, many string) string {
	if n > 1 {
		return fmt.Sprintf("%d %s", n, many)
	}
	return fmt.Sprintf("%d %s", n, one)
}

// ncUpdates lit la réponse complète (skipApps=false) : mise à jour du serveur
// et des applis.
func ncUpdates(i *ncInfo) tile.Values {
	s := i.Nextcloud.System
	apps := map[string]string{}
	if s.Apps != nil {
		json.Unmarshal(s.Apps.Updates, &apps) // [] (aucune) ne se décode pas en map : reste vide
	}
	names := make([]string, 0, len(apps))
	for a := range apps {
		names = append(names, a)
	}
	slices.Sort(names)
	var rows [][]tile.Cell
	for _, a := range names {
		if len(rows) < 15 {
			rows = append(rows, []tile.Cell{{Text: a}, {Text: apps[a]}})
		}
	}
	up := tile.Value{Text: "à jour", Level: "ok"}
	var parts []string
	if s.Update != nil && s.Update.Available {
		parts = append(parts, "Nextcloud")
	}
	if len(apps) > 0 {
		parts = append(parts, ncPlural(len(apps), "appli", "applis"))
	}
	if len(parts) > 0 {
		up = tile.Value{Text: strings.Join(parts, " + "), Level: "warn"}
	}
	if s.Apps != nil {
		up.Sub = ncPlural(s.Apps.Installed, "appli installée", "applis installées")
	}
	return tile.Values{
		"updates":     up,
		"app_updates": {Head: []string{"Appli", "Nouvelle version"}, Rows: rows, Text: "aucune"},
	}
}

// ncMemcache retire le préfixe de classe : « \OC\Memcache\APCu » → « APCu ».
func ncMemcache(s string) string {
	if s == "" {
		return "aucun"
	}
	return s[strings.LastIndex(s, `\`)+1:]
}

func ncValues(i *ncInfo) tile.Values {
	s, st, sh, srv, a := i.Nextcloud.System, i.Nextcloud.Storage, i.Nextcloud.Shares, i.Server, i.Active
	v := tile.Values{
		"active":      {Text: fmt.Sprint(a.M5), Sub: fmt.Sprintf("1 h : %d · 24 h : %d", a.H1, a.H24)},
		"active_long": {Text: fmt.Sprintf("7 j : %d · 30 j : %d · 1 an : %d", a.D7, a.Month, a.Year)},
		"files":       {Text: fmt.Sprint(st.Files)},
		"free":        {Text: tile.Bytes(s.Free)},
		"users":       {Text: fmt.Sprint(st.Users)},
		"shares":      {Text: fmt.Sprint(sh.Total), Sub: fmt.Sprintf("liens %d (%d sans mot de passe) · utilisateurs %d · groupes %d · courriel %d · fédérés %d/%d", sh.Link, sh.LinkNoPass, sh.User, sh.Groups, sh.Mail, sh.FedSent, sh.FedRecv)},
		"version":     {Text: s.Version},
		"php":         {Text: srv.PHP.Version, Sub: srv.Webserver},
		"db":          {Text: tile.Bytes(srv.Database.Size), Sub: srv.Database.Type + " " + srv.Database.Version},
		"caches":      {Text: "local " + ncMemcache(s.Local) + " · distribué " + ncMemcache(s.Distributed), Sub: "verrous " + ncMemcache(s.Locking)},
		"storages":    {Text: fmt.Sprint(st.Storages)},
		"appdata":     {Text: tile.Bytes(st.Appdata), Sub: ncPlural(st.AppdataFile, "fichier", "fichiers")},
	}
	if st.Disabled > 0 {
		v["users"] = tile.Value{Text: fmt.Sprint(st.Users), Sub: fmt.Sprintf("dont %d désactivés", st.Disabled)}
	}
	if o := srv.PHP.Opcache; o.Enabled {
		tot := o.Mem.Used + o.Mem.Free + o.Mem.Wasted
		val := tile.Value{Text: "succès " + tile.Pct(o.Stats.HitRate), Sub: fmt.Sprintf("%s / %s, %d scripts", tile.Bytes(o.Mem.Used), tile.Bytes(tot), o.Stats.Scripts)}
		if tot > 0 {
			val.Pct = o.Mem.Used / tot * 100
			val.Level = tile.Level(val.Pct, 85, 95)
		}
		if o.Full {
			val.Level, val.Sub = "crit", "cache plein · "+val.Sub
		}
		v["opcache"] = val
	} else {
		v["opcache"] = tile.Value{Text: "désactivé", Level: "warn"}
	}
	if ap := srv.PHP.APCu; ap != nil {
		tot := ap.SMA.Segs * ap.SMA.Size
		val := tile.Value{Sub: fmt.Sprintf("%d entrées", ap.Cache.Entries)}
		if req := ap.Cache.Hits + ap.Cache.Misses; req > 0 {
			val.Text = "succès " + tile.Pct(ap.Cache.Hits/req*100)
		}
		if tot > 0 {
			val.Pct = (tot - ap.SMA.Avail) / tot * 100
			val.Sub = tile.Bytes(tot-ap.SMA.Avail) + " / " + tile.Bytes(tot) + ", " + val.Sub
		}
		v["apcu"] = val
	}
	if f := srv.FPM; f != nil {
		val := tile.Value{Text: fmt.Sprintf("actifs : %d / %d", f.Active, f.Total), Sub: fmt.Sprintf("%d connexions, file %d, lentes %d", f.Accepted, f.Queue, f.Slow), Level: "ok"}
		if f.MaxChildren > 0 {
			val.Level, val.Sub = "warn", fmt.Sprintf("max_children atteint %d fois · ", f.MaxChildren)+val.Sub
		}
		v["fpm"] = val
	}
	if len(s.Load) == 3 {
		val := tile.Value{Text: tile.Num(s.Load[0]), Sub: fmt.Sprintf("5 min %s · 15 min %s", tile.Num(s.Load[1]), tile.Num(s.Load[2]))}
		if s.CPUs > 0 {
			val.Sub += fmt.Sprintf(" · %d cœurs", s.CPUs)
			val.Level = tile.Level(s.Load[1]/float64(s.CPUs), 0.8, 1.5)
		}
		v["load"] = val
	}
	if s.MemTotal > 0 {
		used := (s.MemTotal - s.MemFree) * 1024
		v["ram"] = tile.Value{Pct: used / (s.MemTotal * 1024) * 100, Text: tile.Bytes(used) + " / " + tile.Bytes(s.MemTotal*1024)}
	}
	return v
}
