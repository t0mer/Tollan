import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Pencil, Plus, Trash2 } from "lucide-react";
import { users, type User } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Modal, Field, inputClass, selectClass } from "@/components/ui/modal";

const ROLES = ["admin", "editor", "viewer"];

export function UsersPage() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["users"], queryFn: users.list });
  const [editing, setEditing] = useState<Partial<User> | null>(null);
  const del = useMutation({
    mutationFn: (id: string) => users.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Link to="/system" className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="size-5" />
          </Link>
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">Users & roles</h1>
            <p className="mt-1 text-sm text-muted-foreground">Local accounts and their permissions.</p>
          </div>
        </div>
        <Button onClick={() => setEditing({ role: "viewer" })}>
          <Plus className="size-4" /> New user
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th className="p-3 font-medium">Username</th>
                <th className="p-3 font-medium">Role</th>
                <th className="p-3" />
              </tr>
            </thead>
            <tbody>
              {list.data?.map((u) => (
                <tr key={u.id} className="border-b border-border last:border-0">
                  <td className="p-3 font-medium">{u.username}</td>
                  <td className="p-3">
                    <Badge variant={u.role === "admin" ? "default" : "muted"}>{u.role}</Badge>
                  </td>
                  <td className="p-3 text-right">
                    <button onClick={() => setEditing(u)} className="rounded p-1 text-muted-foreground hover:bg-muted">
                      <Pencil className="size-4" />
                    </button>
                    <button onClick={() => u.id && del.mutate(u.id)} className="rounded p-1 text-muted-foreground hover:text-destructive">
                      <Trash2 className="size-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>

      {editing && (
        <UserEditor
          initial={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            qc.invalidateQueries({ queryKey: ["users"] });
          }}
        />
      )}
    </div>
  );
}

function UserEditor({ initial, onClose, onSaved }: { initial: Partial<User>; onClose: () => void; onSaved: () => void }) {
  const [username, setUsername] = useState(initial.username ?? "");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState(initial.role ?? "viewer");
  const editing = !!initial.id;
  const save = useMutation({
    mutationFn: async () => {
      if (editing) {
        await users.update(initial.id!, { role, password: password || undefined });
      } else {
        await users.create({ username: username.trim(), password, role });
      }
    },
    onSuccess: onSaved,
  });

  return (
    <Modal title={editing ? "Edit user" : "New user"} onClose={onClose}>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Field label="Username">
          <input className={inputClass} value={username} disabled={editing} onChange={(e) => setUsername(e.target.value)} />
        </Field>
        <Field label={editing ? "New password (blank = unchanged)" : "Password"}>
          <input type="password" className={inputClass} value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
        <Field label="Role">
          <select className={selectClass} value={role} onChange={(e) => setRole(e.target.value)}>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        </Field>
        {save.isError && <p className="text-xs text-destructive">Save failed.</p>}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={save.isPending || (!editing && (!username.trim() || !password))}>
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
