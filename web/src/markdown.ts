import MarkdownIt from "markdown-it";
import DOMPurify from "dompurify";

// Markdown with autolinking and single-newline line breaks. Raw HTML is disabled
// (html:false), and DOMPurify sanitizes the output as defense in depth — message
// text comes from untrusted peers.
const md = new MarkdownIt({ html: false, linkify: true, breaks: true });

export function renderMarkdown(text: string): string {
  return DOMPurify.sanitize(md.render(text));
}
