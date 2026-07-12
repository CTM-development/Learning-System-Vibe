import { isValidElement, type MouseEvent, type ReactNode } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import { useNavigate } from "react-router-dom";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";
import rehypeHighlight from "rehype-highlight";
import rehypeSlug from "rehype-slug";
import "katex/dist/katex.min.css";
import "highlight.js/styles/github-dark.css";
import MermaidDiagram from "./components/MermaidDiagram";
import HoverCard from "./components/HoverCard";
import NotePreview from "./components/NotePreview";

// Obsidian renders $$…$$ as display math even when content shares a line
// with the delimiters ("$$x$$", "$$x\ny$$"). remark-math only recognizes a
// math block when each $$ sits alone on its line — otherwise it treats the
// first line as discarded "meta" and an end-of-line closing $$ doesn't
// close the block, swallowing the rest of the document. Rewrite such
// blocks to the strict form, leaving code fences and inline code alone.
function normalizeMathBlocks(md: string): string {
  return md.replace(
    /(```|~~~)[\s\S]*?(?:\n\1[ \t]*(?=\n|$)|$)|`[^`\n]*`|\$\$([\s\S]+?)\$\$/g,
    (match, _fence, math: string | undefined, offset: number) => {
      if (math === undefined) return match;
      // Keep the opening line's indentation (list nesting); math that
      // starts mid-line gets pushed onto its own line instead.
      const lineStart = md.lastIndexOf("\n", offset - 1) + 1;
      const prefix = md.slice(lineStart, offset);
      const indent = /^[ \t]*$/.test(prefix) ? prefix : "";
      const open = indent === prefix ? "" : "\n";
      const next = md[offset + match.length];
      const close = next === undefined || next === "\n" ? "" : "\n";
      const body = math.trim().replace(/\n[ \t]*/g, `\n${indent}`);
      return `${open}$$\n${indent}${body}\n${indent}$$${close}`;
    },
  );
}

// Recursively collects the text content of a react-markdown-rendered code
// element, so we can hand mermaid the raw diagram source.
function collectText(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(collectText).join("");
  if (isValidElement(node)) {
    return collectText((node.props as { children?: ReactNode }).children);
  }
  return "";
}

// Shared markdown renderer: GFM + KaTeX + syntax highlighting.
// - assetBase ("" or "dir/") resolves relative image/link paths against the
//   note's directory via the notes-assets endpoint.
// - App-internal hrefs (from preprocessed [[wikilinks]]) navigate client-side;
//   red links (/wiki?topic=…) are styled as such.
// - disableHoverPreviews suppresses wikilink hover cards; used inside a
//   NotePreview's own Markdown so previews don't chain into previews.
export default function Markdown({
  children,
  assetBase,
  disableHoverPreviews,
}: {
  children: string;
  assetBase?: string;
  disableHoverPreviews?: boolean;
}) {
  const navigate = useNavigate();

  const urlTransform = (url: string) => {
    if (
      assetBase !== undefined &&
      !/^([a-z][a-z0-9+.-]*:|\/|#)/i.test(url)
    ) {
      url = `/api/notes-assets/${assetBase}${url}`;
    }
    return defaultUrlTransform(url);
  };

  const onLinkClick = (e: MouseEvent<HTMLAnchorElement>, href: string) => {
    // SPA routes stay in the app; everything else keeps browser behavior.
    if (href.startsWith("/") && !href.startsWith("/api/")) {
      e.preventDefault();
      navigate(href);
    }
  };

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[
        rehypeKatex,
        // Heading ids use github-slugger — the same slugger noteBody.ts
        // uses to build [[note#Heading]] fragments, so anchors line up.
        rehypeSlug,
        [rehypeHighlight, { plainText: ["mermaid"] }],
      ]}
      urlTransform={urlTransform}
      components={{
        a: ({ href = "", children: linkChildren, ...props }) => {
          const red = href.startsWith("/wiki?topic=");
          const link = (
            <a
              {...props}
              href={href}
              onClick={(e) => onLinkClick(e, href)}
              className={
                red
                  ? "text-red-600 decoration-dotted hover:text-red-500 dark:text-red-400"
                  : undefined
              }
              title={red ? "No note yet — generate a wiki article" : undefined}
            >
              {linkChildren}
            </a>
          );
          if (!disableHoverPreviews && href.startsWith("/notes/")) {
            // Drop any #heading fragment: the API path is fragment-free and
            // the query key must match NoteReader's for cache sharing.
            const path = decodeURIComponent(
              href.slice("/notes/".length).split("#")[0],
            );
            return (
              <HoverCard content={<NotePreview path={path} />}>
                {link}
              </HoverCard>
            );
          }
          return link;
        },
        // A <div> (mermaid's rendered SVG wrapper) can't legally sit inside a
        // <pre>, so mermaid code blocks are rendered as their own component
        // instead of the default <pre><code> pair.
        pre: ({ children, ...props }) => {
          const child = Array.isArray(children) ? children[0] : children;
          const className = isValidElement(child)
            ? (child.props as { className?: string }).className
            : undefined;
          if (className?.includes("language-mermaid")) {
            const code = collectText(
              (child as { props: { children?: ReactNode } }).props.children,
            ).replace(/\n$/, "");
            return <MermaidDiagram code={code} />;
          }
          return <pre {...props}>{children}</pre>;
        },
      }}
    >
      {normalizeMathBlocks(children)}
    </ReactMarkdown>
  );
}
