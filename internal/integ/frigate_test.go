package integ

import (
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Forme d'après la doc Frigate (« GET /api/stats ») : cameras, detectors,
// detection_fps, gpu_usages, service.{uptime,version,storage en Mo}.
func TestFrigate(t *testing.T) {
	srv := f4Serve(t, nil, nil, map[string]string{
		"/api/stats": `{"cameras":{"jardin":{"camera_fps":5.0,"process_fps":5.0,"skipped_fps":0.0,"detection_fps":0.4},"garage":{"camera_fps":0.0,"process_fps":0.0,"skipped_fps":0.0,"detection_fps":0.0}},
"detectors":{"coral":{"inference_speed":8.5,"detection_start":0.0,"pid":99}},"detection_fps":0.4,"gpu_usages":{"nvidia":{"gpu":"23 %","mem":"10 %"}},
"service":{"uptime":90000,"version":"0.15.0-abc","storage":{"/media/frigate/recordings":{"total":1000000,"used":524288,"free":475712},"/tmp/cache":{"total":1000,"used":1,"free":999}}}}`,
	})
	defer srv.Close()
	src, _ := newFrigate(config.Tile{URL: srv.URL}, nil)
	v, err := src.(*frigate).poll(t.Context())
	if err != nil || v["cameras"].Text != "1 / 2 en ligne" || v["cameras"].Level != "crit" || v["inference"].Text != "8,5 ms" || v["version"].Text != "0.15.0-abc" || v["storage"].Sub != tile.Bytes(524288*(1<<20))+" / "+tile.Bytes(1000000*(1<<20)) {
		t.Fatalf("valeurs : %v %+v", err, v)
	}
	if g := v["gpu"].Bars; len(g) != 1 || g[0].Pct != 23 || v["camera_list"].Rows[0][0].Text != "garage" {
		t.Fatalf("gpu/caméras : %+v %+v", v["gpu"], v["camera_list"].Rows)
	}
}
