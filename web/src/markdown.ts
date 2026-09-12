import MarkdownIt from "markdown-it";
import DOMPurify from "dompurify";

// Markdown with autolinking and single-newline line breaks. Raw HTML is disabled
// (html:false), and DOMPurify sanitizes the output as defense in depth — message
// text comes from untrusted peers.
const md = new MarkdownIt({ html: false, linkify: true, breaks: true });
// linkify-it v6 (markdown-it 15) stopped linking scheme-less hosts by default;
// keep "example.com" / "www.example.com" clickable as before, since that's how
// people paste links in chat.
md.linkify.set({ fuzzyLink: true });

export function renderMarkdown(text: string): string {
  return DOMPurify.sanitize(md.render(text));
}
