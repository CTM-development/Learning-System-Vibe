import type { MouseEvent } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import { useNavigate } from "react-router-dom";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";
import rehypeHighlight from "rehype-highlight";
import "katex/dist/katex.min.css";
import "highlight.js/styles/github-dark.css";

// Shared markdown renderer: GFM + KaTeX + syntax highlighting.
// - assetBase ("" or "dir/") resolves relative image/link paths against the
//   note's directory via the notes-assets endpoint.
// - App-internal hrefs (from preprocessed [[wikilinks]]) navigate client-side;
//   red links (/wiki?topic=…) are styled as such.
export default function Markdown({
  children,
  assetBase,
}: {
  children: string;
  assetBase?: string;
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
      rehypePlugins={[rehypeKatex, rehypeHighlight]}
      urlTransform={urlTransform}
      components={{
        a: ({ href = "", children: linkChildren, ...props }) => {
          const red = href.startsWith("/wiki?topic=");
          return (
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
        },
      }}
    >
      {children}
    </ReactMarkdown>
  );
}
