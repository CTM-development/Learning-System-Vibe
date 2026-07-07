import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiGet, apiPost, type Source } from "../api";

export const kindIcons: Record<Source["kind"], string> = {
  pdf: "📄",
  url: "🔗",
  book: "📕",
  scan: "📷",
};

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

      <ScanUpload />

      <AddReference />

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
              <span aria-hidden>{kindIcons[s.kind] ?? "📄"}</span>
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

// AddReference registers a citable URL or book without a file: it gets a
// citation key for note frontmatter just like an uploaded PDF.
function AddReference() {
  const queryClient = useQueryClient();
  const [kind, setKind] = useState<"url" | "book">("url");
  const [title, setTitle] = useState("");
  const [url, setUrl] = useState("");
  const [key, setKey] = useState("");

  const create = useMutation({
    mutationFn: () =>
      apiPost<Source>("/api/sources", {
        kind,
        title,
        url: kind === "url" ? url : undefined,
        key: key || undefined,
      }),
    onSuccess: () => {
      setTitle("");
      setUrl("");
      setKey("");
      queryClient.invalidateQueries({ queryKey: ["sources"] });
    },
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (title.trim()) create.mutate();
      }}
      className="flex flex-wrap items-center gap-2 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900"
    >
      <select
        value={kind}
        onChange={(e) => setKind(e.target.value as "url" | "book")}
        className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      >
        <option value="url">URL</option>
        <option value="book">Book</option>
      </select>
      <input
        type="text"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder={kind === "book" ? "Title, author, year" : "Title"}
        className="w-64 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />
      {kind === "url" && (
        <input
          type="url"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://…"
          className="w-64 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />
      )}
      <input
        type="text"
        value={key}
        onChange={(e) => setKey(e.target.value)}
        placeholder="Citation key (optional)"
        className="w-44 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />
      <button
        type="submit"
        disabled={create.isPending || !title.trim()}
        className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
      >
        {create.isPending ? "Adding…" : "Add reference"}
      </button>
      {create.isError && (
        <span className="text-sm text-red-600">{String(create.error)}</span>
      )}
    </form>
  );
}

const MAX_DIMENSION = 1600;
const JPEG_QUALITY = 0.85;
const SKIP_THRESHOLD_BYTES = 1.5 * 1024 * 1024;
const alreadyWebFriendly = new Set(["image/jpeg", "image/png", "image/webp"]);

// Downscales a page photo to keep phone-camera uploads (often 4000px+, many
// MB) off the wire. Falls back to the original file if decoding fails —
// the server rejects unsupported formats itself.
async function prepareScanPage(file: File): Promise<File> {
  if (
    alreadyWebFriendly.has(file.type) &&
    file.size <= SKIP_THRESHOLD_BYTES
  ) {
    let withinBounds = true;
    try {
      const bitmap = await createImageBitmap(file);
      withinBounds = bitmap.width <= MAX_DIMENSION && bitmap.height <= MAX_DIMENSION;
      bitmap.close();
    } catch {
      return file;
    }
    if (withinBounds) return file;
  }

  try {
    const bitmap = await createImageBitmap(file);
    const scale = Math.min(1, MAX_DIMENSION / Math.max(bitmap.width, bitmap.height));
    const width = Math.round(bitmap.width * scale);
    const height = Math.round(bitmap.height * scale);
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext("2d");
    if (!ctx) return file;
    ctx.drawImage(bitmap, 0, 0, width, height);
    bitmap.close();
    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, "image/jpeg", JPEG_QUALITY),
    );
    if (!blob) return file;
    const name = file.name.replace(/\.\w+$/, "") + ".jpg";
    return new File([blob], name, { type: "image/jpeg" });
  } catch {
    return file;
  }
}

// ScanUpload turns a burst of page photos into one "scan" source. Pages are
// downscaled client-side before upload; capture="environment" lets a phone
// on the LAN shoot pages directly into the form.
function ScanUpload() {
  const queryClient = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);
  const [title, setTitle] = useState("");
  const [error, setError] = useState("");
  const [processing, setProcessing] = useState(false);

  const upload = useMutation({
    mutationFn: async () => {
      const files = fileInput.current?.files;
      if (!files || files.length === 0) throw new Error("Choose page photos first.");
      setProcessing(true);
      let pages: File[];
      try {
        pages = await Promise.all(Array.from(files).map(prepareScanPage));
      } finally {
        setProcessing(false);
      }
      const form = new FormData();
      for (const page of pages) form.append("pages", page);
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
        accept="image/*"
        capture="environment"
        multiple
        className="text-sm file:mr-3 file:rounded-md file:border-0 file:bg-zinc-100 file:px-3 file:py-1.5 file:text-sm file:font-medium dark:file:bg-zinc-800"
      />
      <input
        type="text"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder="Scan title (optional)"
        className="w-56 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />
      <button
        type="submit"
        disabled={upload.isPending}
        className="rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
      >
        {upload.isPending ? (processing ? "Processing…" : "Uploading…") : "Upload scan pages"}
      </button>
      {error && <span className="text-sm text-red-600">{error}</span>}
    </form>
  );
}
