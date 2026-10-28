package integ

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Paperless-ngx : /api/statistics/, /api/tasks/ et /api/status/. Jeton d'API
// (rôle token, profil de l'utilisateur), en-tête « Authorization: Token … ».
// /api/status/ est réservé aux administrateurs : sans lui, version et stockage
// restent masqués. Relevé toutes les minutes.
type paperless struct {
	url, token string
	hc         *http.Client
}

func newPaperless(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	p := &paperless{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}
	var err error
	if p.token, err = secret("token"); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *paperless) Fields() []tile.Field {
	return []tile.Field{
		tile.F("documents", "Documents", tile.Stat, tile.OnTile),
		tile.F("inbox", "Boîte de réception", tile.Stat, tile.OnTile),
		tile.F("tasks", "Tâches", tile.Stat, tile.OnTile),
		tile.F("storage", "Stockage", tile.Meter, tile.InDetail),
		tile.F("classes", "Étiquettes, correspondants, types", tile.Stat, tile.InDetail),
		tile.F("chars", "Caractères indexés", tile.Stat, tile.InDetail),
		tile.F("types", "Types de fichiers", tile.Table, tile.InDetail),
		tile.F("task_list", "Dernières tâches", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (p *paperless) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, p.hc, p.url+path, out, func(r *http.Request) {
		r.Header.Set("Authorization", "Token "+p.token)
		r.Header.Set("Accept", "application/json")
	})
}

func (p *paperless) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, time.Minute, func() {
		v, err := p.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

type paperlessTask struct {
	FileName string    `json:"task_file_name"`
	Status   string    `json:"status"`
	Created  time.Time `json:"date_created"`
	Result   string    `json:"result"`
}

func (p *paperless) poll(ctx context.Context) (tile.Values, error) {
	var st struct {
		Total      int   `json:"documents_total"`
		Inbox      int   `json:"documents_inbox"`
		Chars      int64 `json:"character_count"`
		Tags       int   `json:"tag_count"`
		Corr       int   `json:"correspondent_count"`
		DocTypes   int   `json:"document_type_count"`
		FileCounts []struct {
			Mime  string `json:"mime_type"`
			Count int    `json:"mime_type_count"`
		} `json:"document_file_type_counts"`
	}
	if err := p.get(ctx, "/statistics/", &st); err != nil {
		return nil, err
	}
	inbox := tile.Value{Text: fmt.Sprint(st.Inbox)}
	if st.Inbox > 0 {
		inbox.Level = "warn"
		inbox.Sub = "à classer"
	}
	v := tile.Values{
		"documents": {Text: fmt.Sprint(st.Total)},
		"inbox":     inbox,
		"classes":   {Text: fmt.Sprintf("%d / %d / %d", st.Tags, st.Corr, st.DocTypes)},
		"chars":     {Text: tile.Num(float64(st.Chars))},
	}
	if len(st.FileCounts) > 0 {
		var rows [][]tile.Cell
		for _, f := range st.FileCounts {
			rows = append(rows, []tile.Cell{{Text: f.Mime}, {Text: fmt.Sprint(f.Count)}})
		}
		v["types"] = tile.Value{Head: []string{"Type", "Documents"}, Rows: rows}
	}

	// Selon la version, /tasks/ renvoie un tableau ou une page {results}.
	var raw json.RawMessage
	if p.get(ctx, "/tasks/", &raw) == nil {
		var tasks []paperlessTask
		if json.Unmarshal(raw, &tasks) != nil {
			var pg struct{ Results []paperlessTask }
			json.Unmarshal(raw, &pg)
			tasks = pg.Results
		}
		sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Created.After(tasks[j].Created) })
		var run, fail int
		var rows [][]tile.Cell
		for _, t := range tasks {
			switch t.Status {
			case "PENDING", "STARTED", "RETRY":
				run++
			case "FAILURE":
				if time.Since(t.Created) < 24*time.Hour {
					fail++
				}
			}
			if len(rows) < 15 {
				lvl := ""
				switch t.Status {
				case "FAILURE":
					lvl = "crit"
				case "SUCCESS":
					lvl = "ok"
				}
				rows = append(rows, []tile.Cell{{Text: t.FileName}, {Text: paperlessStatus(t.Status), Level: lvl}, {Text: tile.Ago(t.Created)}})
			}
		}
		tv := tile.Value{Text: "Aucune en cours"}
		if run > 0 {
			tv.Text = fmt.Sprintf("%d en cours", run)
		}
		if fail > 0 {
			tv.Sub = fmt.Sprintf("%d échec(s) sur 24 h", fail)
			tv.Level = "crit"
		}
		v["tasks"] = tv
		v["task_list"] = tile.Value{Head: []string{"Fichier", "État", "Lancée"}, Rows: rows}
	}

	var ss struct {
		Version string `json:"pngx_version"`
		Storage struct{ Total, Available float64 }
	}
	if p.get(ctx, "/status/", &ss) == nil {
		if ss.Version != "" {
			v["version"] = tile.Value{Text: ss.Version}
		}
		if ss.Storage.Total > 0 {
			used := ss.Storage.Total - ss.Storage.Available
			pct := used / ss.Storage.Total * 100
			v["storage"] = tile.Value{Pct: pct, Text: tile.Pct(pct), Sub: tile.Bytes(used) + " / " + tile.Bytes(ss.Storage.Total), Level: tile.Level(pct, 80, 90)}
		}
	}
	return v, nil
}

func paperlessStatus(s string) string {
	switch s {
	case "SUCCESS":
		return "Réussie"
	case "FAILURE":
		return "Échec"
	case "STARTED":
		return "En cours"
	case "PENDING":
		return "En attente"
	case "RETRY":
		return "Nouvel essai"
	case "REVOKED":
		return "Annulée"
	}
	return s
}
