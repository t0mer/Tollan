import { useCallback, useEffect, useState } from "react";

const KEY = "tollan-search-history";
const MAX = 15;

/** useSearchHistory keeps a de-duplicated, most-recent-first list of past query
 * strings in localStorage. */
export function useSearchHistory() {
  const [items, setItems] = useState<string[]>([]);

  useEffect(() => {
    try {
      const raw = localStorage.getItem(KEY);
      if (raw) setItems(JSON.parse(raw));
    } catch {
      /* ignore malformed storage */
    }
  }, []);

  const push = useCallback((q: string) => {
    const trimmed = q.trim();
    if (!trimmed) return;
    setItems((prev) => {
      const next = [trimmed, ...prev.filter((x) => x !== trimmed)].slice(0, MAX);
      localStorage.setItem(KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  return { items, push };
}
