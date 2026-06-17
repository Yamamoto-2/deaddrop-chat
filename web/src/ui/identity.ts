import { h } from "./dom";
import {
  PALETTE,
  getNick,
  getColor,
  setNick,
  setColor,
  randomColor,
} from "../identity";
import { randomNick } from "../random";

// A reusable name + color editor used on the home screen and inside the room's
// identity popover. Graphical (click swatches), keyboard-friendly (Tab/Enter),
// styled to look like a terminal prompt.
export interface IdentityEditor {
  el: HTMLElement;
  focus(): void;
  read(): { nick: string; color: string };
  persist(): { nick: string; color: string };
}

export function identityEditor(onEnter?: () => void): IdentityEditor {
  let color = getColor();

  const nickInput = h("input", {
    type: "text",
    placeholder: "name",
    value: getNick(),
    class: "dd-nick-input",
    "aria-label": "your name",
  });
  if (onEnter) {
    nickInput.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        // preventDefault so this same Enter doesn't bleed into the message
        // textarea (which onEnter focuses) and insert a stray newline.
        e.preventDefault();
        onEnter();
      }
    });
  }

  const swatches = h("div", {
    class: "dd-swatches",
    role: "radiogroup",
    "aria-label": "name color",
  });
  function paint(): void {
    swatches.replaceChildren(
      ...PALETTE.map((c) => {
        const sw = h("button", {
          type: "button",
          class: "dd-swatch" + (c === color ? " is-on" : ""),
          style: `--sw:${c}`,
          title: c,
          role: "radio",
          "aria-checked": String(c === color),
          "aria-label": "color " + c,
          // Roving tabindex: Tab lands on the selected swatch; arrows move.
          tabindex: c === color ? "0" : "-1",
        });
        sw.addEventListener("click", () => {
          color = c;
          paint();
        });
        return sw;
      }),
    );
  }
  paint();

  // Arrow keys move the selection within the swatch grid (TUI feel).
  function moveColor(delta: number): void {
    const idx = Math.max(0, PALETTE.indexOf(color));
    color = PALETTE[(idx + delta + PALETTE.length) % PALETTE.length];
    paint();
    (swatches.children[PALETTE.indexOf(color)] as HTMLElement | undefined)?.focus();
  }
  swatches.addEventListener("keydown", (e) => {
    if (e.key === "ArrowRight" || e.key === "ArrowDown") {
      e.preventDefault();
      moveColor(1);
    } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      moveColor(-1);
    }
  });

  const nameRandom = h(
    "button",
    { type: "button", class: "dd-shuffle", title: "random name", "aria-label": "random name" },
    ["⟳"],
  );
  nameRandom.addEventListener("click", () => {
    nickInput.value = randomNick();
    nickInput.focus();
  });

  const colorRandom = h(
    "button",
    { type: "button", class: "dd-shuffle", title: "random color", "aria-label": "random color" },
    ["⟳"],
  );
  colorRandom.addEventListener("click", () => {
    color = randomColor();
    paint();
  });

  const el = h("div", { class: "dd-identity" }, [
    h("div", { class: "dd-id-row" }, [
      h("span", { class: "dd-prompt" }, ["›"]),
      nickInput,
      nameRandom,
    ]),
    h("div", { class: "dd-swatch-row" }, [swatches, colorRandom]),
  ]);

  function read(): { nick: string; color: string } {
    return { nick: nickInput.value.trim() || "anon", color };
  }
  function persist(): { nick: string; color: string } {
    const v = read();
    setNick(v.nick);
    setColor(v.color);
    return v;
  }

  return { el, focus: () => nickInput.focus(), read, persist };
}
