import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { apiGet, type Source } from "../api";
import { useReadTimer } from "../useReadTimer";

// In-browser PDF viewer. Time spent here is logged as pdf_read events
// against the source id.
export default function SourceViewer() {
  const { id = "" } = useParams();
  useReadTimer("pdf_read", id ? `source:${id}` : "");

  const source = useQuery({
    queryKey: ["source", id],
    queryFn: () => apiGet<Source>(`/api/sources/${id}`),
    enabled: id !== "",
  });

  if (source.isPending) return <p className="text-sm text-zinc-500">Loading…</p>;
  if (source.isError)
    return (
      <div className="space-y-3">
        <p className="text-sm text-red-600">{String(source.error)}</p>
        <Link to="/sources" className="text-sm text-zinc-500 hover:underline">
          ← Back to sources
        </Link>
      </div>
    );

  const s = source.data;
  const fileUrl = `/api/sources/${s.id}/file`;

  return (
    <div className="flex h-[calc(100vh-8rem)] flex-col space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <Link to="/sources" className="text-sm text-zinc-500 hover:underline">
          ← Sources
        </Link>
        <span className="font-medium">{s.title}</span>
        <code className="rounded bg-violet-100 px-1.5 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
          {s.key}
        </code>
        <a
          href={fileUrl}
          target="_blank"
          rel="noreferrer"
          className="ml-auto text-sm text-zinc-500 hover:underline"
        >
          Open in new tab ↗
        </a>
      </div>
      <iframe
        src={fileUrl}
        title={s.title}
        className="w-full flex-1 rounded-lg border border-zinc-200 bg-white dark:border-zinc-800"
      />
    </div>
  );
}
