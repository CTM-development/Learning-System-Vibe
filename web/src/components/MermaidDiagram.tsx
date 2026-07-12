import { useEffect, useId, useState } from "react";

// Lazily-loaded mermaid module, shared across all diagram instances so the
// (sizeable) mermaid chunk is fetched once and initialized exactly once.
let mermaidLoader: Promise<typeof import("mermaid").default> | null = null;

function loadMermaid() {
  if (!mermaidLoader) {
    mermaidLoader = import("mermaid").then((m) => {
      const mermaid = m.default;
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "neutral",
      });
      return mermaid;
    });
  }
  return mermaidLoader;
}

// Renders a fenced ```mermaid code block as an SVG diagram. Falls back to
// the raw source while the (lazy-loaded) mermaid chunk is fetched, and to a
// visible error box if the diagram source doesn't parse.
export default function MermaidDiagram({ code }: { code: string }) {
  const rawId = useId().replace(/[^a-zA-Z0-9]/g, "");
  const id = `mmd${rawId}`;
  const [svg, setSvg] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setSvg(null);
    setError(null);
    loadMermaid()
      .then((mermaid) => mermaid.render(id, code))
      .then(({ svg }) => {
        if (!cancelled) setSvg(svg);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [code]);

  if (error) {
    return (
      <div className="border border-red-300 rounded p-3 text-sm text-red-600 dark:border-red-800 dark:text-red-400">
        <p>{error}</p>
        <pre className="mt-2 overflow-x-auto text-xs">{code}</pre>
      </div>
    );
  }

  if (!svg) {
    return <pre className="text-xs text-zinc-400">{code}</pre>;
  }

  return (
    <div
      className="my-4 flex justify-center overflow-x-auto"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
