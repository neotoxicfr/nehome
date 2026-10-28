package integ

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Speedtest Tracker (alexjustesen, API v1) : jeton d'API (rôle token, droit
// de lecture des résultats), en-tête Bearer. download/upload sont en octets
// par seconde dans la base (download_bits en bits/s quand l'API le donne), le
// ping en ms. Un test n'a lieu que toutes les quelques heures : relevé toutes
// les 5 min.
type speedtest struct {
	url, token string
	hc         *http.Client
}

func newSpeedtest(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	s := &speedtest{url: strings.TrimRight(t.URL, "/") + "/api/v1", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if s.token, err = secret("token"); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *speedtest) Fields() []tile.Field {
	return []tile.Field{
		tile.F("download", "Débit descendant", tile.Stat, tile.OnTile),
		tile.F("upload", "Débit montant", tile.Stat, tile.OnTile),
		tile.F("ping", "Latence", tile.Stat, tile.OnTile),
		tile.F("when", "Dernier test", tile.Stat, tile.InDetail),
		tile.F("server", "Serveur de test", tile.Stat, tile.InDetail),
		tile.F("download_hist", "Descendant (derniers tests)", tile.Spark, tile.InDetail),
		tile.F("upload_hist", "Montant (derniers tests)", tile.Spark, tile.InDetail),
		tile.F("ping_hist", "Latence (derniers tests)", tile.Spark, tile.InDetail),
	}
}

func (s *speedtest) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, s.hc, s.url+path, out, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+s.token)
		r.Header.Set("Accept", "application/json")
	})
}

func (s *speedtest) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 5*time.Minute, func() {
		v, err := s.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

type speedResult struct {
	Ping         float64
	Download     float64 // octets/s
	Upload       float64
	DownloadBits float64 `json:"download_bits"`
	UploadBits   float64 `json:"upload_bits"`
	Status       string
	ServerName   string `json:"server_name"`
	CreatedAt    string `json:"created_at"` // « 2026-10-27 12:00:00 », sans fuseau
	Data         struct {
		Server struct{ Name, Location string }
		ISP    string
	}
}

// when lit created_at : heure locale du serveur (toDateTimeString), RFC 3339 en repli.
func (r speedResult) when() time.Time {
	if t, err := time.ParseInLocation(time.DateTime, r.CreatedAt, time.Local); err == nil {
		return t
	}
	t, _ := time.Parse(time.RFC3339, r.CreatedAt)
	return t
}

// bits renvoie le débit en bit/s, que l'API donne les bits ou les octets.
func speedBits(bits, bytes float64) float64 {
	if bits > 0 {
		return bits
	}
	return bytes * 8
}

func speedMbit(bps float64) string { return tile.Num(bps/1e6) + " Mbit/s" }

func (s *speedtest) poll(ctx context.Context) (tile.Values, error) {
	var last struct{ Data speedResult }
	if err := s.get(ctx, "/results/latest", &last); err != nil {
		return nil, err
	}
	r := last.Data
	v := tile.Values{
		"download": {Text: speedMbit(speedBits(r.DownloadBits, r.Download))},
		"upload":   {Text: speedMbit(speedBits(r.UploadBits, r.Upload))},
		"ping":     {Text: tile.Num(r.Ping) + " ms"},
		"when":     {Text: tile.Ago(r.when())},
	}
	name := r.ServerName
	if name == "" {
		name = r.Data.Server.Name
	}
	if name != "" {
		sub := r.Data.ISP
		if r.Data.Server.Location != "" {
			sub = strings.TrimSpace(r.Data.Server.Location + " " + sub)
		}
		v["server"] = tile.Value{Text: name, Sub: sub}
	}

	var hist struct{ Data []speedResult }
	if s.get(ctx, "/results?"+url.Values{"page[size]": {"30"}, "sort": {"-created_at"}}.Encode(), &hist) == nil && len(hist.Data) > 1 {
		rs := hist.Data
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].when().Before(rs[j].when()) })
		var d, u, p []float64
		for _, x := range rs {
			if x.Status != "" && x.Status != "completed" {
				continue
			}
			d = append(d, speedBits(x.DownloadBits, x.Download)/1e6)
			u = append(u, speedBits(x.UploadBits, x.Upload)/1e6)
			p = append(p, x.Ping)
		}
		if len(d) > 1 {
			v["download_hist"] = tile.Value{Points: d, Text: fmt.Sprintf("%d tests", len(d))}
			v["upload_hist"] = tile.Value{Points: u, Text: fmt.Sprintf("%d tests", len(u))}
			v["ping_hist"] = tile.Value{Points: p, Text: fmt.Sprintf("%d tests", len(p))}
		}
	}
	return v, nil
}
