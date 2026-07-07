import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPost,
  ApiError,
  type CreateNoteRequest,
  type LLMStatus,
  type NoteDetail,
  type NoteType,
  type Source,
  type Stage,
  type TranscribeResponse,
} from "../api";
import ScanPager from "../components/ScanPager";

const stageOptions: { value: Stage; label: string }[] = [
  { value: "", label: "none" },
  { value: "skim", label: "skim" },
  { value: "deep", label: "deep" },
  { value: "synthesis", label: "synthesis" },
];

// Workbench: turn a photographed scan into a reading note (optionally
// AI-drafted from the page images), or write a born-digital Thought.
// /workbench?scan=<source id>&type=thought|reading
export default function Workbench() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const scanParam = params.get("scan");
  const scanId = scanParam ? Number(scanParam) : null;
  const hasScan = scanId !== null && !Number.isNaN(scanId);

  const source = useQuery({
    queryKey: ["source", scanId],
    queryFn: () => apiGet<Source>(`/api/sources/${scanId}`),
    enabled: hasScan,
  });

  const llmStatus = useQuery({
    queryKey: ["llm", "status"],
    queryFn: () => apiGet<LLMStatus>("/api/llm/status"),
  });

  const [title, setTitle] = useState("");
  const [noteType, setNoteType] = useState<NoteType>(
    params.get("type") === "thought" ? "thought" : "reading",
  );
  const [stage, setStage] = useState<Stage>("");
  const [folder, setFolder] = useState("");
  const [tags, setTags] = useState("");
  const [sourcesText, setSourcesText] = useState("");
  const [body, setBody] = useState("");
  const [transcribedBy, setTranscribedBy] = useState("");
  const [saveError, setSaveError] = useState("");

  const sourcesPrefilled = useRef(false);
  useEffect(() => {
    if (source.data && !sourcesPrefilled.current) {
      sourcesPrefilled.current = true;
      setSourcesText(source.data.key);
    }
  }, [source.data]);

  // Editing time is the note's elapsed_ms: starts on the first change to any
  // form field, ends at save.
  const editStart = useRef<number | null>(null);
  const markEdited = () => {
    if (editStart.current === null) editStart.current = Date.now();
  };
  const transcribe = useMutation({
    mutationFn: () =>
      apiPost<TranscribeResponse>("/api/llm/transcribe", { source_id: scanId }),
    onSuccess: (res) => {
      setBody(res.text);
      setTranscribedBy(res.model);
    },
  });

  const handleDraft = () => {
    if (body.trim() && !window.confirm("Replace the current draft with a fresh AI transcription?")) {
      return;
    }
    transcribe.mutate();
  };

  const save = useMutation({
    mutationFn: () => {
      const elapsed_ms = editStart.current !== null ? Date.now() - editStart.current : 0;
      const req: CreateNoteRequest = {
        title: title.trim(),
        type: noteType,
        tags: tags.split(",").map((t) => t.trim()).filter(Boolean),
        sources: sourcesText.split(",").map((s) => s.trim()).filter(Boolean),
        body,
        elapsed_ms,
      };
      if (noteType === "reading") {
        if (stage) req.stage = stage;
        if (folder.trim()) req.folder = folder.trim();
      }
      // Provenance survives hand-correction: the note is still AI-drafted
      // even after the human fixes it (that's the intended flow).
      if (transcribedBy && body.trim()) {
        req.transcribed_by = transcribedBy;
      }
      return apiPost<NoteDetail>("/api/notes", req);
    },
    onSuccess: (note) => {
      queryClient.invalidateQueries({ queryKey: ["notes"] });
      navigate(`/notes/${note.path}`);
    },
    onError: (e) => setSaveError(e instanceof ApiError ? e.message : String(e)),
  });

  const canDraft = hasScan && llmStatus.data?.configured === true;
  const draftDisabledReason = !hasScan
    ? ""
    : llmStatus.data?.configured === false
      ? "Configure openrouter_api_key on the server to enable AI drafting."
      : "";

  return (
    <div className={hasScan ? "grid grid-cols-1 gap-6 lg:grid-cols-2" : "space-y-4"}>
      {hasScan && (
        <div className="space-y-2">
          {source.isPending && <p className="text-sm text-zinc-500">Loading scan…</p>}
          {source.isError && (
            <p className="text-sm text-red-600">{String(source.error)}</p>
          )}
          {source.data && (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{source.data.title}</span>
                <code className="rounded bg-violet-100 px-1.5 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
                  {source.data.key}
                </code>
              </div>
              <ScanPager source={source.data} />
            </>
          )}
        </div>
      )}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (title.trim() && !save.isPending) save.mutate();
        }}
        className="space-y-3"
      >
        <input
          type="text"
          value={title}
          onChange={(e) => {
            markEdited();
            setTitle(e.target.value);
          }}
          placeholder="Title"
          required
          className="w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-base text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
        />

        <div className="flex gap-1">
          {(["reading", "thought"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => {
                markEdited();
                setNoteType(t);
              }}
              className={`rounded-full px-3 py-1 text-sm transition-colors ${
                noteType === t
                  ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900"
                  : "bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:bg-zinc-700"
              }`}
            >
              {t === "reading" ? "Reading note" : "Thought"}
            </button>
          ))}
        </div>

        {noteType === "reading" && (
          <div className="flex flex-wrap gap-2">
            <label className="flex flex-col gap-1 text-xs text-zinc-500">
              Stage
              <select
                value={stage}
                onChange={(e) => {
                  markEdited();
                  setStage(e.target.value as Stage);
                }}
                className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
              >
                {stageOptions.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-1 flex-col gap-1 text-xs text-zinc-500">
              Folder
              <input
                type="text"
                value={folder}
                onChange={(e) => {
                  markEdited();
                  setFolder(e.target.value);
                }}
                placeholder="folder (optional, e.g. analysis)"
                className="rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
              />
            </label>
          </div>
        )}

        <input
          type="text"
          value={tags}
          onChange={(e) => {
            markEdited();
            setTags(e.target.value);
          }}
          placeholder="Tags, comma-separated"
          className="w-full rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />
        <input
          type="text"
          value={sourcesText}
          onChange={(e) => {
            markEdited();
            setSourcesText(e.target.value);
          }}
          placeholder="Source citation keys, comma-separated"
          className="w-full rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />

        {hasScan && (
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={handleDraft}
              disabled={!canDraft || transcribe.isPending}
              title={draftDisabledReason}
              className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {transcribe.isPending ? "Drafting…" : "✨ Draft with AI"}
            </button>
            {transcribedBy && (
              <span className="text-xs text-zinc-400">
                drafted by {transcribedBy} — review against the original before saving
              </span>
            )}
            {transcribe.isError && (
              <span className="text-sm text-red-600">{String(transcribe.error)}</span>
            )}
          </div>
        )}

        <textarea
          value={body}
          onChange={(e) => {
            markEdited();
            setBody(e.target.value);
          }}
          placeholder="Markdown body. Q: / A: pairs and {{c1::cloze}} spans become live cards on save."
          className={`w-full rounded-md border border-zinc-300 bg-white px-3 py-2 font-mono text-sm dark:border-zinc-700 dark:bg-zinc-900 ${
            hasScan ? "min-h-[60vh]" : "min-h-64"
          }`}
        />

        {saveError && <p className="text-sm text-red-600">{saveError}</p>}

        <button
          type="submit"
          disabled={!title.trim() || save.isPending}
          className="rounded-md bg-zinc-900 px-4 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          {save.isPending ? "Saving…" : "Save note"}
        </button>
      </form>
    </div>
  );
}
