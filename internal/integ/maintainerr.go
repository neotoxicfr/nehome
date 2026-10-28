package integ

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// Maintainerr (API sans authentification ; seule l'action écrit). Les règles tournent
// une fois par jour : relevé toutes les 5 min. Date de suppression d'un
// élément = date d'entrée dans la collection + deleteAfterDays.
type maintainerr struct {
	url string
	hc  *http.Client
}

func newMaintainerr(t config.Tile, _ func(string) (string, error)) (tile.Source, error) {
	return &maintainerr{url: strings.TrimRight(t.URL, "/") + "/api", hc: httpClient(t.InsecureTLS, false)}, nil
}

func (m *maintainerr) Fields() []tile.Field {
	return []tile.Field{
		tile.F("items", "Éléments à supprimer", tile.Stat, tile.OnTile),
		tile.F("size", "Espace à libérer", tile.Stat, tile.OnTile),
		tile.F("next", "Prochaine suppression", tile.Stat, tile.OnTile),
		tile.F("freed", "Déjà supprimé", tile.Stat, tile.InDetail),
		tile.F("collections", "Collections", tile.Table, tile.InDetail),
		tile.F("upcoming", "Prochains éléments supprimés", tile.Table, tile.InDetail),
		tile.F("version", "Version", tile.Stat, tile.InDetail),
	}
}

func (m *maintainerr) get(ctx context.Context, path string, out any) error {
	return getJSON(ctx, m.hc, m.url+path, out, nil)
}

func (m *maintainerr) Run(ctx context.Context, emit func(tile.Values, error)) {
	every(ctx, 5*time.Minute, func() {
		v, err := m.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

type maintainerrItem struct {
	AddDate   time.Time
	SizeBytes float64
	MediaData struct {
		Title, ParentTitle string
		Type               string
	}
}

func (m *maintainerr) poll(ctx context.Context) (tile.Values, error) {
	var cols []struct {
		ID                    int
		Title, Type           string
		IsActive              bool
		DeleteAfterDays       int
		MediaCount            int
		TotalSizeBytes        float64
		HandledMediaAmount    int
		HandledMediaSizeBytes float64
		LastDurationInSeconds float64
	}
	if err := m.get(ctx, "/collections", &cols); err != nil {
		return nil, err
	}
	var items, handled int
	var size, freed float64
	var rows [][]tile.Cell
	type up struct {
		name, col string
		at        time.Time
		size      float64
	}
	var ups []up
	for _, c := range cols {
		items += c.MediaCount
		size += c.TotalSizeBytes
		handled += c.HandledMediaAmount
		freed += c.HandledMediaSizeBytes
		state := fmt.Sprintf("%d j", c.DeleteAfterDays)
		if !c.IsActive {
			state = "inactive"
		}
		rows = append(rows, []tile.Cell{{Text: c.Title}, {Text: c.Type}, {Text: fmt.Sprint(c.MediaCount)}, {Text: state}, {Text: tile.Bytes(c.TotalSizeBytes)}, {Text: fmt.Sprint(c.HandledMediaAmount)}})
		if !c.IsActive || c.MediaCount == 0 {
			continue
		}
		var page struct{ Items []maintainerrItem }
		if m.get(ctx, fmt.Sprintf("/collections/media/%d/content/1?size=50", c.ID), &page) != nil {
			continue // les éléments d'une collection manquent, le reste s'affiche
		}
		for _, it := range page.Items {
			name := it.MediaData.Title
			if it.MediaData.Type == "season" && it.MediaData.ParentTitle != "" {
				name = it.MediaData.ParentTitle + " – " + name
			}
			ups = append(ups, up{name, c.Title, it.AddDate.Add(time.Duration(c.DeleteAfterDays) * 24 * time.Hour), it.SizeBytes})
		}
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].at.Before(ups[j].at) })
	left := func(t time.Time) string {
		if d := time.Until(t); d > 0 {
			return "dans " + tile.Dur(d)
		}
		return "au prochain passage"
	}
	v := tile.Values{
		"items":       {Text: fmt.Sprint(items), Sub: fmt.Sprintf("dans %d collection(s)", len(cols))},
		"size":        {Text: tile.Bytes(size)},
		"freed":       {Text: tile.Bytes(freed), Sub: fmt.Sprintf("%d éléments traités", handled)},
		"collections": {Head: []string{"Collection", "Type", "Éléments", "Délai", "Taille", "Traités"}, Rows: rows},
	}
	if len(ups) > 0 {
		v["next"] = tile.Value{Text: ups[0].name, Sub: left(ups[0].at)}
	} else {
		v["next"] = tile.Value{Text: "aucune"}
	}
	var urows [][]tile.Cell
	for i, u := range ups {
		if i == 15 {
			break
		}
		urows = append(urows, []tile.Cell{{Text: u.name}, {Text: u.col}, {Text: left(u.at)}, {Text: tile.Bytes(u.size)}})
	}
	v["upcoming"] = tile.Value{Head: []string{"Titre", "Collection", "Suppression", "Taille"}, Rows: urows}

	var st struct {
		Version         string
		UpdateAvailable bool
	}
	if m.get(ctx, "/app/status", &st) == nil {
		ver := tile.Value{Text: st.Version}
		if st.UpdateAvailable {
			ver.Sub, ver.Level = "mise à jour disponible", "warn"
		}
		v["version"] = ver
	}
	return v, nil
}

func (m *maintainerr) Actions() []tile.Action {
	return []tile.Action{{ID: "rules", Label: "Exécuter les règles"}}
}

// Do : POST /api/rules/execute (RulesController de Maintainerr) met en file
// tous les groupes de règles actifs ; 409 si déjà en cours ou rien d'actif.
func (m *maintainerr) Do(ctx context.Context, id, _ string) (string, error) {
	if id != "rules" {
		return "", errUnknownAction
	}
	err := callAction(ctx, m.hc, http.MethodPost, m.url+"/rules/execute", nil, nil)
	var se statusError
	if errors.As(err, &se) && se.Code == http.StatusConflict {
		return "", errors.New("Maintainerr refuse : règles déjà en cours d'exécution ou aucun groupe actif")
	}
	if err != nil {
		return "", err
	}
	return "Exécution des règles lancée", nil
}
