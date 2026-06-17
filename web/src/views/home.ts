import { h, clear } from "../ui/dom";
import { identityEditor } from "../ui/identity";
import { buildHash } from "../proto";
import { randomRoom, randomPassword } from "../random";
import { copyText } from "../clipboard";

// Home screen: terminal-style "login" — no card/box chrome, just prompts on the
// black background. Fully mouse- and keyboard-driven; no CLI logic.
export function renderHome(app: HTMLElement): () => void {
  clear(app);

  const ident = identityEditor(go);

  const room = h("input", {
    type: "text",
    placeholder: "room name",
    "aria-label": "room name",
  });
  const pass = h("input", {
    type: "password",
    placeholder: "optional password",
    "aria-label": "password",
  });

  // Password reveal toggle. A randomized password is useless if you can't read
  // it, so randomizing also forces it visible.
  let pwVisible = false;
  const reveal = h(
    "button",
    { type: "button", class: "dd-shuffle", title: "show / hide password", "aria-label": "toggle password visibility" },
    ["show"],
  );
  function setReveal(v: boolean): void {
    pwVisible = v;
    pass.type = v ? "text" : "password";
    reveal.textContent = v ? "hide" : "show";
  }
  reveal.addEventListener("click", () => setReveal(!pwVisible));

  const enter = h("button", { type: "button", class: "dd-action" }, ["enter drop"]);

  function go(): void {
    const r = room.value.trim();
    if (!r) {
      room.focus();
      return;
    }
    ident.persist();
    location.hash = buildHash(r, pass.value);
  }

  enter.addEventListener("click", go);
  for (const el of [room, pass]) {
    el.addEventListener("keydown", (e) => {
      if (e.key === "Enter") go();
    });
  }

  const view = h("div", { class: "dd-login" }, [
    h("div", { class: "dd-logo" }, ["▚ DEADDROP"]),
    h("p", { class: "dd-sub" }, [
      "anonymous · ephemeral · end-to-end encrypted",
    ]),
    field("identity", ident.el, {}),
    field("room", room, {
      prompt: true,
      onRandom: () => {
        room.value = randomRoom();
        room.focus();
      },
    }),
    field("pass", pass, {
      prompt: true,
      extra: [reveal],
      onRandom: () => {
        pass.value = randomPassword();
        setReveal(true);
        pass.focus();
      },
    }),
    enter,
    h("p", { class: "dd-hint" }, [
      "tip: room + password derive the shared key — give both to invite. blank password = obscure room name only.",
    ]),
    cliLine(),
  ]);

  // Up/Down arrows move between the main fields (TUI feel). Swatches handle
  // their own left/right; this only acts when focus is on a field/button.
  view.addEventListener("keydown", (e) => {
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    const list: HTMLElement[] = [];
    const nameInput = ident.el.querySelector("input");
    if (nameInput) list.push(nameInput);
    list.push(room, pass, enter);
    const i = list.indexOf(document.activeElement as HTMLElement);
    if (i === -1) return;
    e.preventDefault();
    const ni = (i + (e.key === "ArrowDown" ? 1 : -1) + list.length) % list.length;
    list[ni].focus();
  });

  app.append(h("div", { class: "dd-center" }, [view]));
  ident.focus();

  return () => clear(app);
}

// A copyable `curl <host>/cli | sh` line for the native terminal client.
function cliLine(): HTMLElement {
  const cmd = `curl ${location.origin}/cli | sh`;
  const copy = h("button", { type: "button", class: "dd-cli-copy" }, ["copy"]);
  copy.addEventListener("click", () => {
    void copyText(cmd).then((ok) => {
      copy.textContent = ok ? "copied ✓" : "copy failed";
      setTimeout(() => {
        copy.textContent = "copy";
      }, 1500);
    });
  });
  return h("div", { class: "dd-cli" }, [
    h("span", { class: "dd-cli-label" }, ["terminal client"]),
    h("div", { class: "dd-cli-row" }, [
      h("span", { class: "dd-prompt" }, ["$"]),
      h("code", { class: "dd-cli-cmd" }, [cmd]),
      copy,
    ]),
  ]);
}

// A labelled field. `prompt` wraps the control in a `›` row; `onRandom` adds a
// trailing `⟳` button. The identity editor brings its own prompt + buttons, so
// it passes neither.
function field(
  label: string,
  control: HTMLElement,
  opts: { prompt?: boolean; onRandom?: () => void; extra?: HTMLElement[] },
): HTMLElement {
  let body: HTMLElement = control;
  if (opts.prompt) {
    const row = h("div", { class: "dd-prompt-row" }, [
      h("span", { class: "dd-prompt" }, ["›"]),
      control,
    ]);
    if (opts.extra) for (const e of opts.extra) row.append(e);
    if (opts.onRandom) {
      const rnd = h(
        "button",
        { type: "button", class: "dd-shuffle", title: "random", "aria-label": "random " + label },
        ["⟳"],
      );
      rnd.addEventListener("click", opts.onRandom);
      row.append(rnd);
    }
    body = row;
  }
  return h("label", { class: "dd-field" }, [
    h("span", { class: "dd-label" }, [label]),
    body,
  ]);
}
