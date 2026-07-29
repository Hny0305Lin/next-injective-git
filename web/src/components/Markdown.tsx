import { useMemo } from "react";
import { marked } from "marked";
import DOMPurify from "dompurify";

/** Renders markdown text, sanitized, GitHub-flavored enough for READMEs. */
export function Markdown({ text }: { text: string }) {
  const html = useMemo(() => {
    const raw = marked.parse(text, { async: false, gfm: true, breaks: false }) as string;
    return DOMPurify.sanitize(raw);
  }, [text]);
  return <div className="markdown-body" dangerouslySetInnerHTML={{ __html: html }} />;
}
