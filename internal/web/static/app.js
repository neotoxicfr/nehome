// Seul JS de nehome : remplacer les tuiles reçues par SSE, ouvrir le détail.
// Les fragments sont rendus et échappés côté serveur (html/template).
"use strict";
const dlg = document.getElementById("detail");
const live = document.getElementById("live");

// Les mises à jour remplacent le HTML : on garde ce que l'utilisateur a choisi
// (élément d'une action, case « Confirmer », listes dépliées) et le focus.
function snap(root) {
  const v = new Map();
  root.querySelectorAll("form.act select, form.act input, .more-t").forEach((el) => {
    const k = (el.form ? el.form.getAttribute("action") : el.id) + "|" + el.name;
    v.set(k, el.type === "checkbox" ? el.checked : el.value);
  });
  const a = document.activeElement;
  const f = a && root.contains(a) && a.form ? a.form.getAttribute("action") + "|" + a.name : "";
  return { v, f };
}
function restore(root, s) {
  root.querySelectorAll("form.act select, form.act input, .more-t").forEach((el) => {
    const k = (el.form ? el.form.getAttribute("action") : el.id) + "|" + el.name;
    if (!s.v.has(k)) return;
    if (el.type === "checkbox") el.checked = s.v.get(k);
    else if ([...(el.options || [])].some((o) => o.value === s.v.get(k))) el.value = s.v.get(k);
    if (k === s.f) el.focus();
  });
}

function openDetail(id) {
  const t = document.getElementById("t-" + id);
  if (!dlg || !t) return;
  const body = dlg.querySelector(".dlg-body");
  const keep = dlg.open && dlg.dataset.id === id ? snap(body) : null;
  const top = body.scrollTop;
  dlg.dataset.id = id;
  dlg.querySelector("h2").textContent = t.dataset.title;
  body.replaceChildren(t.querySelector("template.detail").content.cloneNode(true));
  if (keep) { restore(body, keep); body.scrollTop = top; }
  if (!dlg.open) dlg.showModal();
}

if (dlg) {
  document.addEventListener("click", (e) => {
    const t = e.target.closest(".tile");
    if (!t || e.target.closest("a.name, .acts")) return; // le titre ouvre le service, les actions agissent
    e.preventDefault();
    openDetail(t.dataset.id);
  });
  document.addEventListener("keydown", (e) => {
    const t = e.target.closest && e.target.closest(".tile");
    if (t && (e.key === "Enter" || e.key === " ") && e.target === t) {
      e.preventDefault();
      openDetail(t.dataset.id);
    }
  });
  dlg.addEventListener("click", (e) => { if (e.target === dlg) dlg.close(); }); // clic sur le fond
  dlg.querySelector(".close").addEventListener("click", () => dlg.close());
  dlg.addEventListener("close", () => { delete dlg.dataset.id; });

  const es = new EventSource("/events");
  es.onopen = () => { live.textContent = "en direct"; live.classList.add("on"); };
  es.onerror = () => { live.textContent = "reconnexion…"; live.classList.remove("on"); };
  es.addEventListener("reload", () => location.reload()); // liste des tuiles changée (config relue)
  es.addEventListener("brand", (e) => { // identité modifiée ailleurs : appliquée sans rechargement
    const m = JSON.parse(e.data);
    const link = document.querySelector('link[href^="/theme.css"]');
    if (link) link.href = "/theme.css?v=" + m.ThemeV;
    const brand = document.querySelector(".brand");
    if (brand) brand.lastChild.textContent = m.Brand;
    document.title = m.Brand;
    document.querySelectorAll('link[rel="icon"]').forEach((l) => { l.href = m.Favicon || "data:,"; });
  });
  es.addEventListener("tile", (e) => {
    const m = JSON.parse(e.data);
    const old = document.getElementById("t-" + m.id);
    if (!old || !m.html) return;
    const hadFocus = document.activeElement === old;
    const keep = snap(old);
    old.outerHTML = m.html;
    const nu = document.getElementById("t-" + m.id);
    restore(nu, keep);
    if (hadFocus) nu.focus();
    if (dlg.open && dlg.dataset.id === m.id) openDetail(m.id);
  });
}

// Réglages → Identité : aperçu immédiat (variables CSS, autorisées par la CSP via
// le CSSOM) puis enregistrement automatique ; le serveur valide et pousse le thème
// aux autres pages ouvertes.
const idForm = document.getElementById("identity");
if (idForm) {
  document.querySelectorAll(".nojs").forEach((e) => { e.hidden = true; });
  const root = document.documentElement.style;
  const lum = (hex) => {
    const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
      .map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
  };
  const mixBlack = (hex, part) => "#" + [1, 3, 5].map((i) => Math.round(parseInt(hex.slice(i, i + 2), 16) * (1 - part)).toString(16).padStart(2, "0")).join("");
  const apply = () => {
    const mode = idForm.querySelector('input[name="theme"]:checked');
    const sw = idForm.querySelector('input[name="accent"]:checked');
    if (!mode) return;
    const d = mode.dataset;
    let acc = !sw ? "#7aa2ff" : sw.value === "custom" ? idForm.elements.accent_custom.value : sw.dataset.hex;
    const light = lum(d.bg) > 0.4;
    if (light && lum(acc) > 0.3) acc = mixBlack(acc, 0.3); // comme le serveur : accent foncé sur fond clair
    root.setProperty("--bg", d.bg);
    root.setProperty("--surface", d.surface);
    root.setProperty("--text", d.text);
    root.setProperty("--accent", acc);
    root.setProperty("--on-accent", lum(acc) < 0.35 ? "#ffffff" : "#0b0f17");
    root.setProperty("color-scheme", light ? "light" : "dark");
    for (const [k, dk, lt] of [["--ok", "#3ecf8e", "#138a5a"], ["--warn", "#f5b84b", "#b7791f"], ["--crit", "#ff6b6b", "#d64545"]]) root.setProperty(k, light ? lt : dk);
    const name = idForm.elements.name.value.trim() || "nehome";
    const brand = document.querySelector(".brand");
    if (brand) brand.lastChild.textContent = name;
    const pv = document.querySelector(".pv-name");
    if (pv) pv.textContent = name;
    document.title = "Réglages · " + name;
  };
  const status = document.getElementById("id-status");
  let timer;
  const save = () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      const r = await fetch("/identity", { method: "POST", headers: { "X-Requested-With": "fetch" }, body: new URLSearchParams(new FormData(idForm)) });
      status.textContent = r.ok ? "Enregistré" : "Erreur d'enregistrement";
    }, 400);
  };
  idForm.elements.accent_custom.addEventListener("input", () => { idForm.querySelector('input[name="accent"][value="custom"]').checked = true; });
  idForm.addEventListener("input", () => { apply(); status.textContent = ""; save(); });
  idForm.addEventListener("change", () => { apply(); save(); });

  const favForm = document.getElementById("favicon-form");
  favForm.addEventListener("change", async () => {
    const st = document.getElementById("fav-status");
    const r = await fetch("/identity/favicon", { method: "POST", headers: { "X-Requested-With": "fetch" }, body: new FormData(favForm) });
    if (!r.ok) { st.textContent = (await r.text()).trim(); return; }
    const url = (await r.json()).favicon;
    const img = favForm.querySelector(".fav-preview");
    img.hidden = !url;
    if (url) img.src = url;
    document.querySelectorAll('link[rel="icon"]').forEach((l) => { l.href = url || "data:,"; });
    st.textContent = "Enregistré";
  });
}

// Actions : confirmation, exécution sans quitter la page, résultat en bas d'écran.
const toast = (msg, bad) => {
  let el = document.getElementById("toast");
  if (!el) { el = document.createElement("div"); el.id = "toast"; el.setAttribute("role", "status"); document.body.append(el); }
  el.textContent = msg;
  el.className = bad ? "toast bad" : "toast";
  clearTimeout(el._t);
  el._t = setTimeout(() => { el.className = "toast gone"; }, 5000);
};
document.querySelectorAll(".act-confirm").forEach((e) => { e.hidden = true; });
document.addEventListener("submit", async (e) => {
  const f = e.target.closest("form.act");
  if (!f) return;
  e.preventDefault();
  const q = f.dataset.confirm;
  if (q && !window.confirm(q)) return;
  const body = new URLSearchParams(new FormData(f));
  if (q) body.set("confirmed", "1");
  const btn = f.querySelector("button");
  btn.disabled = true;
  try {
    const r = await fetch(f.action, { method: "POST", headers: { "X-Requested-With": "fetch" }, body });
    const m = await r.json().catch(() => ({ error: "réponse illisible (HTTP " + r.status + ")" }));
    toast(m.message || m.error || "", !r.ok);
  } catch (err) {
    toast("Échec : " + err.message, true);
  } finally {
    btn.disabled = false;
  }
});

// Réglages : services présents filtrés, ajout et modification dans une fenêtre.
const filter = (input, items, groups) => {
  input.addEventListener("input", () => {
    const q = input.value.trim().toLowerCase();
    items().forEach((el) => { el.hidden = q !== "" && !el.dataset.q.toLowerCase().includes(q); });
    if (groups) groups().forEach((g) => { g.hidden = !g.querySelector("[data-q]:not([hidden])"); });
  });
};
const tileQ = document.getElementById("tile-q");
if (tileQ) { tileQ.hidden = false; filter(tileQ, () => document.querySelectorAll(".tcard")); }
document.querySelectorAll(".nojs-only").forEach((e) => e.remove());
const sdlg = document.getElementById("svc-dlg");
if (sdlg) {
  const body = sdlg.querySelector(".dlg-body");
  const title = sdlg.querySelector("h2");
  const show = (t, nodes) => { title.textContent = t; body.replaceChildren(...nodes); if (!sdlg.open) sdlg.showModal(); };
  sdlg.querySelector(".close").addEventListener("click", () => sdlg.close());
  sdlg.addEventListener("click", (e) => { if (e.target === sdlg) sdlg.close(); });
  const gallery = () => {
    const frag = document.getElementById("gallery-tpl").content.cloneNode(true);
    const q = frag.querySelector(".svc-q");
    show("Ajouter un service", [frag]);
    filter(q, () => body.querySelectorAll(".svc"), () => body.querySelectorAll(".svc-cat"));
    q.focus();
  };
  document.getElementById("add-btn").addEventListener("click", (e) => { e.preventDefault(); gallery(); });
  body.addEventListener("click", async (e) => {
    const a = e.target.closest(".svc");
    if (a) {
      e.preventDefault();
      const r = await fetch("/settings/form?add=" + encodeURIComponent(a.dataset.type));
      const back = document.createElement("button");
      back.type = "button"; back.className = "btn btn-ghost back"; back.textContent = "← Autres services";
      back.addEventListener("click", gallery);
      const box = document.createElement("div");
      box.innerHTML = await r.text(); // fragment rendu et échappé par le serveur (html/template)
      show("Ajouter · " + a.dataset.title, [back, box]);
    }
  });
  document.querySelectorAll("[data-display]").forEach((b) => b.addEventListener("click", () => {
    const tpl = b.closest(".tcard").querySelector(".display-tpl");
    show("Affichage et actions · " + b.dataset.title, [tpl.content.cloneNode(true)]);
  }));
}
