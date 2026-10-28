package integ

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Formes d'après l'OpenAPI d'Immich (ServerStatsResponseDto,
// ServerStorageResponseDto, ServerVersionResponseDto, AllJobStatusResponseDto).
func TestImmich(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/server/statistics":
			w.Write([]byte(`{"photos":12000,"videos":300,"usage":250000000000,"usageByUser":[{"userId":"u1","userName":"neo","photos":12000,"videos":300,"usage":250000000000,"quotaSizeInBytes":500000000000},{"userId":"u2","userName":"invité","photos":0,"videos":0,"usage":0,"quotaSizeInBytes":null}]}`))
		case "/api/server/storage":
			w.Write([]byte(`{"diskSize":"1 To","diskSizeRaw":1000000000000,"diskUseRaw":900000000000,"diskAvailableRaw":100000000000,"diskUsagePercentage":90}`))
		case "/api/server/version":
			w.Write([]byte(`{"major":1,"minor":132,"patch":3}`))
		case "/api/server/version-check":
			w.Write([]byte(`{"checkedAt":"2026-01-01T00:00:00Z","releaseVersion":"v1.133.0"}`))
		case "/api/queues":
			w.Write([]byte(`[{"name":"thumbnailGeneration","isPaused":false,"statistics":{"active":2,"waiting":40,"failed":1,"delayed":0,"paused":0}},{"name":"library","isPaused":false,"statistics":{"active":0,"waiting":0,"failed":0}}]`))
		default:
			t.Errorf("chemin inattendu %s", r.URL)
		}
	}))
	defer srv.Close()
	src, err := newImmich(config.Tile{URL: srv.URL}, fakeSecret)
	if err != nil {
		t.Fatal(err)
	}
	v := collect(t, src, func(v tile.Values) bool {
		return v["jobs"].Text != "" && v["version"].Text != "" && v["disk"].Text != ""
	})
	if v["photos"].Text != "12000" || v["usage"].Text != "250 Go" {
		t.Errorf("stats = %v %v", v["photos"], v["usage"])
	}
	if d := v["disk"]; d.Pct != 90 || d.Level == "ok" || d.Level == "" {
		t.Errorf("disk = %+v", d)
	}
	if x := v["version"]; x.Text != "v1.132.3" || x.Sub != "v1.133.0 disponible" || x.Level != "warn" {
		t.Errorf("version = %+v", x)
	}
	if u := v["users"].Rows; len(u) != 2 || u[0][4].Text != "50 % de 500 Go" || u[1][4].Text != "illimité" {
		t.Errorf("users = %+v", u)
	}
	if j := v["jobs"]; j.Text != "2" || j.Level != "warn" || j.Sub != "40 en attente · 1 en échec" {
		t.Errorf("jobs = %+v", j)
	}
	if r := v["jobs_table"].Rows; len(r) != 1 || r[0][0].Text != "thumbnailGeneration" {
		t.Errorf("jobs_table = %+v", r)
	}
}
