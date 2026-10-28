package integ

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neotoxicfr/nehome/internal/config"
	"github.com/neotoxicfr/nehome/internal/tile"
)

// JDownloader par le relais MyJDownloader (api.jdownloader.org). Protocole :
// secrets dérivés de SHA-256(courriel+mot de passe+"server"|"device"),
// requêtes serveur signées en HMAC-SHA256, réponses et requêtes appareil
// chiffrées en AES-128-CBC (IV = 16 premiers octets du jeton, clé = 16
// suivants). Une seule session gardée tant qu'elle vit (MyJD limite les
// connexions) ; jeton expiré → nouveau /my/connect.
// Les appels appareil sont des POST imposés par le protocole, mais seulement
// des lectures (query*, get*), sauf les actions start/pause. Sondage 30 s.
// mu sérialise sondage et actions sur l'unique session.
type jdown struct {
	api, device, loginEmail string
	hc                      *http.Client

	mu sync.Mutex

	loginSecret, deviceSecret []byte
	session                   string
	serverTok, deviceTok      []byte
	deviceID                  string
	rid                       int64
	speed                     *tile.Ring
}

const jdAppKey = "nehome"

type jdPackage struct {
	Name     string  `json:"name"`
	Loaded   float64 `json:"bytesLoaded"`
	Total    float64 `json:"bytesTotal"`
	Speed    float64 `json:"speed"`
	ETA      int64   `json:"eta"` // secondes, -1 ou absent si inconnu
	Status   string  `json:"status"`
	Finished bool    `json:"finished"`
	Running  bool    `json:"running"`
	Enabled  bool    `json:"enabled"`
	Children int     `json:"childCount"`
}

// jdError : erreur renvoyée en clair par MyJD ({"src":"MYJD","type":"TOKEN_INVALID"}).
type jdError struct {
	Code int
	Type string
}

func (e jdError) Error() string {
	if e.Type == "" {
		return fmt.Sprintf("MyJDownloader : HTTP %d", e.Code)
	}
	return fmt.Sprintf("MyJDownloader : %s (HTTP %d)", e.Type, e.Code)
}

// jdAuthErr : la session est à refaire.
func jdAuthErr(err error) bool {
	var e jdError
	if !errors.As(err, &e) {
		return false
	}
	return e.Type == "TOKEN_INVALID" || e.Type == "AUTH_FAILED" || e.Code == http.StatusUnauthorized || e.Code == http.StatusForbidden
}

func newJDownloader(t config.Tile, secret func(string) (string, error)) (tile.Source, error) {
	j := &jdown{api: strings.TrimRight(t.URL, "/"), device: t.Options["device"], hc: httpClient(t.InsecureTLS, false), speed: tile.NewRing(60)}
	if j.api == "" {
		j.api = "https://api.jdownloader.org"
	}
	if j.device == "" {
		return nil, errors.New("options.device (nom de l'appareil JDownloader) manquant dans config.yaml")
	}
	user, err := secret("user")
	if err != nil {
		return nil, err
	}
	pass, err := secret("pass")
	if err != nil {
		return nil, err
	}
	j.loginSecret = jdHash([]byte(strings.ToLower(user) + pass + "server"))
	j.deviceSecret = jdHash([]byte(strings.ToLower(user) + pass + "device"))
	j.loginEmail = strings.ToLower(user)
	return j, nil
}

func (j *jdown) Fields() []tile.Field {
	return []tile.Field{
		tile.F("speed", "Vitesse", tile.Spark, tile.OnTile),
		tile.F("state", "État", tile.Stat, tile.OnTile),
		tile.F("active", "Paquets en cours", tile.Stat, tile.OnTile),
		tile.F("remaining", "Reste à télécharger", tile.Stat, tile.OnTile),
		tile.F("grabber", "Collecteur de liens", tile.Stat, tile.OnTile),
		tile.F("total", "Liste de téléchargement", tile.Stat, tile.InDetail),
		tile.F("packages", "Paquets non terminés", tile.Table, tile.InDetail),
	}
}

func (j *jdown) Run(ctx context.Context, emit func(tile.Values, error)) {
	defer j.disconnect()
	every(ctx, 30*time.Second, func() {
		v, err := j.poll(ctx)
		if ctx.Err() == nil {
			emit(v, err)
		}
	})
}

func jdHash(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// jdToken : nouveau jeton = SHA-256(ancien secret + jeton de session décodé de l'hexa).
func jdToken(secret []byte, session string) []byte {
	s, _ := hex.DecodeString(session)
	return jdHash(append(slices.Clone(secret), s...))
}

func jdDecrypt(body []byte, tok []byte) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(body)))
	if err != nil || len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return nil, errors.New("MyJDownloader : réponse chiffrée illisible")
	}
	b, _ := aes.NewCipher(tok[16:])
	cipher.NewCBCDecrypter(b, tok[:16]).CryptBlocks(raw, raw)
	pad := int(raw[len(raw)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(raw) {
		return nil, errors.New("MyJDownloader : déchiffrement impossible (identifiants faux ?)")
	}
	return raw[:len(raw)-pad], nil
}

func jdEncrypt(plain []byte, tok []byte) string {
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	buf := append(slices.Clone(plain), bytes.Repeat([]byte{byte(pad)}, pad)...)
	b, _ := aes.NewCipher(tok[16:])
	cipher.NewCBCEncrypter(b, tok[:16]).CryptBlocks(buf, buf)
	return base64.StdEncoding.EncodeToString(buf)
}

func (j *jdown) nextRid() int64 {
	if r := time.Now().UnixMilli(); r > j.rid {
		j.rid = r
	} else {
		j.rid++
	}
	return j.rid
}

// do envoie la requête ; l'URL (jetons, signature) n'apparaît jamais dans l'erreur.
func (j *jdown) do(ctx context.Context, method, u string, body io.Reader, ctype string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, errors.New("MyJDownloader : requête invalide")
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := j.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("MyJDownloader injoignable : %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("MyJDownloader : %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Type string `json:"type"`
		}
		json.Unmarshal(data, &e)
		return nil, jdError{resp.StatusCode, e.Type}
	}
	return data, nil
}

// server appelle /my/… : requête signée, réponse chiffrée par la même clé.
func (j *jdown) server(ctx context.Context, path string, q url.Values, key []byte, out any) error {
	rid := j.nextRid()
	q.Set("rid", fmt.Sprint(rid))
	p := path + "?" + q.Encode()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(p))
	data, err := j.do(ctx, http.MethodGet, j.api+p+"&signature="+hex.EncodeToString(mac.Sum(nil)), nil, "")
	if err != nil {
		return err
	}
	if data, err = jdDecrypt(data, key); err != nil {
		return err
	}
	var r struct {
		Rid int64 `json:"rid"`
	}
	if err := json.Unmarshal(data, &r); err != nil || r.Rid != rid {
		return errors.New("MyJDownloader : réponse inattendue (rid)")
	}
	return json.Unmarshal(data, out)
}

type jdSession struct {
	Session string `json:"sessiontoken"`
}

// login ouvre une session. /my/reconnect (regaintoken) a répondu
// TOKEN_INVALID au relevé, même sur une session fraîche : on refait un connect.
func (j *jdown) login(ctx context.Context) error {
	var s jdSession
	q := url.Values{"email": {j.loginEmail}, "appkey": {jdAppKey}}
	if err := j.server(ctx, "/my/connect", q, j.loginSecret, &s); err != nil {
		return err
	}
	if s.Session == "" {
		return errors.New("MyJDownloader : pas de jeton de session")
	}
	j.session = s.Session
	j.serverTok = jdToken(j.loginSecret, s.Session)
	j.deviceTok = jdToken(j.deviceSecret, s.Session)
	j.deviceID = ""
	return nil
}

func (j *jdown) disconnect() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.session == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var r struct{}
	j.server(ctx, "/my/disconnect", url.Values{"sessiontoken": {j.session}}, j.serverTok, &r)
	j.session = ""
}

func (j *jdown) findDevice(ctx context.Context) error {
	var r struct {
		List []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"list"`
	}
	if err := j.server(ctx, "/my/listdevices", url.Values{"sessiontoken": {j.session}}, j.serverTok, &r); err != nil {
		return err
	}
	var seen []string
	for _, d := range r.List {
		if d.Name == j.device {
			j.deviceID = d.ID
			return nil
		}
		seen = append(seen, d.Name)
	}
	return fmt.Errorf("appareil JDownloader %q introuvable (connectés : %s)", j.device, strings.Join(seen, ", "))
}

// call interroge l'appareil (requête chiffrée par le jeton appareil).
func (j *jdown) call(ctx context.Context, path string, params any, out any) error {
	rid := j.nextRid()
	body := map[string]any{"url": path, "rid": rid, "apiVer": 1}
	if params != nil {
		p, _ := json.Marshal(params)
		body["params"] = []string{string(p)}
	}
	plain, _ := json.Marshal(body)
	u := j.api + "/t_" + url.PathEscape(j.session) + "_" + url.PathEscape(j.deviceID) + path
	data, err := j.do(ctx, http.MethodPost, u, strings.NewReader(jdEncrypt(plain, j.deviceTok)), "application/aesjson-jd; charset=utf-8")
	if err != nil {
		return err
	}
	if data, err = jdDecrypt(data, j.deviceTok); err != nil {
		return err
	}
	var r struct {
		Rid  int64           `json:"rid"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &r); err != nil || r.Rid != rid {
		return errors.New("MyJDownloader : réponse inattendue (rid)")
	}
	return json.Unmarshal(r.Data, out)
}

type jdState struct {
	state    string
	speed    float64
	packages []jdPackage
	grabber  []jdPackage
}

// ensure ouvre la session et retrouve l'appareil si besoin.
func (j *jdown) ensure(ctx context.Context) error {
	if j.session == "" {
		if err := j.login(ctx); err != nil {
			return err
		}
	}
	if j.deviceID == "" {
		return j.findDevice(ctx)
	}
	return nil
}

func (j *jdown) fetch(ctx context.Context) (*jdState, error) {
	if err := j.ensure(ctx); err != nil {
		return nil, err
	}
	var s jdState
	err := j.call(ctx, "/downloadsV2/queryPackages", map[string]any{
		"bytesLoaded": true, "bytesTotal": true, "enabled": true, "eta": true, "finished": true,
		"running": true, "speed": true, "status": true, "childCount": true, "maxResults": -1, "startAt": 0,
	}, &s.packages)
	if err != nil {
		return nil, err
	}
	// Le reste est secondaire : un échec laisse la valeur vide.
	j.call(ctx, "/downloadcontroller/getCurrentState", nil, &s.state)
	j.call(ctx, "/downloadcontroller/getSpeedInBps", nil, &s.speed)
	if j.call(ctx, "/linkgrabberv2/queryPackages", map[string]any{"childCount": true, "bytesTotal": true, "maxResults": -1, "startAt": 0}, &s.grabber) != nil {
		s.grabber = nil
	}
	return &s, nil
}

func (j *jdown) Actions() []tile.Action {
	return []tile.Action{
		{ID: "start", Label: "Démarrer les téléchargements"},
		{ID: "pause", Label: "Mettre en pause"},
	}
}

// Do : /downloadcontroller/start (sans paramètre) et
// /downloadcontroller/pause (value=true), sur la session partagée.
func (j *jdown) Do(ctx context.Context, id, _ string) (string, error) {
	var path, msg string
	var params any
	switch id {
	case "start":
		path, msg = "/downloadcontroller/start", "Téléchargements démarrés"
	case "pause":
		path, msg, params = "/downloadcontroller/pause", "Téléchargements en pause", true
	default:
		return "", fmt.Errorf("action inconnue : %s", id)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	run := func() error {
		if err := j.ensure(ctx); err != nil {
			return err
		}
		var ok bool
		return j.call(ctx, path, params, &ok)
	}
	err := run()
	if jdAuthErr(err) && ctx.Err() == nil {
		if err = j.login(ctx); err == nil {
			err = run()
		}
	}
	if err != nil {
		return "", err
	}
	return msg, nil
}

func (j *jdown) poll(ctx context.Context) (tile.Values, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	s, err := j.fetch(ctx)
	if jdAuthErr(err) && ctx.Err() == nil {
		// Jeton expiré : une seule relance, avec une session refaite.
		if err = j.login(ctx); err == nil {
			s, err = j.fetch(ctx)
		}
	}
	var je jdError
	if errors.As(err, &je) && je.Type != "" && !jdAuthErr(err) {
		j.deviceID = "" // appareil hors ligne ou renommé : on le recherchera
	}
	if err != nil {
		return nil, err
	}
	return j.values(s), nil
}

var jdStates = map[string]struct{ text, level string }{
	"RUNNING":  {"en cours", "ok"},
	"PAUSE":    {"en pause", "warn"},
	"IDLE":     {"à l'arrêt", ""},
	"STOPPED":  {"arrêté", ""},
	"STOPPING": {"arrêt en cours", ""},
}

func (j *jdown) values(s *jdState) tile.Values {
	v := tile.Values{"speed": {Text: tile.Rate(s.speed), Points: j.speed.Push(s.speed)}}
	if st, ok := jdStates[s.state]; ok {
		v["state"] = tile.Value{Text: st.text, Level: st.level}
	} else if s.state != "" {
		v["state"] = tile.Value{Text: strings.ToLower(s.state)}
	}
	var running, done int
	var left, total float64
	var rows [][]tile.Cell
	for _, p := range s.packages {
		total += p.Total
		if p.Finished {
			done++
			continue
		}
		left += max(p.Total-p.Loaded, 0)
		if p.Running {
			running++
		}
		if len(rows) < 15 {
			pct := 0.0
			if p.Total > 0 {
				pct = p.Loaded / p.Total * 100
			}
			eta := ""
			if p.ETA > 0 {
				eta = tile.Dur(time.Duration(p.ETA) * time.Second)
			}
			lvl := ""
			if p.Running {
				lvl = "ok"
			} else if !p.Enabled {
				lvl = "warn"
			}
			name := tile.Cell{Text: p.Name, Level: lvl}
			rows = append(rows, []tile.Cell{name, {Text: tile.Pct(pct)}, {Text: jdSize(p.Total)}, {Text: jdSpeed(p.Speed)}, {Text: eta}, {Text: p.Status}})
		}
	}
	v["active"] = tile.Value{Text: fmt.Sprint(running), Sub: fmt.Sprintf("%d en attente", len(s.packages)-done-running)}
	rem := tile.Value{Text: tile.Bytes(left)}
	if s.speed > 0 && left > 0 {
		rem.Sub = "fin dans " + tile.Dur(time.Duration(left/s.speed)*time.Second)
	}
	v["remaining"] = rem
	v["total"] = tile.Value{Text: fmt.Sprintf("%d paquets", len(s.packages)), Sub: fmt.Sprintf("%d terminés, %s au total", done, tile.Bytes(total))}
	v["packages"] = tile.Value{Head: []string{"Paquet", "Progression", "Taille", "Vitesse", "Reste", "État"}, Rows: rows, Text: "rien en attente"}
	if s.grabber != nil {
		links := 0
		var size float64
		for _, p := range s.grabber {
			links += p.Children
			size += p.Total
		}
		sub := fmt.Sprintf("%d paquets", len(s.grabber))
		if size > 0 {
			sub += ", " + tile.Bytes(size)
		}
		v["grabber"] = tile.Value{Text: ncPlural(links, "lien", "liens"), Sub: sub}
	}
	return v
}

// jdSize : taille inconnue (lien hors ligne ou pas encore vérifié) → « ? ».
func jdSize(b float64) string {
	if b <= 0 {
		return "?"
	}
	return tile.Bytes(b)
}

func jdSpeed(b float64) string {
	if b <= 0 {
		return ""
	}
	return tile.Rate(b)
}
