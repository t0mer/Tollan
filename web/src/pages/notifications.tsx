import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Pencil, Plus, Send, Trash2 } from "lucide-react";
import { channels, type Channel } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass, selectClass } from "@/components/ui/modal";

function emptyChannel(): Channel {
  return { name: "", provider: "shoutrrr", enabled: true, notify_on_success: false, notify_on_failure: true };
}

export function NotificationsPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["channels"], queryFn: channels.list });
  const [editing, setEditing] = useState<Channel | null>(null);
  const del = useMutation({
    mutationFn: (id: string) => channels.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["channels"] }),
  });

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">Notification channels</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Where events are delivered. Credentials are encrypted at rest.
          </p>
        </div>
        <Button onClick={() => setEditing(emptyChannel())}>
          <Plus className="size-4" /> New channel
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Name</th>
                <th className="p-3 font-medium">Provider</th>
                <th className="p-3 font-medium">Status</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {list.data?.map((c) => (
                <tr key={c.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{c.name}</td>
                  <td className="p-3 font-mono text-xs">{c.provider}</td>
                  <td className="p-3">
                    <Badge variant={c.enabled ? "success" : "muted"}>{c.enabled ? "enabled" : "disabled"}</Badge>
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => setEditing(c)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                      <Pencil className="size-4" />
                    </button>
                    <button onClick={() => c.id && del.mutate(c.id)} className="rounded p-1 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {list.data && list.data.length === 0 && (
            <div className="p-8 text-center text-sm text-muted-foreground">
              <Bell className="mx-auto mb-2 size-6 opacity-50" />
              No channels yet.
            </div>
          )}
        </CardContent>
      </Card>

      {editing && (
        <ChannelEditor
          initial={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["channels"] });
          }}
        />
      )}
    </div>
  );
}

function ChannelEditor({ initial, onClose, onSaved }: { initial: Channel; onClose: () => void; onSaved: () => void }) {
  const [c, setC] = useState<Channel>(initial);
  const [testMsg, setTestMsg] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () => (c.id ? channels.update(c.id, c) : channels.create(c)),
    onSuccess: onSaved,
  });
  const test = useMutation({
    mutationFn: () => channels.test(c),
    onSuccess: () => setTestMsg("Test sent ✓"),
    onError: (e: Error) => setTestMsg(`Failed: ${e.message}`),
  });

  return (
    <Modal title={c.id ? "Edit channel" : "New channel"} onClose={onClose} wide>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (c.name.trim()) save.mutate();
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Field label="Name">
            <input className={inputClass} value={c.name} onChange={(e) => setC({ ...c, name: e.target.value })} />
          </Field>
          <Field label="Provider">
            <select className={selectClass} value={c.provider} onChange={(e) => setC({ ...c, provider: e.target.value as Channel["provider"] })}>
              <option value="shoutrrr">Shoutrrr</option>
              <option value="greenapi">GreenAPI (WhatsApp)</option>
              <option value="whatsapp_web">WhatsApp Web</option>
            </select>
          </Field>
        </div>

        {c.provider === "shoutrrr" && (
          <Field label="URL (slack://…, telegram://…, smtp://…)">
            <input className={`${inputClass} font-mono`} value={c.url ?? ""} onChange={(e) => setC({ ...c, url: e.target.value })} />
          </Field>
        )}
        {c.provider === "greenapi" && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Instance ID">
              <input className={inputClass} value={c.instance_id ?? ""} onChange={(e) => setC({ ...c, instance_id: e.target.value })} />
            </Field>
            <Field label="Token">
              <input className={inputClass} value={c.token ?? ""} onChange={(e) => setC({ ...c, token: e.target.value })} />
            </Field>
            <Field label="Recipient phone (digits only)">
              <input className={inputClass} placeholder="972501234567" value={c.phone ?? ""} onChange={(e) => setC({ ...c, phone: e.target.value })} />
            </Field>
            <Field label="API URL (optional)">
              <input className={inputClass} placeholder="https://api.green-api.com" value={c.api_url ?? ""} onChange={(e) => setC({ ...c, api_url: e.target.value })} />
            </Field>
          </div>
        )}
        {c.provider === "whatsapp_web" && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Base URL">
              <input className={inputClass} value={c.base_url ?? ""} onChange={(e) => setC({ ...c, base_url: e.target.value })} />
            </Field>
            <Field label="Recipient phone">
              <input className={inputClass} value={c.phone ?? ""} onChange={(e) => setC({ ...c, phone: e.target.value })} />
            </Field>
            <Field label="Username (optional)">
              <input className={inputClass} value={c.username ?? ""} onChange={(e) => setC({ ...c, username: e.target.value })} />
            </Field>
            <Field label="Password (optional)">
              <input type="password" className={inputClass} value={c.password ?? ""} onChange={(e) => setC({ ...c, password: e.target.value })} />
            </Field>
          </div>
        )}

        <div className="flex flex-wrap gap-4 text-sm">
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={c.enabled} onChange={(e) => setC({ ...c, enabled: e.target.checked })} /> Enabled
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={c.notify_on_success} onChange={(e) => setC({ ...c, notify_on_success: e.target.checked })} /> On success
          </label>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={c.notify_on_failure} onChange={(e) => setC({ ...c, notify_on_failure: e.target.checked })} /> On failure
          </label>
        </div>

        {testMsg && <p className="text-xs text-muted-foreground">{testMsg}</p>}
        <div className="flex justify-between gap-2">
          <Button type="button" variant="subtle" onClick={() => test.mutate()} disabled={test.isPending}>
            <Send className="size-4" /> Send test
          </Button>
          <div className="flex gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!c.name.trim() || save.isPending}>
              Save
            </Button>
          </div>
        </div>
      </form>
    </Modal>
  );
}
