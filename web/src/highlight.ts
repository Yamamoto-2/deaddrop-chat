// Lazy-loaded syntax highlighting. highlight.js/lib/common bundles a curated
// set of ~35 common languages and is emitted as a SEPARATE chunk — it's only
// fetched the first time a message actually contains a code block, so it never
// weighs down the initial load.

interface Hljs {
  highlightElement: (el: HTMLElement) => void;
}

let hljs: Hljs | null = null;

export async function highlightCode(container: HTMLElement): Promise<void> {
  const blocks = container.querySelectorAll<HTMLElement>("pre code");
  if (blocks.length === 0) return;
  if (!hljs) {
    const mod = await import("highlight.js/lib/common");
    hljs = mod.default as unknown as Hljs;
  }
  blocks.forEach((b) => hljs!.highlightElement(b));
}
