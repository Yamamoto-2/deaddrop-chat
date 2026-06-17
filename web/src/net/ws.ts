import type { ConnState, Incoming, Outgoing } from "../proto";

// Connection wraps a WebSocket with auto-reconnect and an outgoing queue.
// The URL is derived from the current origin, so no domain is ever hardcoded:
// works on localhost and behind nginx unchanged.
export class Connection {
  private ws: WebSocket | null = null;
  private readonly url: string;
  private queue: string[] = [];
  private closed = false;
  private retry = 0;

  constructor(
    private readonly onMsg: (m: Incoming) => void,
    private readonly onState: (s: ConnState) => void,
  ) {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    this.url = `${proto}://${location.host}/ws`;
  }

  connect(): void {
    this.onState("connecting");
    const ws = new WebSocket(this.url);
    this.ws = ws;

    ws.onopen = () => {
      this.retry = 0;
      this.onState("open");
      for (const m of this.queue) ws.send(m);
      this.queue = [];
    };
    ws.onmessage = (ev) => {
      try {
        this.onMsg(JSON.parse(ev.data));
      } catch {
        /* ignore malformed frames */
      }
    };
    ws.onclose = () => {
      this.onState("closed");
      if (!this.closed) {
        this.retry = Math.min(this.retry + 1, 6);
        setTimeout(() => this.connect(), 400 * this.retry);
      }
    };
    ws.onerror = () => ws.close();
  }

  send(m: Outgoing): void {
    const s = JSON.stringify(m);
    if (this.ws && this.ws.readyState === WebSocket.OPEN) this.ws.send(s);
    else this.queue.push(s);
  }

  close(): void {
    this.closed = true;
    this.ws?.close();
  }
}
