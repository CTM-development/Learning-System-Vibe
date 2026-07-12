import { slug } from "github-slugger";
import type { NoteLink } from "./api";

// headingFragment turns "[[target#Some Heading]]"'s heading part into the
// "#some-heading" URL fragment. github-slugger is what rehype-slug uses to
// id headings in Markdown.tsx, so the two sides always agree.
function headingFragment(heading: string): string {
  return `#${slug(heading.trim())}`;
}

// Reading view: frontmatter is shown as chips instead; srs ID anchors are
// plumbing; cloze markers collapse to their visible text; wikilinks become
// real links — to the note (with a heading anchor when given) when
// resolved, to wiki generation when red.
export function renderableNoteBody(content: string, links: NoteLink[]): string {
  const linkTargets = new Map(links.map((l) => [l.target, l.to_path]));
  return content
    .replace(/^---\n[\s\S]*?\n---\n?/, "")
    .replace(/\s*<!--\s*srs:[0-9a-f]+\s*-->/g, "")
    .replace(/\{\{c\d+::(.*?)(?:::.*?)?\}\}/g, "$1")
    .replace(
      /\[\[([^[\]|]+)(?:\|([^[\]]*))?\]\]/g,
      (whole, target: string, label?: string) => {
        const t = target.trim();
        const text = (label ?? "").trim() || t;
        // [[#Heading]] stays within the current note; the backend never
        // resolves pure-fragment targets, so handle them before the lookup.
        if (t.startsWith("#")) return `[${text}](${headingFragment(t.slice(1))})`;
        const to = linkTargets.get(t);
        if (to) {
          const hash = t.indexOf("#");
          const frag = hash >= 0 ? headingFragment(t.slice(hash + 1)) : "";
          return `[${text}](/notes/${to}${frag})`;
        }
        if (to === "") return `[${text}](/wiki?topic=${encodeURIComponent(t)})`;
        return whole; // inside a code fence — parser didn't record it
      },
    );
}
