import { useMemo } from "react";
import hljs from "highlight.js/lib/common";
import "highlight.js/styles/github-dark.css";

const EXT_LANG: Record<string, string> = {
  rs: "rust",
  go: "go",
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  py: "python",
  sh: "bash",
  bash: "bash",
  json: "json",
  yml: "yaml",
  yaml: "yaml",
  toml: "ini",
  css: "css",
  html: "xml",
  c: "c",
  h: "c",
  cpp: "cpp",
  java: "java",
  sql: "sql",
  lock: "json",
};

/** Syntax-highlighted, line-numbered file view. */
export function CodeBlock({ code, fileName }: { code: string; fileName: string }) {
  const html = useMemo(() => {
    const ext = fileName.split(".").pop()?.toLowerCase() ?? "";
    const lang = EXT_LANG[ext];
    try {
      if (lang && hljs.getLanguage(lang)) {
        return hljs.highlight(code, { language: lang }).value;
      }
      return hljs.highlightAuto(code).value;
    } catch {
      return undefined;
    }
  }, [code, fileName]);

  const lines = code.split("\n").length;
  return (
    <div className="fileview">
      <div className="code-grid">
        <pre className="line-nums" aria-hidden>
          {Array.from({ length: lines }, (_, i) => `${i + 1}\n`).join("")}
        </pre>
        {html !== undefined ? (
          <pre className="hljs code-body" dangerouslySetInnerHTML={{ __html: html }} />
        ) : (
          <pre className="code-body">{code}</pre>
        )}
      </div>
    </div>
  );
}
