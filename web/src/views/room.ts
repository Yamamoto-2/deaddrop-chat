import { h, clear } from "../ui/dom";
import { Connection } from "../net/ws";
import { identityEditor } from "../ui/identity";
import { getNick, getColor } from "../identity";
import { deriveKey, encryptJSON, decryptJSON } from "../crypto";
import { renderMarkdown } from "../markdown";
import { highlightCode } from "../highlight";
import { copyText } from "../clipboard";
import { readFilePayload, fileBlobUrl, humanSize, MAX_FILE_BYTES } from "../file";
import {
  deriveRoutingId,
  type ConnState,
  type FilePayload,
  type Incoming,
  type MsgPayload,
  type ParsedHash,
} from "../proto";

// Room screen: terminal-window panel. Messages render Markdown; attachments
// (images + arbitrary files) travel encrypted in the same channel. Input is a
// textarea — Enter sends, Ctrl+Enter inserts a newline.
export function renderRoom(app: HTMLElement, parsed: ParsedHash): () => void {
  clear(app);

  let nick = getNick() || "anon";
  let color = getColor();

  const presence = h("span", { class: "dd-dim" }, ["◍ 0"]);
  const status = h("span", { class: "dd-dim" }, ["connecting…"]);

  const idBtn = h("button", {
    type: "button",
    class: "dd-id",
    "aria-label": "change your name and color",
  });
  function paintId(): void {
    idBtn.replaceChildren(
      h("span", { class: "dd-dot", style: `--sw:${color}` }),
      document.createTextNode(nick),
    );
  }
  paintId();

  const invite = h("button", { type: "button", class: "dd-link" }, ["invite"]);
  invite.addEventListener("click", () => flashCopy(invite, "invite"));
  const leave = h("button", { type: "button", class: "dd-link" }, ["leave"]);
  leave.addEventListener("click", () => {
    location.hash = "";
  });

  const header = h("header", { class: "dd-header" }, [
    h("span", { class: "dd-room" }, ["#" + parsed.room]),
    h("span", { class: "dd-spacer" }),
    h("span", { class: "dd-meta" }, [
      presence,
      h("span", { class: "dd-sep" }, ["·"]),
      status,
    ]),
    h("span", { class: "dd-sep" }, ["·"]),
    idBtn,
    invite,
    leave,
  ]);

  const log = h("div", { class: "dd-log" });
  const empty = buildEmptyState(parsed);
  log.append(empty);

  const input = h("textarea", {
    rows: "1",
    placeholder: "type a message…  (Enter to send, Ctrl+Enter for newline)",
    class: "dd-input",
    "aria-label": "message",
  });
  const fileInput = h("input", { type: "file", class: "dd-fileinput" });
  const attach = h(
    "button",
    { type: "button", class: "dd-attach", title: "attach a file", "aria-label": "attach file" },
    ["+"],
  );
  attach.addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", () => {
    const f = fileInput.files?.[0];
    if (f) void sendFile(f);
    fileInput.value = "";
  });
  const send = h("button", { type: "button", class: "dd-send" }, ["↵"]);
  const inputBar = h("div", { class: "dd-inputbar" }, [
    h("span", { class: "dd-prompt" }, ["›"]),
    input,
    attach,
    send,
    fileInput,
  ]);

  app.append(header, rule(), log, rule(), inputBar);

  // --- identity modal ---
  let backdrop: HTMLElement | null = null;
  function onEsc(e: KeyboardEvent): void {
    if (e.key === "Escape" && backdrop) {
      e.stopPropagation();
      closeIdentity();
    }
  }
  function closeIdentity(): void {
    backdrop?.remove();
    backdrop = null;
    document.removeEventListener("keydown", onEsc, true);
    input.focus();
  }
  function openIdentity(firstRun: boolean): void {
    if (backdrop) return;
    const ed = identityEditor(save);
    const ok = h("button", { type: "button", class: "dd-send" }, ["save"]);
    function save(): void {
      const v = ed.persist();
      nick = v.nick;
      color = v.color;
      paintId();
      closeIdentity();
    }
    ok.addEventListener("click", save);
    const modal = h("div", { class: "dd-modal", "box-": "square" }, [
      h("div", { class: "dd-modal-title" }, [
        firstRun ? "set your identity" : "identity",
      ]),
      ed.el,
      h("div", { class: "dd-modal-actions" }, [ok]),
    ]);
    backdrop = h("div", { class: "dd-backdrop" }, [modal]);
    backdrop.addEventListener("mousedown", (e) => {
      if (e.target === backdrop) closeIdentity();
    });
    app.append(backdrop);
    ed.focus();
    document.addEventListener("keydown", onEsc, true);
  }
  idBtn.addEventListener("click", () => openIdentity(false));

  // --- connection + crypto ---
  let routingId = "";
  let key: Uint8Array | null = null;
  const conn = new Connection(onMessage, onState);

  void Promise.all([
    deriveRoutingId(parsed.room),
    deriveKey(parsed.room, parsed.password),
  ]).then(([id, k]) => {
    routingId = id;
    key = k;
    conn.connect();
  });

  function onState(s: ConnState): void {
    status.textContent =
      s === "open" ? "● live" : s === "connecting" ? "connecting…" : "offline";
    status.classList.toggle("dd-live", s === "open");
    if (s === "open" && routingId) conn.send({ t: "join", room: routingId });
  }

  function onMessage(m: Incoming): void {
    if (m.t === "presence") {
      presence.textContent = "◍ " + m.n;
    } else if (m.t === "msg") {
      if (!key) return;
      const p = decryptJSON<MsgPayload>(key, m.c);
      if (p) appendMsg(p);
    }
  }

  // --- message log ---
  let hasMessages = false;

  function clearEmpty(): void {
    if (!hasMessages) {
      empty.remove();
      hasMessages = true;
    }
  }

  function appendMsg(p: MsgPayload): void {
    clearEmpty();
    const nearBottom = log.scrollHeight - log.scrollTop - log.clientHeight < 60;

    const nickEl = h("span", { class: "dd-nick" }, [p.nick + ":"]);
    nickEl.style.color = p.color || "var(--foreground1)";

    const content = h("div", { class: "dd-text" });
    if (typeof p.text === "string" && p.text.length > 0) {
      content.innerHTML = renderMarkdown(p.text);
      content.querySelectorAll("a").forEach((a) => {
        a.setAttribute("target", "_blank");
        a.setAttribute("rel", "noopener noreferrer");
      });
      void highlightCode(content); // lazy: only fetches hljs if a code block exists
    }
    if (p.file) content.append(fileToken(p.file));

    log.append(
      h("div", { class: "dd-msg" }, [
        h("span", { class: "dd-time" }, [fmtTime(p.ts)]),
        nickEl,
        content,
      ]),
    );
    if (nearBottom) log.scrollTop = log.scrollHeight;
  }

  function sysMsg(text: string): HTMLElement {
    clearEmpty();
    const el = h("div", { class: "dd-sys" }, [text]);
    log.append(el);
    log.scrollTop = log.scrollHeight;
    return el;
  }

  // --- attachments ---
  function fileToken(f: FilePayload): HTMLElement {
    const isImg = f.mime.startsWith("image/");
    const label = `${isImg ? "▣ image" : "⎙ " + f.name} · ${humanSize(f.size)}`;
    const btn = h("button", { type: "button", class: "dd-file" }, [label]);
    if (isImg) {
      btn.addEventListener("click", () => openOverlay(f));
      btn.addEventListener("mouseenter", () => showPreview(f, btn));
      btn.addEventListener("mouseleave", hidePreview);
    } else {
      btn.addEventListener("click", () => downloadFile(f));
    }
    return btn;
  }

  let preview: HTMLElement | null = null;
  let previewUrl: string | null = null;
  function showPreview(f: FilePayload, anchor: HTMLElement): void {
    hidePreview();
    previewUrl = fileBlobUrl(f);
    preview = h("div", { class: "dd-preview" }, [
      h("img", { src: previewUrl, class: "dd-preview-img", alt: "" }),
    ]);
    document.body.append(preview);
    const r = anchor.getBoundingClientRect();
    preview.style.left = r.left + "px";
    preview.style.bottom = window.innerHeight - r.top + 6 + "px";
  }
  function hidePreview(): void {
    preview?.remove();
    preview = null;
    if (previewUrl) {
      URL.revokeObjectURL(previewUrl);
      previewUrl = null;
    }
  }

  function openOverlay(f: FilePayload): void {
    const url = fileBlobUrl(f);
    const overlay = h("div", { class: "dd-overlay" }, [
      h("img", { src: url, class: "dd-overlay-img", alt: f.name }),
    ]);
    function close(): void {
      overlay.remove();
      URL.revokeObjectURL(url);
      document.removeEventListener("keydown", onKey, true);
    }
    function onKey(e: KeyboardEvent): void {
      if (e.key === "Escape") close();
    }
    overlay.addEventListener("click", close);
    document.addEventListener("keydown", onKey, true);
    app.append(overlay);
  }

  function downloadFile(f: FilePayload): void {
    const url = fileBlobUrl(f);
    const a = h("a", { href: url, download: f.name });
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
  }

  async function sendFile(file: File): Promise<void> {
    if (!routingId || !key) return;
    if (file.size > MAX_FILE_BYTES) {
      sysMsg(`"${file.name}" is too large (${humanSize(file.size)}); max ${humanSize(MAX_FILE_BYTES)}`);
      return;
    }
    // Show progress before the (synchronous) encrypt blocks the thread.
    const pending = sysMsg(`sending "${file.name}" (${humanSize(file.size)})…`);
    await new Promise((r) => setTimeout(r, 0));
    let filePayload: FilePayload;
    try {
      filePayload = await readFilePayload(file);
    } catch {
      pending.remove();
      sysMsg(`could not read "${file.name}"`);
      return;
    }
    const p: MsgPayload = { nick, color, ts: Date.now(), file: filePayload };
    try {
      conn.send({ t: "msg", room: routingId, c: encryptJSON(key, p) });
    } catch {
      pending.remove();
      sysMsg(`failed to send "${file.name}"`);
      return;
    }
    pending.remove();
    appendMsg(p);
  }

  // drag & drop files onto the log
  log.addEventListener("dragover", (e) => {
    e.preventDefault();
    log.classList.add("dd-drop");
  });
  log.addEventListener("dragleave", () => log.classList.remove("dd-drop"));
  log.addEventListener("drop", (e) => {
    e.preventDefault();
    log.classList.remove("dd-drop");
    const f = e.dataTransfer?.files?.[0];
    if (f) void sendFile(f);
  });

  // --- composing ---
  function autogrow(): void {
    input.style.height = "auto";
    input.style.height = input.scrollHeight + "px";
  }
  function insertNewline(): void {
    const start = input.selectionStart;
    const end = input.selectionEnd;
    input.value = input.value.slice(0, start) + "\n" + input.value.slice(end);
    input.selectionStart = input.selectionEnd = start + 1;
    autogrow();
  }
  function doSend(): void {
    const text = input.value;
    if (!text.trim() || !routingId || !key) return;
    const p: MsgPayload = { nick, color, ts: Date.now(), text };
    conn.send({ t: "msg", room: routingId, c: encryptJSON(key, p) });
    appendMsg(p);
    input.value = "";
    autogrow();
    input.focus();
  }

  send.addEventListener("click", doSend);
  input.addEventListener("input", autogrow);
  input.addEventListener("keydown", (e) => {
    if (e.key !== "Enter") return;
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault();
      insertNewline();
    } else {
      e.preventDefault();
      doSend();
    }
  });
  // Paste an image straight from the clipboard → send it as an attachment.
  input.addEventListener("paste", (e) => {
    for (const item of e.clipboardData?.items ?? []) {
      if (item.kind === "file" && item.type.startsWith("image/")) {
        const f = item.getAsFile();
        if (f) {
          e.preventDefault();
          void sendFile(f);
          return;
        }
      }
    }
  });

  if (!getNick()) openIdentity(true);
  else input.focus();

  return () => {
    hidePreview();
    closeIdentity();
    conn.close();
    clear(app);
  };
}

// --- helpers ---

function rule(): HTMLElement {
  return h("div", { class: "dd-rule", "aria-hidden": "true" });
}

function fmtTime(ts: number): string {
  const d = new Date(ts);
  const p = (n: number): string => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}`;
}

function buildEmptyState(parsed: ParsedHash): HTMLElement {
  const copyBtn = h("button", { type: "button", class: "dd-action" }, [
    "copy invite link",
  ]);
  copyBtn.addEventListener("click", () => flashCopy(copyBtn, "copy invite link"));

  // One-click terminal join for this exact room (carries room[:password]).
  const roomArg = parsed.password ? `${parsed.room}:${parsed.password}` : parsed.room;
  const cliCmd = `curl ${location.origin}/cli | sh -s -- '${roomArg}'`;
  const cliCopy = h("button", { type: "button", class: "dd-cli-copy" }, ["copy"]);
  cliCopy.addEventListener("click", () => {
    void copyText(cliCmd).then((ok) => {
      cliCopy.textContent = ok ? "copied ✓" : "copy failed";
      setTimeout(() => {
        cliCopy.textContent = "copy";
      }, 1500);
    });
  });

  return h("div", { class: "dd-empty" }, [
    h("p", { class: "dd-dim" }, [`#${parsed.room} — no messages yet`]),
    h("p", { class: "dd-dim" }, [
      "share this link to invite (it carries the room key in the # fragment):",
    ]),
    h("div", { class: "dd-invite-url" }, [location.href]),
    copyBtn,
    h("p", { class: "dd-dim" }, ["or join from a terminal:"]),
    h("div", { class: "dd-cli-row" }, [
      h("span", { class: "dd-prompt" }, ["$"]),
      h("code", { class: "dd-cli-cmd" }, [cliCmd]),
      cliCopy,
    ]),
  ]);
}

function flashCopy(btn: HTMLElement, original: string): void {
  void copyText(location.href).then((ok) => {
    btn.textContent = ok ? "copied ✓" : "copy failed";
    setTimeout(() => {
      btn.textContent = original;
    }, 1500);
  });
}
