import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, type Source } from "../api";

export default function Sources() {
  const queryClient = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");

  const sources = useQuery({
    queryKey: ["sources"],
    queryFn: () => apiGet<Source[]>("/api/sources"),
  });

  const upload = useMutation({
    mutationFn: async () => {
      const file = fileInput.current?.files?.[0];
      if (!file) throw new Error("Choose a PDF first.");
      const form = new FormData();
      form.append("file", file);
      if (title) form.append("title", title);
      const res = await fetch("/api/sources", { method: "POST", body: form });
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { error?: string };
        throw new ApiError(res.status, body.error ?? res.statusText);
      }
      return res.json() as Promise<Source>;
    },
    onSuccess: () => {
      setTitle("");
      setError("");
      if (fileInput.current) fileInput.current.value = "";
      queryClient.invalidateQueries({ queryKey: ["sources"] });
    },
    onError: (e) => setError(String(e)),
  });

  return (
    <div className="space-y-6">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          upload.mutate();
        }}
        className="flex flex-wrap items-center gap-2 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900"
      >
        <input
          ref={fileInput}
          type="file"
          accept="application/pdf,.pdf"
          className="text-sm file:mr-3 file:rounded-md file:border-0 file:bg-zinc-100 file:px-3 file:py-1.5 file:text-sm file:font-medium dark:file:bg-zinc-800"
        />
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Title (optional)"
          className="w-56 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />
        <button
          type="submit"
          disabled={upload.isPending}
          className="rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          {upload.isPending ? "Uploading…" : "Upload PDF"}
        </button>
        {error && <span className="text-sm text-red-600">{error}</span>}
      </form>

      {sources.isPending && <p className="text-sm text-zinc-500">Loading…</p>}
      {sources.isError && (
        <p className="text-sm text-red-600">{String(sources.error)}</p>
      )}

      {sources.data && sources.data.length === 0 && (
        <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center text-sm text-zinc-500 dark:border-zinc-700">
          No sources yet. Upload a PDF, then cite it from a note's frontmatter
          with <code className="rounded bg-zinc-100 px-1 dark:bg-zinc-800">sources: [its-key]</code>.
        </div>
      )}

      <ul className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 bg-white dark:divide-zinc-800 dark:border-zinc-800 dark:bg-zinc-900">
        {sources.data?.map((s) => (
          <li key={s.id}>
            <Link
              to={`/sources/${s.id}`}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
            >
              <span aria-hidden>📄</span>
              <span className="font-medium">{s.title}</span>
              <code className="rounded bg-violet-100 px-1.5 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
                {s.key}
              </code>
              <span className="ml-auto text-xs text-zinc-400">
                {new Date(s.added_at).toLocaleDateString()}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
