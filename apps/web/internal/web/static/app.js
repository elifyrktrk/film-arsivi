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
