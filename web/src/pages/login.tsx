import { useState } from "react";
import { auth } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { inputClass } from "@/components/ui/modal";

/** LoginScreen handles both first-run admin setup and normal login. */
export function LoginScreen() {
  const { needsSetup, refresh } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      if (needsSetup) {
        await auth.setup(username.trim(), password);
      } else {
        await auth.login(username.trim(), password);
      }
      await refresh();
    } catch {
      setError(needsSetup ? "Could not create the admin account." : "Invalid username or password.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid min-h-screen place-items-center bg-background p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center gap-2.5">
          <span className="grid h-9 w-9 place-items-center rounded-md bg-primary text-primary-foreground">
            <span className="font-display text-lg font-bold leading-none">T</span>
          </span>
          <span className="font-display text-xl font-semibold tracking-tight">Tollan</span>
        </div>

        <div className="rounded-lg border border-border bg-card p-6 shadow-sm">
          <h1 className="font-display text-lg font-semibold">
            {needsSetup ? "Create the admin account" : "Sign in"}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {needsSetup ? "This is the first run — set up an administrator." : "Enter your credentials to continue."}
          </p>
          <form className="mt-5 space-y-3" onSubmit={submit}>
            <input
              className={inputClass}
              placeholder="Username"
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
            <input
              className={inputClass}
              type="password"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            {error && <p className="text-xs text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={busy || !username.trim() || !password}>
              {needsSetup ? "Create admin" : "Sign in"}
            </Button>
          </form>
        </div>
      </div>
    </div>
  );
}
