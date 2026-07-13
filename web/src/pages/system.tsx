import { useState } from "react";
import { Link } from "react-router-dom";
import { Bell, Download, KeyRound, Package, Send, Upload, Users } from "lucide-react";
import { contentPacks, type ImportResult } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";

export function SystemPage() {
  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight">System</h1>
        <p className="mt-1 text-sm text-muted-foreground">Outputs, notifications, access control and content packs.</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <NavCard to="/outputs" icon={<Send className="size-5" />} title="Outputs" desc="Forward messages downstream." />
        <NavCard to="/notifications" icon={<Bell className="size-5" />} title="Notification channels" desc="Slack, Telegram, WhatsApp, email…" />
        <NavCard to="/users" icon={<Users className="size-5" />} title="Users & roles" desc="Local accounts and permissions." />
        <NavCard to="/tokens" icon={<KeyRound className="size-5" />} title="API tokens" desc="Programmatic access." />
      </div>

      <ContentPacks />
    </div>
  );
}

function NavCard({ to, icon, title, desc }: { to: string; icon: React.ReactNode; title: string; desc: string }) {
  return (
    <Link to={to}>
      <Card className="transition-colors hover:border-primary/50">
        <CardContent className="flex items-center gap-3 p-5">
          <span className="grid size-10 place-items-center rounded-md bg-accent text-accent-foreground">{icon}</span>
          <div>
            <div className="font-medium">{title}</div>
            <div className="text-xs text-muted-foreground">{desc}</div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

function ContentPacks() {
  const [result, setResult] = useState<ImportResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<unknown>(null);

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    setResult(null);
    setError(null);
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const bundle = JSON.parse(await file.text());
      setPending(bundle);
      setResult(await contentPacks.import(bundle, true)); // dry-run diff
    } catch (err) {
      setError(String(err));
    }
    e.target.value = "";
  }

  async function apply() {
    if (!pending) return;
    try {
      await contentPacks.import(pending, false);
      setResult(null);
      setPending(null);
      setError("Imported ✓");
    } catch (err) {
      setError(String(err));
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Package className="size-5" /> Content packs
        </CardTitle>
        <CardDescription>Export or import streams, pipelines, dashboards, events and outputs.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline">
            <a href={contentPacks.exportUrl()} download>
              <Download className="size-4" /> Export
            </a>
          </Button>
          <label className="inline-flex cursor-pointer items-center gap-2 rounded-md border border-border px-4 py-2 text-sm font-medium hover:bg-muted">
            <Upload className="size-4" /> Import…
            <input type="file" accept="application/json" className="hidden" onChange={onFile} />
          </label>
        </div>

        {error && <p className="text-xs text-muted-foreground">{error}</p>}

        {result && (
          <div className="rounded-md border border-border p-3">
            <div className="mb-2 text-sm font-medium">Preview ({result.changes.length} changes)</div>
            <ul className="max-h-48 space-y-1 overflow-y-auto font-mono text-xs">
              {result.changes.map((c, i) => (
                <li key={i} className="flex justify-between">
                  <span>
                    {c.kind}: {c.name}
                  </span>
                  <span className={c.action === "create" ? "text-success" : "text-warning"}>{c.action}</span>
                </li>
              ))}
            </ul>
            <div className="mt-3 flex justify-end">
              <Button size="sm" onClick={apply}>
                Apply import
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
