import { useQuery } from "@tanstack/react-query";
import { apiGet, type NoteDetail } from "../api";
import Markdown from "../Markdown";
import { renderableNoteBody } from "../noteBody";

// Hover-card body for a wikilink: reuses NoteReader's exact query key/fn so
// the cache is shared — hovering warms the note, and a follow-up click into
// NoteReader renders instantly.
export default function NotePreview({ path }: { path: string }) {
  const note = useQuery({
    queryKey: ["note", path],
    queryFn: () => apiGet<NoteDetail>(`/api/notes/${path}`),
  });

  if (note.isPending) {
    return <p className="text-xs text-zinc-500">Loading…</p>;
  }
  if (note.isError) {
    return <p className="text-xs text-red-600">{String(note.error)}</p>;
  }

  const n = note.data;
  const assetBase = path.includes("/")
    ? path.slice(0, path.lastIndexOf("/") + 1)
    : "";

  return (
    <div className="relative max-h-72 overflow-hidden">
      <p className="text-sm font-medium">{n.title}</p>
      <div className="prose prose-sm dark:prose-invert max-w-none">
        <Markdown assetBase={assetBase} disableHoverPreviews>
          {renderableNoteBody(n.content, n.links).slice(0, 1500)}
        </Markdown>
      </div>
      <div className="absolute inset-x-0 bottom-0 h-8 bg-gradient-to-t from-white dark:from-zinc-900" />
    </div>
  );
}
