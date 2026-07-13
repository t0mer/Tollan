import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LayoutDashboard, Plus, Trash2 } from "lucide-react";
import { dashboards, type Dashboard } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass } from "@/components/ui/modal";

export function DashboardsPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["dashboards"], queryFn: dashboards.list });
  const [creating, setCreating] = useState(false);
  const del = useMutation({
    mutationFn: (id: string) => dashboards.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["dashboards"] }),
  });

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">Dashboards</h1>
          <p className="mt-1 text-sm text-muted-foreground">Compose widgets into shareable views.</p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus className="size-4" /> New dashboard
        </Button>
      </div>

      {list.data && list.data.length === 0 ? (
        <Card>
          <CardContent className="p-10 text-center text-sm text-muted-foreground">
            No dashboards yet. Create one to start adding widgets.
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {list.data?.map((d) => (
            <Card key={d.id} className="group relative">
              <Link to={`/dashboards/${d.id}`}>
                <CardContent className="flex items-center gap-3 p-5">
                  <span className="grid size-10 place-items-center rounded-md bg-accent text-accent-foreground">
                    <LayoutDashboard className="size-5" />
                  </span>
                  <div className="min-w-0">
                    <div className="truncate font-medium">{d.name}</div>
                    <div className="text-xs text-muted-foreground">
                      {d.widgets?.length ?? 0} widget{(d.widgets?.length ?? 0) !== 1 ? "s" : ""}
                    </div>
                  </div>
                </CardContent>
              </Link>
              <button
                onClick={() => d.id && del.mutate(d.id)}
                className="absolute right-2 top-2 rounded p-1 text-muted-foreground opacity-0 hover:text-destructive group-hover:opacity-100"
              >
                <Trash2 className="size-4" />
              </button>
            </Card>
          ))}
        </div>
      )}

      {creating && (
        <CreateDialog
          onClose={() => setCreating(false)}
          onCreated={() => {
            setCreating(false);
            qc.invalidateQueries({ queryKey: ["dashboards"] });
          }}
        />
      )}
    </div>
  );
}

function CreateDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: () =>
      dashboards.create({ name: name.trim(), widgets: [], time_range: "now-24h", refresh_seconds: 60 } as Dashboard),
    onSuccess: onCreated,
  });
  return (
    <Modal title="New dashboard" onClose={onClose}>
      <form
        className="space-y-4"
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
    </Modal>
  );
}
