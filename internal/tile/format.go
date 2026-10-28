package tile

import (
	"fmt"
	"strings"
	"time"
)

// Num : nombre à la française (virgule décimale), un chiffre après la virgule
// sous 100, aucun au-delà.
func Num(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	if f >= 100 || f <= -100 {
		s = fmt.Sprintf("%.0f", f)
	}
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1)
}

var units = []string{"o", "ko", "Mo", "Go", "To", "Po"}

// Bytes : taille en unités décimales (comme qBittorrent et les débits réseau).
func Bytes(n float64) string {
	i := 0
	for n >= 1000 && i < len(units)-1 {
		n /= 1000
		i++
	}
	return Num(n) + " " + units[i]
}

func Rate(bytesPerSec float64) string { return Bytes(bytesPerSec) + "/s" }

func Pct(f float64) string { return Num(f) + " %" }

// Dur : « 26 j 23 h », « 3 h 12 min », « 45 s ».
func Dur(d time.Duration) string {
	d = d.Round(time.Second)
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	m := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d j %d h", days, h)
	case h > 0:
		return fmt.Sprintf("%d h %d min", h, m)
	case m > 0:
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d s", int(d.Seconds()))
}

// Ago : « il y a 3 h 12 min » ; vide pour l'heure zéro.
func Ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return "il y a " + Dur(time.Since(t))
}

// Level renvoie warn/crit au-delà des seuils, ok sinon.
func Level(v, warn, crit float64) string {
	switch {
	case v >= crit:
		return "crit"
	case v >= warn:
		return "warn"
	}
	return "ok"
}

// Ring garde les n dernières valeurs d'une courbe.
type Ring struct {
	n   int
	buf []float64
}

func NewRing(n int) *Ring { return &Ring{n: n} }

func (r *Ring) Push(v float64) []float64 {
	r.buf = append(r.buf, v)
	if len(r.buf) > r.n {
		r.buf = r.buf[len(r.buf)-r.n:]
	}
	return append([]float64(nil), r.buf...)
}
