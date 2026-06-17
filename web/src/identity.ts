// Local identity: a display name and a color. Both are per-person (stored in
// localStorage), so two people who pick the same name are still told apart by
// color. The color travels with every message (see proto.ts).

// Palette chosen for readable contrast on the dark background.
export const PALETTE: string[] = [
  "#5fd7a7", "#5fafff", "#ff87d7", "#ffd75f",
  "#ff8787", "#af87ff", "#5fd7d7", "#ffaf5f",
  "#87d75f", "#d7d787", "#ff6fb5", "#9d9dff",
];

// Identity lives in memory ONLY — never localStorage or cookies. A page reload
// clears it, so the name must be entered again every time (even same browser,
// same room). Leaving no trace is a project requirement.
let sessionNick = "";
let sessionColor = "";

export function getNick(): string {
  return sessionNick;
}

export function setNick(nick: string): void {
  sessionNick = nick;
}

export function randomColor(): string {
  return PALETTE[Math.floor(Math.random() * PALETTE.length)];
}

export function getColor(): string {
  if (!sessionColor) sessionColor = randomColor();
  return sessionColor;
}

export function setColor(color: string): void {
  sessionColor = color;
}
