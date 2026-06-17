// Tiny DOM helpers. No framework: WebTUI styles plain HTML via attributes,
// so we just build elements and set attributes directly.

type Child = Node | string;

export function h<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Record<string, string> = {},
  children: Child[] = [],
): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") el.className = v;
    else el.setAttribute(k, v);
  }
  for (const c of children) el.append(c);
  return el;
}

export function clear(el: HTMLElement): void {
  el.replaceChildren();
}
