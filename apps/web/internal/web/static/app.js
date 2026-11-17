// Hints that only make sense with JavaScript (e.g. "tap to save") start hidden.
document.querySelectorAll("[data-js-only]").forEach((el) => { el.hidden = false; });

// Progressive enhancement only: every form works without this file.
// Shows a loading state while a form submits and prevents double submits.
document.addEventListener("submit", (e) => {
  const form = e.target;
  if (!(form instanceof HTMLFormElement) || !form.hasAttribute("data-busy")) return;
  if (form.dataset.sending === "1") { e.preventDefault(); return; }
  form.dataset.sending = "1";
  document.documentElement.classList.add("is-busy");
  form.querySelectorAll("button[type=submit]").forEach((b) => b.setAttribute("aria-busy", "true"));
});
// Coming back via the back/forward cache must not leave the page looking busy.
window.addEventListener("pageshow", () => {
  document.documentElement.classList.remove("is-busy");
  document.querySelectorAll("form[data-sending]").forEach((f) => delete f.dataset.sending);
  document.querySelectorAll("[aria-busy]").forEach((b) => b.removeAttribute("aria-busy"));
});
// Card links also show the loading bar (detail pages wait on TMDB/AniList).
document.addEventListener("click", (e) => {
  const a = e.target instanceof Element ? e.target.closest("a[href^='/icerik/']") : null;
  if (a && !e.metaKey && !e.ctrlKey && !e.shiftKey && e.button === 0) document.documentElement.classList.add("is-busy");
});

// Search as you type: from the 2nd letter, 300 ms after the last keystroke, fetch
// only the results area (X-Partial) and keep ?q= in the address bar. The previous
// request is cancelled. Without JS the form's "Ara" button does the same.
(() => {
  const form = document.querySelector("form[data-live-search]");
  const out = document.getElementById("sonuclar");
  if (!form || !out || !window.fetch || !window.AbortController) return;
  const input = form.querySelector("input[name=q]");
  let timer = 0;
  let ctrl = null;
  let last = input.value.trim();

  const run = async () => {
    const q = input.value.trim();
    if (q === last || (q.length > 0 && q.length < 2)) return;
    last = q;
    if (ctrl) ctrl.abort();
    const mine = (ctrl = new AbortController());
    const url = q ? "/?q=" + encodeURIComponent(q) : "/";
    history.replaceState(null, "", url);
    out.setAttribute("aria-busy", "true");
    try {
      const res = await fetch(url, { headers: { "X-Partial": "1" }, signal: mine.signal, credentials: "same-origin" });
      if (res.redirected) { location.href = res.url; return; } // e.g. session expired
      const html = await res.text();
      if (!mine.signal.aborted) out.innerHTML = html;
    } catch (e) {
      if (e.name !== "AbortError") location.href = url; // fall back to a normal page load
    } finally {
      if (ctrl === mine) out.removeAttribute("aria-busy");
    }
  };

  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = setTimeout(run, 300);
  });
  form.addEventListener("submit", () => clearTimeout(timer));
})();

// ---------- Tap to save ----------
// Detail page: tapping a score or status saves it at once (tap the selected one again
// to clear it); the note saves 800 ms after typing stops and when the field loses
// focus. The UI updates optimistically and rolls back if the save fails.
// Bookmark buttons ("İzleyeceğim") anywhere toggle the planned status the same way.
(() => {
  if (!window.fetch) return;
  const csrf = () => (document.querySelector("input[name=csrf]") || {}).value || "";

  async function send(url, data) {
    const res = await fetch(url, {
      method: "POST",
      body: new URLSearchParams({ csrf: csrf(), ...data }),
      headers: { Accept: "application/json" },
      credentials: "same-origin",
    });
    if (res.redirected) { location.href = res.url; throw new Error("redirected"); } // e.g. signed out
    const json = await res.json().catch(() => ({}));
    if (!res.ok) throw Object.assign(new Error("save failed"), { msg: json.error });
    return json;
  }

  // small status line next to the "Senin puanın" heading
  const stateEl = document.querySelector("[data-save-state]");
  let stateTimer = 0;
  function showState(kind, msg) {
    if (!stateEl) return;
    clearTimeout(stateTimer);
    stateEl.dataset.kind = kind;
    stateEl.textContent = msg || stateEl.dataset[kind] || "";
    if (kind === "saved") stateTimer = setTimeout(() => { stateEl.textContent = ""; stateEl.dataset.kind = ""; }, 1500);
  }

  // a tiny toast for bookmark taps outside the detail form (search rows)
  function toast(msg) {
    let t = document.querySelector(".toast");
    if (!t) { t = document.createElement("div"); t.className = "toast"; t.setAttribute("role", "status"); document.body.appendChild(t); }
    t.textContent = msg;
    t.classList.add("show");
    clearTimeout(t._timer);
    t._timer = setTimeout(() => t.classList.remove("show"), 2500);
  }

  const form = document.querySelector("form[data-autosave]");
  const endpoint = form && form.dataset.autosave;
  const current = {};
  const seq = { score: 0, status: 0, note: 0 };

  function checkedValue(name) {
    const el = form.querySelector(`input[name=${name}]:checked`);
    return el ? el.value : "";
  }
  function setRadios(name, value) {
    form.querySelectorAll(`input[name=${name}]`).forEach((i) => { i.checked = i.value === value && value !== ""; });
  }
  function paintScore(value) {
    const v = document.querySelector("[data-my-score]");
    const hint = document.querySelector("[data-my-score-hint]");
    if (v) { v.textContent = value || "–"; v.closest(".my-score").classList.toggle("empty", !value); }
    if (hint) hint.textContent = value ? hint.dataset.rated : hint.dataset.unrated;
  }
  function syncStatus(titleId, status) {
    document.querySelectorAll(`form[data-quick][data-title="${CSS.escape(titleId)}"] button`).forEach((b) => {
      b.setAttribute("aria-pressed", status === "planned" ? "true" : "false");
    });
    if (form && form.dataset.title === titleId) { current.status = status; setRadios("status", status); }
  }
  function apply(field, value) {
    current[field] = value;
    if (field === "score") { setRadios("score", value); paintScore(value); }
    if (field === "status") syncStatus(form.dataset.title, value);
  }

  async function save(field, value, previous) {
    const mine = ++seq[field];
    showState("saving");
    try {
      const r = await send(endpoint, { alan: field, deger: value });
      if (mine !== seq[field]) return; // a newer tap is on its way
      apply("score", r.score == null ? "" : String(r.score));
      apply("status", r.status || "");
      current.note = r.note || "";
      showState("saved");
    } catch (e) {
      if (e.message === "redirected" || mine !== seq[field]) return;
      if (field !== "note") apply(field, previous); // roll the selection back
      showState("failed", e.msg);
    }
  }

  if (form) {
    current.score = checkedValue("score");
    current.status = checkedValue("status");
    const note = form.querySelector("textarea[name=note]");
    current.note = note ? note.value.trim() : "";

    form.addEventListener("click", (e) => {
      const input = e.target instanceof HTMLInputElement && e.target.type === "radio" ? e.target : null;
      if (!input || (input.name !== "score" && input.name !== "status")) return;
      const field = input.name;
      const previous = current[field];
      const value = previous === input.value ? "" : input.value; // tap again to clear
      apply(field, value);
      save(field, value, previous);
    });
    form.addEventListener("submit", (e) => e.preventDefault()); // nothing to submit with JS on

    if (note) {
      let timer = 0;
      const flush = () => {
        clearTimeout(timer);
        const v = note.value.trim();
        if (v === current.note) return;
        const prev = current.note;
        current.note = v;
        save("note", note.value, prev);
      };
      note.addEventListener("input", () => { clearTimeout(timer); timer = setTimeout(flush, 800); });
      note.addEventListener("blur", flush);
      window.addEventListener("pagehide", flush);
    }
  }

  // bookmark buttons: one tap toggles "İzleyeceğim"
  document.addEventListener("submit", async (e) => {
    const f = e.target;
    if (!(f instanceof HTMLFormElement) || !f.hasAttribute("data-quick")) return;
    e.preventDefault();
    const id = f.dataset.title;
    const btn = f.querySelector("button");
    const wasOn = btn.getAttribute("aria-pressed") === "true";
    const previous = form && form.dataset.title === id ? current.status : (wasOn ? "planned" : "");
    syncStatus(id, wasOn ? "" : "planned"); // optimistic
    if (form && form.dataset.title === id) showState("saving");
    try {
      const r = await send(f.action, { alan: "status", deger: "planned", tersine: "1" });
      syncStatus(id, r.status || "");
      if (form && form.dataset.title === id) showState("saved"); // elsewhere the filled icon is the feedback
    } catch (err) {
      if (err.message === "redirected") return;
      syncStatus(id, previous);
      const msg = err.msg || f.dataset.failed;
      if (form && form.dataset.title === id) showState("failed", msg); else toast(msg);
    }
  });
})();
