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
