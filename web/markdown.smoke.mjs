// Markdown XSS smoke test. Message text comes from untrusted peers, so the
// renderer must neutralize HTML/script vectors. The app uses html:false (no raw
// HTML) plus DOMPurify as defense-in-depth; this test exercises the html:false
// + linkify layer, which is pure JS and needs no DOM.
import MarkdownIt from "markdown-it";

const md = new MarkdownIt({ html: false, linkify: true, breaks: true });

let failed = false;
const fail = (m) => {
  console.error("FAIL: " + m);
  failed = true;
};

// Raw HTML must be escaped, never emitted as live tags.
for (const v of [
  "<script>alert(1)</script>",
  "<img src=x onerror=alert(1)>",
  '<a href="javascript:alert(1)">x</a>',
  "<iframe src=//evil></iframe>",
]) {
  const out = md.render(v);
  if (/<script|<img|<iframe|<a\s/i.test(out)) {
    fail(`raw HTML survived for ${JSON.stringify(v)} -> ${out}`);
  }
}

// A markdown link to a javascript: URL must not become a live href.
for (const v of ["[x](javascript:alert(1))", "javascript:alert(1)"]) {
  const out = md.render(v);
  if (/href=["']\s*javascript:/i.test(out)) {
    fail(`javascript: URL rendered as a live href for ${JSON.stringify(v)} -> ${out}`);
  }
}

if (failed) process.exit(1);
console.log("PASS: markdown neutralizes raw HTML / script / javascript: vectors");
