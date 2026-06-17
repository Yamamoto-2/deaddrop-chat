// Smoke test for the relay: two clients in one room, one sends, the other must
// receive. Uses Node's built-in global WebSocket (Node >=22).
const URL = "ws://localhost:8080/ws";
const ROOM = "smoke-routing-id";

const wait = (ms) => new Promise((r) => setTimeout(r, ms));

function client(name) {
  const ws = new WebSocket(URL);
  ws.received = [];
  ws.addEventListener("message", (e) => ws.received.push(JSON.parse(e.data)));
  ws.ready = new Promise((res) => ws.addEventListener("open", res));
  ws.name = name;
  return ws;
}

const a = client("A");
const b = client("B");

await Promise.all([a.ready, b.ready]);
a.send(JSON.stringify({ t: "join", room: ROOM }));
b.send(JSON.stringify({ t: "join", room: ROOM }));
await wait(150);

a.send(JSON.stringify({ t: "msg", room: ROOM, nick: "alice", text: "hello dead drop" }));
await wait(200);

const gotMsg = b.received.find((m) => m.t === "msg" && m.text === "hello dead drop");
const gotPresence = b.received.find((m) => m.t === "presence" && m.n === 2);
const senderEcho = a.received.find((m) => m.t === "msg"); // server must NOT echo to sender

console.log("B received:", JSON.stringify(b.received));
console.log("A received:", JSON.stringify(a.received));

let ok = true;
if (!gotMsg) { console.error("FAIL: B did not receive the message"); ok = false; }
if (!gotPresence) { console.error("FAIL: B did not see presence n=2"); ok = false; }
if (senderEcho) { console.error("FAIL: server echoed msg back to sender"); ok = false; }

a.close();
b.close();
console.log(ok ? "PASS: relay works (fan-out, presence, no self-echo)" : "FAILED");
process.exit(ok ? 0 : 1);
