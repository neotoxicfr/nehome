package integ

import (
	"testing"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses d'après la documentation de l'API Paperless-ngx (/api/statistics/,
// /api/tasks/, /api/status/).
func TestPaperless(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	routes := map[string]string{
		"/api/statistics/": `{"documents_total":512,"documents_inbox":3,"inbox_tag":1,"document_file_type_counts":[{"mime_type":"application/pdf","mime_type_count":500}],"character_count":1234567,"tag_count":10,"correspondent_count":20,"document_type_count":5,"storage_path_count":2}`,
		"/api/status/":     `{"pngx_version":"2.14.7","storage":{"total":1000000000,"available":400000000}}`,
	}
	tasks := `[{"task_id":"a","task_file_name":"facture.pdf","status":"FAILURE","date_created":"` + now + `"},{"task_id":"b","task_file_name":"scan.pdf","status":"STARTED","date_created":"` + now + `"}]`
	for name, body := range map[string]string{"tableau": tasks, "page": `{"count":2,"results":` + tasks + `}`} {
		routes["/api/tasks/"] = body
		srv := f4Serve(t, f4Hdr("Authorization", "Token factice"), nil, routes)
		src, _ := newPaperless(config.Tile{URL: srv.URL}, mediaSecret)
		v, err := src.(*paperless).poll(t.Context())
		srv.Close()
		if err != nil || v["documents"].Text != "512" || v["inbox"].Text != "3" || v["inbox"].Level != "warn" || v["classes"].Text != "10 / 20 / 5" {
			t.Fatalf("%s : %v %+v", name, err, v)
		}
		if v["tasks"].Text != "1 en cours" || v["tasks"].Level != "crit" || len(v["task_list"].Rows) != 2 {
			t.Fatalf("%s tâches : %+v", name, v["tasks"])
		}
		if v["storage"].Pct != 60 || v["version"].Text != "2.14.7" {
			t.Fatalf("%s stockage : %+v", name, v["storage"])
		}
	}
}
