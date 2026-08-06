import { useState, useEffect } from "react";

let listeners: Set<(msg: string | null) => void> = new Set();

export function showToast(msg: string) {
  listeners.forEach((fn) => fn(msg));
}

export default function Toast() {
  const [msg, setMsg] = useState<string | null>(null);
  const [timer, setTimer] = useState<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    const handler = (m: string | null) => {
      setMsg(m);
      if (timer) clearTimeout(timer);
      if (m) {
        const t = setTimeout(() => setMsg(null), 1800);
        setTimer(t);
      }
    };
    listeners.add(handler);
    return () => {
      listeners.delete(handler);
      if (timer) clearTimeout(timer);
    };
  }, [timer]);

  if (!msg) return null;
  return (
    <div className="toast" role="status" aria-live="polite">
      {msg}
    </div>
  );
}
