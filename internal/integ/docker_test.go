package integ

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses relevées sur le proxy de socket (champs inutiles retirés).
const dkList = `[
{"Id":"aaaaaaaaaaaa1111","Names":["/nehome"],"Image":"nehome:local","State":"running","Status":"Up 8 minutes (healthy)"},
{"Id":"bbbbbbbbbbbb2222","Names":["/cleanuparr"],"Image":"cleanuparr:latest","State":"running","Status":"Up 7 hours"},
{"Id":"cccccccccccc3333","Names":["/claude-tools"],"Image":"claude-tools","State":"exited","Status":"Exited (0) 2 hours ago"},
{"Id":"dddddddddddd4444","Names":["/jackett"],"Image":"jackett","State":"running","Status":"Up 1 hour (unhealthy)"},
{"Id":"eeeeeeeeeeee5555","Names":["/neuf"],"Image":"neuf","State":"created","Status":"Created"}]`

func dkInspectJSON(id string) string {
	switch id {
	case "aaaaaaaaaaaa1111":
		return `{"RestartCount":0,"Config":{"Image":"nehome:local"},"State":{"Status":"running","Running":true,"StartedAt":"2026-10-27T14:14:34.603728927Z","FinishedAt":"0001-01-01T00:00:00Z","Health":{"Status":"healthy"}}}`
	case "bbbbbbbbbbbb2222":
		return `{"RestartCount":5,"Config":{"Image":"cleanuparr:latest"},"State":{"Status":"running","Running":true,"StartedAt":"2026-10-27T08:00:00Z","FinishedAt":"0001-01-01T00:00:00Z"}}`
	case "cccccccccccc3333":
		return `{"RestartCount":0,"Config":{"Image":"claude-tools"},"State":{"Status":"exited","ExitCode":0,"StartedAt":"2026-10-27T12:00:00Z","FinishedAt":"2026-10-27T12:05:00Z"}}`
	case "dddddddddddd4444":
		return `{"RestartCount":0,"Config":{"Image":"jackett"},"State":{"Status":"running","Running":true,"StartedAt":"2026-10-27T13:00:00Z","FinishedAt":"0001-01-01T00:00:00Z","Health":{"Status":"unhealthy"}}}`
	}
	return `{"RestartCount":0,"Config":{"Image":"neuf"},"State":{"Status":"created","StartedAt":"0001-01-01T00:00:00Z","FinishedAt":"0001-01-01T00:00:00Z"}}`
}

func dkFake(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("méthode %s interdite", r.Method)
		}
		p := r.URL.Path
		switch {
		case p == "/containers/json":
			w.Write([]byte(dkList))
		case p == "/info":
			w.Write([]byte(`{"MemTotal":64000000000}`))
		case p == "/images/json":
			w.Write([]byte(`[{"Id":"a"},{"Id":"b"},{"Id":"c"}]`))
		case strings.HasSuffix(p, "/stats"):
			// Deux échantillons à ~1 s : 0,2 cœur sur 4, 10 Mo hors cache, limite 128 Mo.
			w.Write([]byte(`{"cpu_stats":{"cpu_usage":{"total_usage":1200000000},"system_cpu_usage":4000000000,"online_cpus":4},"precpu_stats":{"cpu_usage":{"total_usage":1000000000},"system_cpu_usage":0},"memory_stats":{"usage":17891328,"stats":{"inactive_file":7891328},"limit":134217728}}`))
		case p == "/containers/absent/json":
			http.NotFound(w, r)
		case strings.HasPrefix(p, "/containers/") && strings.HasSuffix(p, "/json"):
			id := strings.Split(p, "/")[2]
			if id == "nehome" { // l'API accepte le nom comme l'identifiant
				id = "aaaaaaaaaaaa1111"
			}
			w.Write([]byte(dkInspectJSON(id)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDockerSnapshot(t *testing.T) {
	srv := dkFake(t)
	src, _ := newDocker(config.Tile{URL: srv.URL}, nil)
	v, err := src.(*docker).snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v["running"].Text != "3 / 5" || v["healthy"].Text != "1" || v["unhealthy"].Text != "1" || v["stopped"].Text != "1" || v["created"].Text != "1" || v["images"].Text != "3" {
		t.Fatalf("compteurs : %+v", v)
	}
	// Created en tête, puis unhealthy, puis arrêté proprement, puis redémarrages.
	rows := v["problems"].Rows
	got := []string{}
	for _, r := range rows {
		got = append(got, r[0].Text)
	}
	if strings.Join(got, ",") != "neuf,jackett,claude-tools,cleanuparr" || v["trouble"].Level != "crit" || v["trouble"].Text != "4" {
		t.Fatalf("problèmes : %v %+v", got, v["trouble"])
	}
	if rows[0][1].Text != "Created (jamais démarré)" || rows[0][1].Level != "crit" || rows[3][3].Text != "5" {
		t.Fatalf("cellules : %+v", rows)
	}
	cpu := v["cpu"].Rows
	if len(cpu) != 3 || cpu[0][1].Text != "20 %" { // 2e8/4e9*4*100
		t.Fatalf("cpu : %+v", cpu)
	}
	if ram := v["ram"].Rows; len(ram) != 3 || ram[0][1].Text != "10 Mo" || ram[0][2].Text != "134 Mo" {
		t.Fatalf("ram : %+v", ram)
	}
}

func TestDockerPanne(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	src, _ := newDocker(config.Tile{URL: srv.URL}, nil)
	if _, err := src.(*docker).snapshot(t.Context()); err == nil {
		t.Fatal("la liste en échec doit remonter")
	}
}

// Extrait d'un vrai update.log (27/10/2026) : lignes datées, progression compose
// sans date, conteneurs recréés, puis le résumé.
const updLog = `[2026-10-27 02:30:01] === DEBUT MISE A JOUR ===
[2026-10-27 02:30:10] --- system ---
 ntfy Pulling 
 ntfy Pulled 
 Container autoheal  Recreate
 Container autoheal  Recreated
 Container uptime  Recreated
 Container autoheal  Started
[2026-10-27 02:31:20] nextcloud: rebuild local images
[2026-10-27 02:32:23] === BILAN SANTE ===
[2026-10-27 02:32:23] Tous les conteneurs sont Up, aucun unhealthy ni en boucle
[2026-10-27 02:32:32] hub crowdsec mis a jour, crowdsec redemarre
[2026-10-27 02:32:32] Stacks traites : 17
[2026-10-27 02:32:32] Pull OK : 17  |  Up OK : 17
[2026-10-27 02:32:32] Tous les stacks OK
[2026-10-27 02:32:32] === FIN ===
`

func TestUpdateLog(t *testing.T) {
	f := filepath.Join(t.TempDir(), "update.log")
	os.WriteFile(f, []byte(updLog), 0o600)
	d := &docker{updateLog: f}
	v := d.updateValues()
	u := v["update"]
	if u.Text != "27/10 à 02:30" || u.Level != "ok" || !strings.Contains(u.Sub, "2 conteneur") {
		t.Fatalf("résumé : %+v", u)
	}
	rows := v["update_detail"].Rows
	if rows[1][1].Text != "2 min" || rows[2][1].Text != "17" || rows[3][1].Text != "autoheal, uptime" {
		t.Fatalf("détail : %+v", rows)
	}
	// Échec de pull et nuit interrompue.
	os.WriteFile(f, []byte(strings.Replace(strings.Replace(updLog, "[2026-10-27 02:32:32] === FIN ===\n", "", 1), "Tous les stacks OK", "Pull failed  (1): starr", 1)), 0o600)
	u = d.updateValues()["update"]
	if u.Level != "crit" || !strings.Contains(u.Sub, "1 erreur") {
		t.Fatalf("erreur : %+v", u)
	}
	// Le bilan « aucun unhealthy » ne compte pas comme erreur.
	if s := parseUpdate(updLog); len(s.errs) != 0 || !s.done || s.end.Sub(s.start) != 2*time.Minute+31*time.Second {
		t.Fatalf("parse : %+v", s)
	}
	d.updateLog = filepath.Join(t.TempDir(), "absent")
	if d.updateValues()["update"].Level != "warn" {
		t.Fatal("journal absent : warn attendu")
	}
}
