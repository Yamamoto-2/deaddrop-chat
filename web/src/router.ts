import { parseHash } from "./proto";
import { renderHome } from "./views/home";
import { renderRoom } from "./views/room";

// Hash-based router: no room in the fragment -> home; otherwise the room view.
// Each view returns a cleanup function we call on navigation.
export function startRouter(): void {
  const app = document.getElementById("app") as HTMLElement;
  let cleanup: (() => void) | null = null;

  function route(): void {
    cleanup?.();
    cleanup = null;
    const parsed = parseHash(location.hash);
    cleanup = parsed && parsed.room ? renderRoom(app, parsed) : renderHome(app);
  }

  window.addEventListener("hashchange", route);
  route();
}
