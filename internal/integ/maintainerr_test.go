package integ

import (
	"strings"
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

func TestMaintainerr(t *testing.T) {
	soon := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z") // + 3 j = dans ~1 j
	late := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	srv := mediaServer(t, "", "", map[string]string{
		"/api/collections": `[{"id":1,"title":"_Films bientôt supprimés","type":"movie","isActive":true,"deleteAfterDays":3,"mediaCount":12,"totalSizeBytes":69745847911,"handledMediaAmount":1766,"handledMediaSizeBytes":2352070574473,"media":[]},
{"id":4,"title":"_Saisons séries","type":"season","isActive":true,"deleteAfterDays":3,"mediaCount":5,"totalSizeBytes":1000000000,"handledMediaAmount":10,"handledMediaSizeBytes":0},
{"id":9,"title":"vide","type":"movie","isActive":true,"deleteAfterDays":3,"mediaCount":0}]`,
		"/api/collections/media/1/content/1": `{"totalSize":12,"items":[{"addDate":"` + late + `","sizeBytes":5156007282,"mediaData":{"title":"Film A","type":"movie"}}]}`,
		"/api/collections/media/4/content/1": `{"totalSize":5,"items":[{"addDate":"` + soon + `","sizeBytes":12719937206,"mediaData":{"title":"Season 2","parentTitle":"Série B","type":"season"}}]}`,
		"/api/app/status":                    `{"status":1,"version":"3.31.0","updateAvailable":false}`,
	})
	defer srv.Close()
	src, err := newMaintainerr(config.Tile{URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.(*maintainerr).poll(t.Context())
	if err != nil || v["items"].Text != "17" || v["size"].Text != "70,7 Go" || v["version"].Text != "3.31.0" || v["freed"].Text != "2,4 To" {
		t.Fatalf("compteurs : %v %+v", err, v)
	}
	if v["next"].Text != "Série B – Season 2" || !strings.HasPrefix(v["next"].Sub, "dans ") {
		t.Fatalf("prochain : %+v", v["next"])
	}
	if u := v["upcoming"].Rows; len(u) != 2 || u[1][0].Text != "Film A" || len(v["collections"].Rows) != 3 {
		t.Fatalf("à venir : %+v", u)
	}
}
