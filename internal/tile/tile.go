// Package tile définit le contrat entre une intégration et l'affichage : une
// intégration déclare ses champs, puis pousse des valeurs déjà mises en forme.
package tile

import "context"

type Kind string

const (
	Stat  Kind = "stat"  // texte + sous-texte
	Meter Kind = "meter" // jauge 0-100
	Spark Kind = "spark" // courbe des dernières valeurs
	Bars  Kind = "bars"  // plusieurs jauges nommées
	Table Kind = "table" // liste
)

type Placement string

const (
	OnTile   Placement = "tile"
	InDetail Placement = "detail"
	Hidden   Placement = "hidden"
)

func (p Placement) Valid() bool { return p == OnTile || p == InDetail || p == Hidden }

type Field struct {
	ID      string
	Label   string
	Kind    Kind
	Default Placement
}

// Level colore une valeur : "" neutre, "ok", "warn", "crit".
type Value struct {
	Text   string
	Sub    string
	Level  string
	Pct    float64   // Meter
	Points []float64 // Spark
	Bars   []Bar
	Head   []string // Table
	Rows   [][]Cell
}

type Bar struct {
	Label, Text, Level string
	Pct                float64
}

type Cell struct {
	Text, Level string
}

type Values map[string]Value

// Source tourne jusqu'à l'annulation du contexte. emit(v, nil) fusionne des
// valeurs (partielles possibles) ; emit(nil, err) signale une panne.
type Source interface {
	Fields() []Field
	Run(ctx context.Context, emit func(Values, error))
}

// F déclare un champ.
func F(id, label string, k Kind, def Placement) Field {
	return Field{ID: id, Label: label, Kind: k, Default: def}
}

// Action : bouton proposé par une intégration (relancer, rafraîchir…).
type Action struct {
	ID, Label string
	Confirm   string   // question posée avant d'agir ("" : aucune)
	Choices   []string // si non vide, l'action porte sur l'élément choisi (conteneur, demande…)
	Default   Placement
}

// Actor : intégration qui sait agir. Actions() peut être appelée à tout moment
// (rendu) : les choix doivent venir d'un état protégé par un verrou.
type Actor interface {
	Actions() []Action
	Do(ctx context.Context, id, arg string) (string, error) // message de réussite
}
