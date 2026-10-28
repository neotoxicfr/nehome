package integ

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponse réelle d'Open-Meteo (27/10/2026), champs inutiles retirés.
const omJSON = `{"utc_offset_seconds":3600,"current":{"time":"2026-10-27T15:15","temperature_2m":19.4,"apparent_temperature":20.0,"relative_humidity_2m":71,"weather_code":1,"wind_speed_10m":4.8,"wind_gusts_10m":11.2,"precipitation":0.00},
"daily":{"time":["2026-10-27","2026-10-28","2026-10-29"],"weather_code":[45,3,63],"temperature_2m_max":[19.4,21.3,18.5],"temperature_2m_min":[10.2,9.7,7.0],"precipitation_sum":[0.00,0.00,4.2],"precipitation_probability_max":[0,0,80],"sunrise":["2026-10-27T07:13","2026-10-28T07:14","2026-10-29T07:16"],"sunset":["2026-10-27T17:42","2026-10-28T17:41","2026-10-29T17:39"]}}`

func TestWeather(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v1/forecast" || q.Get("latitude") != "48.85" || q.Get("timezone") != "Europe/Paris" {
			http.Error(w, "mauvaise requête", 400)
			return
		}
		w.Write([]byte(omJSON))
	}))
	defer srv.Close()
	src, err := newWeather(config.Tile{Options: map[string]string{"lat": "48.85", "lon": "2.35", "timezone": "Europe/Paris"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := src.(*weather)
	w.base = srv.URL
	if w.clock() != nil {
		t.Fatal("pas d'heure avant la première réponse")
	}
	v, err := w.fetch(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v["temp"].Text != "19,4 °C" || v["temp"].Sub != "Plutôt dégagé · ressenti 20 °C" || v["soleil"].Text != "07:13 / 17:42" || v["pluie"].Text != "0 mm" || v["humidite"].Pct != 71 || v["heure"].Text == "" {
		t.Fatalf("valeurs : %+v", v)
	}
	r := v["previsions"].Rows
	if len(r) != 3 || r[0][0].Text != "Aujourd'hui" || r[2][0].Text != "jeudi 29" || r[2][1].Text != "Pluie" || r[2][2].Text != "7 / 18,5 °C" || r[2][3].Text != "4,2 mm · 80 %" {
		t.Fatalf("prévisions : %+v", r)
	}
	if _, err := newWeather(config.Tile{}, nil); err == nil {
		t.Fatal("lat/lon requis")
	}
}
