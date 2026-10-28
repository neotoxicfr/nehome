package integ

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Météo Open-Meteo (sans clé). Prévisions toutes les 15 min ; l'heure locale
// du fuseau est recalculée chaque minute à partir du décalage UTC renvoyé par
// l'API (pas de base de fuseaux nécessaire dans l'image).
type weather struct {
	base, lat, lon, tz string
	hc                 *http.Client

	mu  sync.Mutex
	loc *time.Location
}

func newWeather(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	w := &weather{base: "https://api.open-meteo.com", lat: t.Options["lat"], lon: t.Options["lon"], tz: t.Options["timezone"], hc: httpClient(false, false)}
	for _, c := range []string{w.lat, w.lon} {
		if _, err := strconv.ParseFloat(c, 64); err != nil {
			return nil, fmt.Errorf("options.lat et options.lon (degrés décimaux) requis")
		}
	}
	if w.tz == "" {
		w.tz = "auto"
	}
	return w, nil
}

func (w *weather) Fields() []tile.Field {
	return []tile.Field{
		tile.F("temp", "Température", tile.Stat, tile.OnTile),
		tile.F("heure", "Heure locale", tile.Stat, tile.OnTile),
		tile.F("soleil", "Lever / coucher du soleil", tile.Stat, tile.OnTile),
		tile.F("pluie", "Pluie aujourd'hui", tile.Stat, tile.OnTile),
		tile.F("vent", "Vent", tile.Stat, tile.InDetail),
		tile.F("humidite", "Humidité", tile.Meter, tile.InDetail),
		tile.F("previsions", "Prévisions sur 3 jours", tile.Table, tile.InDetail),
	}
}

func (w *weather) Run(ctx context.Context, emit func(tile.Values, error)) {
	e := split(emit, 2)
	go every(ctx, time.Minute, func() {
		if v := w.clock(); v != nil && ctx.Err() == nil {
			e[0](v, nil)
		}
	})
	every(ctx, 15*time.Minute, func() {
		v, err := w.fetch(ctx)
		if ctx.Err() == nil {
			e[1](v, err)
		}
	})
}

func (w *weather) fetch(ctx context.Context) (tile.Values, error) {
	q := url.Values{
		"latitude": {w.lat}, "longitude": {w.lon}, "timezone": {w.tz}, "forecast_days": {"3"}, "wind_speed_unit": {"kmh"},
		"current": {"temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m,wind_gusts_10m,precipitation"},
		"daily":   {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,precipitation_probability_max,sunrise,sunset"},
	}
	var raw struct {
		UTCOffset int `json:"utc_offset_seconds"`
		Current   struct {
			Temp     float64 `json:"temperature_2m"`
			Feels    float64 `json:"apparent_temperature"`
			Humidity float64 `json:"relative_humidity_2m"`
			Code     int     `json:"weather_code"`
			Wind     float64 `json:"wind_speed_10m"`
			Gusts    float64 `json:"wind_gusts_10m"`
			Precip   float64 `json:"precipitation"`
		} `json:"current"`
		Daily struct {
			Time    []string  `json:"time"`
			Code    []int     `json:"weather_code"`
			Max     []float64 `json:"temperature_2m_max"`
			Min     []float64 `json:"temperature_2m_min"`
			Rain    []float64 `json:"precipitation_sum"`
			Prob    []float64 `json:"precipitation_probability_max"`
			Sunrise []string  `json:"sunrise"`
			Sunset  []string  `json:"sunset"`
		} `json:"daily"`
	}
	if err := getJSON(ctx, w.hc, w.base+"/v1/forecast?"+q.Encode(), &raw, nil); err != nil {
		return nil, err
	}
	d, c := raw.Daily, raw.Current
	if len(d.Time) == 0 || len(d.Max) != len(d.Time) || len(d.Min) != len(d.Time) || len(d.Code) != len(d.Time) {
		return nil, fmt.Errorf("réponse Open-Meteo incomplète")
	}
	w.mu.Lock()
	w.loc = time.FixedZone("local", raw.UTCOffset)
	w.mu.Unlock()

	v := w.clock()
	cond := wmo(c.Code)
	v["temp"] = tile.Value{Text: tile.Num(c.Temp) + " °C", Sub: cond + " · ressenti " + tile.Num(c.Feels) + " °C"}
	v["vent"] = tile.Value{Text: tile.Num(c.Wind) + " km/h", Sub: "rafales " + tile.Num(c.Gusts) + " km/h"}
	v["humidite"] = tile.Value{Text: tile.Pct(c.Humidity), Pct: c.Humidity}
	if len(d.Sunrise) > 0 && len(d.Sunset) > 0 {
		v["soleil"] = tile.Value{Text: hhmm(d.Sunrise[0]) + " / " + hhmm(d.Sunset[0])}
	}
	if len(d.Rain) > 0 && len(d.Prob) > 0 {
		v["pluie"] = tile.Value{Text: tile.Num(d.Rain[0]) + " mm", Sub: "probabilité " + tile.Pct(d.Prob[0])}
	}
	var rows [][]tile.Cell
	for i, day := range d.Time {
		rain := ""
		if i < len(d.Rain) && i < len(d.Prob) {
			rain = tile.Num(d.Rain[i]) + " mm · " + tile.Pct(d.Prob[i])
		}
		rows = append(rows, []tile.Cell{{Text: dayName(day, i)}, {Text: wmo(d.Code[i])}, {Text: tile.Num(d.Min[i]) + " / " + tile.Num(d.Max[i]) + " °C"}, {Text: rain}})
	}
	v["previsions"] = tile.Value{Head: []string{"Jour", "Temps", "Min / max", "Pluie"}, Rows: rows}
	return v, nil
}

var (
	jours = [...]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
	mois  = [...]string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
)

// clock : l'heure du fuseau de la tuile, nil tant que l'API n'a pas répondu.
func (w *weather) clock() tile.Values {
	w.mu.Lock()
	loc := w.loc
	w.mu.Unlock()
	if loc == nil {
		return nil
	}
	n := time.Now().In(loc)
	return tile.Values{"heure": {Text: n.Format("15:04"), Sub: fmt.Sprintf("%s %d %s", jours[n.Weekday()], n.Day(), mois[n.Month()-1])}}
}

func hhmm(iso string) string {
	if len(iso) >= 16 {
		return iso[11:16]
	}
	return iso
}

func dayName(iso string, i int) string {
	switch i {
	case 0:
		return "Aujourd'hui"
	case 1:
		return "Demain"
	}
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%s %d", jours[t.Weekday()], t.Day())
}

// wmo traduit un code météo WMO.
func wmo(c int) string {
	switch c {
	case 0:
		return "Ciel dégagé"
	case 1:
		return "Plutôt dégagé"
	case 2:
		return "Partiellement nuageux"
	case 3:
		return "Couvert"
	case 45, 48:
		return "Brouillard"
	case 51, 53, 55:
		return "Bruine"
	case 56, 57:
		return "Bruine verglaçante"
	case 61:
		return "Pluie faible"
	case 63:
		return "Pluie"
	case 65:
		return "Forte pluie"
	case 66, 67:
		return "Pluie verglaçante"
	case 71, 73, 75, 77:
		return "Neige"
	case 80, 81:
		return "Averses"
	case 82:
		return "Fortes averses"
	case 85, 86:
		return "Averses de neige"
	case 95:
		return "Orage"
	case 96, 99:
		return "Orage et grêle"
	}
	return "Code météo " + strconv.Itoa(c)
}
