import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Copy, Plus, Trash2 } from "lucide-react";
import { tokens } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass } from "@/components/ui/modal";

export function TokensPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["tokens"], queryFn: tokens.list });
  const [creating, setCreating] = useState(false);
  const del = useMutation({
    mutationFn: (id: string) => tokens.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tokens"] }),
  });

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Link to="/system" className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="size-5" />
          </Link>
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">API tokens</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Bearer tokens for automation. Send as <code className="font-mono">Authorization: Bearer …</code>
            </p>
          </div>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus className="size-4" /> New token
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Name</th>
                <th className="p-3 font-medium">Created</th>
                <th className="p-3 font-medium">Last used</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {list.data?.map((t) => (
                <tr key={t.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{t.name}</td>
                  <td className="p-3 font-mono text-xs text-muted-foreground">{t.created_at.slice(0, 10)}</td>
                  <td className="p-3 font-mono text-xs text-muted-foreground">
                    {t.last_used && t.last_used.slice(0, 4) !== "0001" ? t.last_used.slice(0, 19).replace("T", " ") : "never"}
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => del.mutate(t.id)} className="rounded p-1 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {list.data && list.data.length === 0 && (
            <div className="p-6 text-center text-sm text-muted-foreground">No tokens yet.</div>
          )}
        </CardContent>
      </Card>

      {creating && (
        <CreateToken
          onClose={() => setCreating(false)}
          onCreated={() => qc.invalidateQueries({ queryKey: ["tokens"] })}
        />
      )}
    </div>
  );
}

function CreateToken({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [created, setCreated] = useState<string | null>(null);
  const create = useMutation({
    mutationFn: () => tokens.create(name.trim()),
    onSuccess: (r) => {
      setCreated(r.token);
      onCreated();
    },
  });

  return (
    <Modal title="New API token" onClose={onClose}>
      {created ? (
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">Copy this token now — it won't be shown again.</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 truncate rounded-md bg-muted px-3 py-2 font-mono text-xs">{created}</code>
            <Button size="icon" variant="outline" onClick={() => navigator.clipboard?.writeText(created)}>
              <Copy className="size-4" />
            </Button>
          </div>
          <div className="flex justify-end">
            <Button onClick={onClose}>Done</Button>
          </div>
        </div>
      ) : (
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <Field label="Name">
            <input autoFocus className={inputClass} value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              Create
            </Button>
          </div>
        </form>
      )}
    </Modal>
  );
}
