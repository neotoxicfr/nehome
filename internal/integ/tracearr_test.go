package integ

import (
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

func TestTracearr(t *testing.T) {
	srv := mediaServer(t, "Authorization", "Bearer factice", map[string]string{
		"/api/v1/public/streams": `{"data":[{"username":"Alice","mediaTitle":"The Dog Stars","mediaType":"movie","videoDecision":"directplay","audioDecision":"directplay","bitrate":5893,
"sourceVideoCodecDisplay":"HEVC","sourceAudioCodecDisplay":"EAC3","audioChannelsDisplay":"5.1","resolution":"4K","sourceVideoDetails":{"dynamicRange":"Dolby Vision"}},
{"username":"Bob","mediaTitle":"Limite et éveil","mediaType":"episode","showTitle":"Hunter x Hunter","seasonNumber":2,"episodeNumber":66,"videoDecision":"transcode","audioDecision":"transcode","bitrate":1893,"resolution":"480p"}],
"summary":{"total":3,"transcodes":1,"directStreams":0,"directPlays":2,"totalBitrate":"12.9 Mbps"}}`,
		"/api/v1/public/stats/today": `{"activeStreams":3,"todayPlays":20,"todaySessions":25,"watchTimeHours":9.8,"alertsLast24h":0,"activeUsersToday":10}`,
		"/api/v1/public/stats":       `{"activeStreams":3,"totalUsers":86,"totalSessions":2458,"recentViolations":2}`,
		"/api/v1/public/violations":  `{"data":[],"meta":{"total":7,"page":1,"pageSize":1}}`,
		"/api/v1/public/health":      `{"status":"ok","version":"v3.1.1","servers":[{"name":"Serveur","type":"plex","online":true,"activeStreams":3}]}`,
		"/api/v1/public/activity": `{"plays":[{"date":"2026-10-26 00:00:00","serverId":"s","count":101},{"date":"2026-10-25 00:00:00","serverId":"s","count":117},{"date":"2026-10-26 00:00:00","serverId":"t","count":4}],
"platforms":[{"platform":"Windows","count":577},{"platform":"Fire TV","count":273}],
"quality":{"directPlayPercent":81,"directStreamPercent":0,"transcodePercent":19,"total":2514}}`,
		"/api/v1/public/users": `{"data":[{"displayName":"ok","trustScore":100,"totalViolations":0},{"displayName":"louche","trustScore":60,"totalViolations":3,"lastActivityAt":"2026-10-20T10:00:00.000Z"},{"displayName":"un_peu","trustScore":90,"totalViolations":1,"lastActivityAt":null}]}`,
	})
	defer srv.Close()
	src, err := newTracearr(config.Tile{URL: srv.URL}, mediaSecret)
	if err != nil {
		t.Fatal(err)
	}
	a := src.(*tracearr)
	v, err := a.streams(t.Context())
	if err != nil || v["streams"].Text != "3" || !strings.HasSuffix(v["streams"].Sub, "12,9 Mbps") {
		t.Fatalf("flux : %v %+v", err, v["streams"])
	}
	r := v["details"].Rows
	if r[0][2].Text != "4K HEVC Dolby Vision" || r[0][3].Text != "EAC3 5.1" || r[1][1].Text != "Hunter x Hunter S02E66" || r[1][4].Level != "warn" {
		t.Fatalf("détails : %+v", r)
	}
	v, err = a.counters(t.Context())
	if err != nil || v["today"].Text != "20" || v["violations"].Text != "2" || v["violations"].Level != "crit" || v["violations"].Sub != "7 au total" || v["users"].Text != "86" || !strings.Contains(v["server"].Sub, "Tracearr v3.1.1") {
		t.Fatalf("compteurs : %v %+v", err, v)
	}
	v, err = a.month(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if p := v["plays"].Points; len(p) != 2 || p[0] != 117 || p[1] != 105 {
		t.Fatalf("courbe : %+v", p) // 25/10 puis 26/10 (101 + 4, deux serveurs additionnés)
	}
	if v["quality"].Bars[2].Text != "19 %" || v["platforms"].Rows[0][0].Text != "Windows" {
		t.Fatalf("qualité : %+v", v)
	}
	if tr := v["trust"].Rows; len(tr) != 2 || tr[0][0].Text != "louche" || tr[1][3].Text != "jamais" {
		t.Fatalf("confiance : %+v", tr)
	}
	bad, _ := newTracearr(config.Tile{URL: srv.URL}, func(string) (string, error) { return "mauvais", nil })
	if _, err := bad.(*tracearr).streams(t.Context()); err == nil || strings.Contains(err.Error(), "mauvais") {
		t.Fatalf("erreur attendue : %v", err)
	}
}
