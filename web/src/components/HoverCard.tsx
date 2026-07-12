import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

// Generic hover-triggered popover (no external library). The trigger is an
// inline <span>; the card itself is portaled to document.body so it can be
// position: fixed against the viewport without inheriting any ancestor's
// stacking/transform context, and without nesting block content inside the
// inline trigger.
export default function HoverCard({
  children,
  content,
  openDelay = 350,
  closeDelay = 150,
}: {
  children: ReactNode;
  content: ReactNode;
  openDelay?: number;
  closeDelay?: number;
}) {
  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState<{ top?: number; bottom?: number; left: number }>({
    left: 0,
  });
  const triggerRef = useRef<HTMLSpanElement>(null);
  const openTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (openTimer.current) clearTimeout(openTimer.current);
      if (closeTimer.current) clearTimeout(closeTimer.current);
    },
    [],
  );

  const cancelClose = () => {
    if (closeTimer.current) {
      clearTimeout(closeTimer.current);
      closeTimer.current = null;
    }
  };

  const scheduleOpen = () => {
    cancelClose();
    if (openTimer.current) clearTimeout(openTimer.current);
    openTimer.current = setTimeout(() => {
      const rect = triggerRef.current?.getBoundingClientRect();
      if (rect) {
        const cardWidth = 320; // w-80
        const flip = rect.bottom + 320 > window.innerHeight;
        const left = Math.min(
          Math.max(rect.left, 8),
          Math.max(8, window.innerWidth - cardWidth - 8),
        );
        setStyle(
          flip
            ? { bottom: window.innerHeight - rect.top + 8, left }
            : { top: rect.bottom + 8, left },
        );
      }
      setOpen(true);
    }, openDelay);
  };

  const scheduleClose = () => {
    if (openTimer.current) {
      clearTimeout(openTimer.current);
      openTimer.current = null;
    }
    closeTimer.current = setTimeout(() => setOpen(false), closeDelay);
  };

  useEffect(() => {
    if (!open) return;
    // The article body scrolls (not the window), so listen in the capture
    // phase to catch scrolling on any ancestor.
    const onScroll = () => setOpen(false);
    window.addEventListener("scroll", onScroll, true);
    return () => window.removeEventListener("scroll", onScroll, true);
  }, [open]);

  return (
    <span ref={triggerRef} onMouseEnter={scheduleOpen} onMouseLeave={scheduleClose}>
      {children}
      {open &&
        createPortal(
          <div
            onMouseEnter={cancelClose}
            onMouseLeave={scheduleClose}
            style={style}
            className="fixed z-50 w-80 max-h-72 overflow-hidden rounded-lg border border-zinc-200 bg-white p-4 shadow-lg dark:border-zinc-700 dark:bg-zinc-900"
          >
            {content}
          </div>,
          document.body,
        )}
    </span>
  );
}
