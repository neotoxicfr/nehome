package integ

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neotoxicfr/nehome/internal/config"
)

// Réponses relevées sur une vraie instance Cleanuparr (28/10/2026), titres et empreintes anonymisés.
var cuFixtures = map[string]string{
	"/api/v2/stats": `{"events": {"total": 30, "byType": {"DownloadMarkedForDeletion": 2, "FailedImportStrike": 18, "QueueItemDeleted": 2, "SearchTriggered": 2, "StalledStrike": 6}, "bySeverity": {"Important": 28, "Information": 2}}, "strikes": {"total": 24, "byType": {"Stalled": 6, "FailedImport": 18}, "recovered": 0}, "removals": {"total": 2, "byReason": {"FailedImport": 2}}, "cleaned": {"total": 0, "byReason": {}}, "searches": {"total": 2, "completed": 2, "failed": 0, "grabbed": 1, "byReason": {"Replacement": 2}}, "jobs": {"total": 1029, "completed": 1029, "failed": 0, "byType": {"QueueCleaner": {"total": 27, "completed": 27, "failed": 0, "lastRunAt": "2026-10-27T23:00:00.0002277+00:00", "nextRunAt": "2026-10-28T05:00:00+00:00"}, "Seeker": {"total": 1002, "completed": 1002, "failed": 0, "lastRunAt": "2026-10-28T04:40:00.000451+00:00", "nextRunAt": "2026-10-28T04:50:00+00:00"}, "MalwareBlocker": {"total": 0, "completed": 0, "failed": 0, "lastRunAt": null, "nextRunAt": null}, "DownloadCleaner": {"total": 0, "completed": 0, "failed": 0, "lastRunAt": null, "nextRunAt": null}, "BlacklistSynchronizer": {"total": 0, "completed": 0, "failed": 0, "lastRunAt": null, "nextRunAt": null}, "CustomFormatScoreSyncer": {"total": 0, "completed": 0, "failed": 0, "lastRunAt": null, "nextRunAt": null}}}, "health": {"downloadClients": [{"id": "ae38a9ee-53ee-4187-a7de-c4df2c028ccc", "name": "qBittorrent", "type": "qBittorrent", "isHealthy": true, "lastChecked": "2026-10-28T04:39:51.5007585+00:00", "responseTimeMs": 7.8296, "errorMessage": null}], "arrInstances": [{"id": "3921fdce-41b5-41a5-a714-5d2aa69228da", "name": "Sonarr", "type": "Sonarr", "isHealthy": true, "lastChecked": "2026-10-28T04:39:51.5048504+00:00", "errorMessage": null}, {"id": "18d16840-2918-4ffc-b2fb-29f4bd95582e", "name": "Radarr", "type": "Radarr", "isHealthy": true, "lastChecked": "2026-10-28T04:39:51.5070571+00:00", "errorMessage": null}]}, "timeframeHours": 168, "generatedAt": "2026-10-28T04:41:34.9760144+00:00"}`,
	"/api/strikes":  `{"items": [{"downloadItemId": "01a17f97-8c5d-70c9-a055-f352e836007e", "downloadId": "1111111111111111111111111111111111111111", "title": "Show.One.S01E02.1080p.WEB.x264-GROUP", "totalStrikes": 1, "strikesByType": {"FailedImport": 1}, "latestStrikeAt": "2026-10-27T23:00:00.734666+00:00", "firstStrikeAt": "2026-10-27T23:00:00.734666+00:00", "isMarkedForRemoval": false, "isRemoved": false, "isReturning": false, "hasDryRunStrikes": false, "strikes": [{"id": "01a17f97-8c5e-7a6f-869a-edcfca999fcd", "type": "FailedImport", "createdAt": "2026-10-27T23:00:00.734666+00:00", "lastDownloadedBytes": null, "jobRunId": "01a17f97-8980-7254-90e6-aa61ff36479d", "isDryRun": false}]}, {"downloadItemId": "01a17f97-8b27-79b3-8bb7-4bd25572d2dd", "downloadId": "2222222222222222222222222222222222222222", "title": "Show.Two.S00E03.1080p.WEBRip.x265-GROUP", "totalStrikes": 1, "strikesByType": {"FailedImport": 1}, "latestStrikeAt": "2026-10-27T23:00:00.4641516+00:00", "firstStrikeAt": "2026-10-27T23:00:00.4641516+00:00", "isMarkedForRemoval": false, "isRemoved": false, "isReturning": false, "hasDryRunStrikes": false, "strikes": [{"id": "01a17f97-8b50-749a-a037-8d3c9010e2bf", "type": "FailedImport", "createdAt": "2026-10-27T23:00:00.4641516+00:00", "lastDownloadedBytes": null, "jobRunId": "01a17f97-8980-7254-90e6-aa61ff36479d", "isDryRun": false}]}], "page": 1, "pageSize": 3, "totalCount": 2, "totalPages": 1}`,
	"/api/events":   `{"items": [{"id": "01a17f97-8c60-7dd5-b603-bc5e3782c6ab", "timestamp": "2026-10-27T23:00:00.7360487+00:00", "eventType": "FailedImportStrike", "message": "Item 'Show.One.S01E02.1080p.WEB.x264-GROUP' has been struck 1 times for reason 'FailedImport'", "severity": "Important", "trackingId": null, "strikeId": "01a17f97-8c5e-7a6f-869a-edcfca999fcd", "jobRunId": "01a17f97-8980-7254-90e6-aa61ff36479d", "arrInstanceId": "3921fdce-41b5-41a5-a714-5d2aa69228da", "downloadClientId": null, "searchStatus": null, "completedAt": null, "cycleId": null, "isDryRun": false, "itemTitle": "Show.One.S01E02.1080p.WEB.x264-GROUP", "itemHash": "1111111111111111111111111111111111111111", "strikeCount": 1, "failedImportReasons": ["Show.One.S01E02.1080p.WEB.x264-GROUP: Found matching series via grab history, but release was matched to series by ID. Automatic import is not possible. See the FAQ for details."], "deleteReason": null, "removeFromClient": null, "cleanReason": null, "cleanedCategory": null, "seedRatio": null, "seedingTimeHours": null, "oldCategory": null, "newCategory": null, "isCategoryTag": null, "searchType": null, "searchReason": null, "grabbedItems": []}, {"id": "01a17f97-8b80-78b3-8106-2205313391f3", "timestamp": "2026-10-27T23:00:00.5128926+00:00", "eventType": "FailedImportStrike", "message": "Item 'Show.Two.S00E03.1080p.WEBRip.x265-GROUP' has been struck 1 times for reason 'FailedImport'", "severity": "Important", "trackingId": null, "strikeId": "01a17f97-8b50-749a-a037-8d3c9010e2bf", "jobRunId": "01a17f97-8980-7254-90e6-aa61ff36479d", "arrInstanceId": "3921fdce-41b5-41a5-a714-5d2aa69228da", "downloadClientId": null, "searchStatus": null, "completedAt": null, "cycleId": null, "isDryRun": false, "itemTitle": "Show.Two.S00E03.1080p.WEBRip.x265-GROUP", "itemHash": "2222222222222222222222222222222222222222", "strikeCount": 1, "failedImportReasons": ["Show.Two.S00E03.1080p.WEBRip.x265-GROUP: Found matching series via grab history, but release was matched to series by ID. Automatic import is not possible. See the FAQ for details."], "deleteReason": null, "removeFromClient": null, "cleanReason": null, "cleanedCategory": null, "seedRatio": null, "seedingTimeHours": null, "oldCategory": null, "newCategory": null, "isCategoryTag": null, "searchType": null, "searchReason": null, "grabbedItems": []}, {"id": "01a17a71-a31c-7d57-b96e-05d95911664b", "timestamp": "2026-10-26T23:00:30.1086+00:00", "eventType": "SearchTriggered", "message": "Search triggered for Show.Three.S02.2160p.WEBRip.x265-GROUP", "severity": "Information", "trackingId": null, "strikeId": null, "jobRunId": "01a17a71-2d8f-7113-a71a-ac15942de0b7", "arrInstanceId": "3921fdce-41b5-41a5-a714-5d2aa69228da", "downloadClientId": null, "searchStatus": "Completed", "completedAt": "2026-10-26T23:01:06.6068528+00:00", "cycleId": null, "isDryRun": false, "itemTitle": "Show.Three.S02.2160p.WEBRip.x265-GROUP", "itemHash": null, "strikeCount": null, "failedImportReasons": [], "deleteReason": null, "removeFromClient": null, "cleanReason": null, "cleanedCategory": null, "seedRatio": null, "seedingTimeHours": null, "oldCategory": null, "newCategory": null, "isCategoryTag": null, "searchType": "Replacement", "searchReason": "Replacement", "grabbedItems": ["Gen.V.S02E03.MULTI.VF2.2160p.WEBRip.AMZ.DV.HDR10+.x265.EAC3.5.1.Atmos-Amen.FRENCH"]}], "page": 1, "pageSize": 3, "totalCount": 343, "totalPages": 115}`,
	"/api/jobs":     `[{"name": "QueueCleaner", "status": "Scheduled", "schedule": "0 0 0/6 * * ?", "nextRunTime": "2026-10-28T05:00:00+00:00", "previousRunTime": null, "jobType": "QueueCleaner"}, {"name": "MalwareBlocker", "status": "Not Scheduled", "schedule": "", "nextRunTime": null, "previousRunTime": null, "jobType": "MalwareBlocker"}, {"name": "DownloadCleaner", "status": "Not Scheduled", "schedule": "", "nextRunTime": null, "previousRunTime": null, "jobType": "DownloadCleaner"}, {"name": "BlacklistSynchronizer", "status": "Not Scheduled", "schedule": "", "nextRunTime": null, "previousRunTime": null, "jobType": "BlacklistSynchronizer"}, {"name": "Seeker", "status": "Scheduled", "schedule": "0 */10 * * * ?", "nextRunTime": "2026-10-28T04:50:00+00:00", "previousRunTime": "2026-10-28T04:40:00+00:00", "jobType": "Seeker"}, {"name": "CustomFormatScoreSyncer", "status": "Not Scheduled", "schedule": "", "nextRunTime": null, "previousRunTime": null, "jobType": "CustomFormatScoreSyncer"}]`,
	"/api/health":   `{"ae38a9ee-53ee-4187-a7de-c4df2c028ccc": {"isHealthy": true, "lastChecked": "2026-10-28T04:39:51.5007585+00:00", "errorMessage": null, "responseTime": "00:00:00.0078296", "clientId": "ae38a9ee-53ee-4187-a7de-c4df2c028ccc", "clientName": "qBittorrent", "clientTypeName": "qBittorrent"}}`,
}

func TestCleanuparr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "factice" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(cuFixtures[r.URL.Path]))
	}))
	defer srv.Close()
	src, err := newCleanuparr(config.Tile{URL: srv.URL}, func(string) (string, error) { return "factice", nil })
	if err != nil {
		t.Fatal(err)
	}
	c := src.(*cleanuparr)
	v, err := c.fast(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if v["strikes_total"].Text != "24" || v["removals"].Text != "2" || v["clients"].Text != "1 / 1" || v["struck"].Text == "" || len(v["strikes"].Rows) == 0 {
		t.Fatalf("valeurs : %+v", v)
	}
	s := c.slow(t.Context())
	if len(s["events"].Rows) == 0 || len(s["jobs"].Rows) != 2 {
		t.Fatalf("événements/tâches : %+v %+v", s["events"], s["jobs"])
	}
	// Mauvaise clé : erreur sans la clé dans le message.
	bad, _ := newCleanuparr(config.Tile{URL: srv.URL}, func(string) (string, error) { return "faux", nil })
	if _, err := bad.(*cleanuparr).fast(t.Context()); err == nil || strings.Contains(err.Error(), "faux") {
		t.Fatalf("erreur : %v", err)
	}
}
