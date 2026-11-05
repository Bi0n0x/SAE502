// Casino R&T. Small comforts on top of forms that work without JavaScript:
// stake shortcuts, the name of the chosen roulette bet, and keeping the wheel
// in view on a phone when the ball is launched.
(() => {
  "use strict";

  const reduced = matchMedia("(prefers-reduced-motion: reduce)");

  // Stake shortcuts. The buttons ship hidden and only appear here, so a page
  // without JavaScript never shows controls that do nothing.
  function setupStake(root) {
    for (const group of root.querySelectorAll(".stake-keys[hidden]")) {
      group.hidden = false;
    }
  }

  document.addEventListener("click", (e) => {
    const key = e.target.closest(".stake-keys button");
    if (!key) return;
    const input = key.closest("form").querySelector('input[name="amount"]');
    if (!input) return;
    const lo = Number(input.min) || 1;
    const hi = Number(input.max) || lo;
    const cur = Number(input.value) || lo;
    let next = cur;
    switch (key.dataset.stake) {
      case "min": next = lo; break;
      case "half": next = Math.floor(cur / 2); break;
      case "double": next = cur * 2; break;
      case "max": next = hi; break;
    }
    input.value = String(Math.min(hi, Math.max(lo, next)));
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });

  // Roulette: say in words which pocket the stake is on.
  document.addEventListener("change", (e) => {
    const radio = e.target;
    if (radio.name !== "option") return;
    const form = radio.closest("form");
    const out = form && form.querySelector(".pick b");
    const label = radio.closest("[data-pick]");
    if (out && label) out.textContent = label.dataset.pick;
  });

  // After a launch, bring the wheel back on screen if the felt pushed it away.
  // The stage is swapped with outerHTML, so look it up again rather than
  // trusting the event target, which is the element that was replaced.
  document.addEventListener("htmx:afterSettle", () => {
    const stage = document.getElementById("stage");
    if (!stage) return;
    setupStake(stage);
    const wheel = stage.querySelector(".wheel");
    if (!wheel) return;
    const r = wheel.getBoundingClientRect();
    if (r.top < 0 || r.bottom > innerHeight) {
      wheel.scrollIntoView({ block: "center", behavior: reduced.matches ? "auto" : "smooth" });
    }
  });

  setupStake(document);
})();
