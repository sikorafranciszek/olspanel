import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { Eye, Pencil, Plus, Trash2, Ban, CheckCircle } from "lucide-react";
import { del, get, post, put, errorMessage } from "../../lib/api";
import type { Me, Package, User } from "../../lib/types";
import { useAuth } from "../../lib/auth";
import { useToast } from "../../components/toast";
import { Badge, Button, Confirm, Empty, Field, Input, Modal, PageHeader, Select, Spinner, Table, Td, Th, formatMB, formatDate } from "../../components/ui";

interface UserForm {
  username: string;
  password: string;
  email: string;
  role: "admin" | "user";
  package_id: number;
}

export default function AdminUsers() {
  const qc = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const { applyMe, user: me } = useAuth();
  const users = useQuery({ queryKey: ["admin", "users"], queryFn: () => get<User[]>("/admin/users") });
  const packages = useQuery({ queryKey: ["admin", "packages"], queryFn: () => get<Package[]>("/admin/packages") });
  const [create, setCreate] = useState(false);
  const [edit, setEdit] = useState<User | null>(null);
  const [remove, setRemove] = useState<User | null>(null);
  const [form, setForm] = useState<UserForm>({ username: "", password: "", email: "", role: "user", package_id: 0 });
  const [editForm, setEditForm] = useState({ email: "", package_id: 0, password: "" });

  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "users"] });
  const createM = useMutation({
    mutationFn: (f: UserForm) => post<User>("/admin/users", f),
    onSuccess: () => {
      toast.success("Użytkownik został utworzony");
      setCreate(false);
      setForm({ username: "", password: "", email: "", role: "user", package_id: 0 });
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const editM = useMutation({
    mutationFn: ({ id, body }: { id: number; body: typeof editForm }) => put<User>(`/admin/users/${id}`, body),
    onSuccess: () => {
      toast.success("Zapisano zmiany");
      setEdit(null);
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const suspendM = useMutation({
    mutationFn: ({ id, suspend }: { id: number; suspend: boolean }) => post(`/admin/users/${id}/${suspend ? "suspend" : "unsuspend"}`),
    onSuccess: () => {
      toast.success("Zaktualizowano status konta");
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const removeM = useMutation({
    mutationFn: (id: number) => del(`/admin/users/${id}`),
    onSuccess: () => {
      toast.success("Użytkownik został usunięty");
      setRemove(null);
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const impersonate = async (u: User) => {
    try {
      const m = await post<Me>(`/admin/users/${u.id}/impersonate`);
      applyMe(m);
      navigate("/");
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const submitCreate = (e: FormEvent) => {
    e.preventDefault();
    createM.mutate(form);
  };
  const openEdit = (u: User) => {
    setEdit(u);
    setEditForm({ email: u.email, package_id: u.package_id, password: "" });
  };

  return (
    <>
      <PageHeader title="Użytkownicy" subtitle="Konta hostingowe i administratorzy" actions={<Button onClick={() => setCreate(true)}><Plus className="h-4 w-4" /> Nowy użytkownik</Button>} />
      {users.isLoading ? (
        <Spinner />
      ) : (
        <Table
          head={<><Th>Użytkownik</Th><Th>Rola</Th><Th>Pakiet</Th><Th>Domeny</Th><Th>Dysk</Th><Th>Status</Th><Th>Utworzono</Th><Th className="text-right">Akcje</Th></>}
          empty={users.data?.length === 0 && <Empty>Brak użytkowników</Empty>}
        >
          {users.data?.map((u) => (
            <tr key={u.id} className="hover:bg-slate-50">
              <Td><div className="font-medium text-slate-900">{u.username}</div><div className="text-xs text-slate-500">{u.email || "–"}</div></Td>
              <Td><Badge tone={u.role === "admin" ? "blue" : "slate"}>{u.role === "admin" ? "admin" : "użytkownik"}</Badge></Td>
              <Td>{u.package_name || "–"}</Td>
              <Td>{u.role === "user" ? u.domain_count : "–"}</Td>
              <Td>{u.role === "user" ? formatMB(u.disk_used_mb) : "–"}</Td>
              <Td>{u.suspended ? <Badge tone="red">zawieszony</Badge> : <Badge tone="green">aktywny</Badge>}</Td>
              <Td className="text-slate-500">{formatDate(u.created_at)}</Td>
              <Td>
                <div className="flex justify-end gap-1">
                  {u.role === "user" && <Button size="sm" variant="ghost" title="Zaloguj jako" onClick={() => impersonate(u)}><Eye className="h-4 w-4" /></Button>}
                  <Button size="sm" variant="ghost" title="Edytuj" onClick={() => openEdit(u)}><Pencil className="h-4 w-4" /></Button>
                  {u.role === "user" && (
                    <Button size="sm" variant="ghost" title={u.suspended ? "Odwieś" : "Zawieś"} onClick={() => suspendM.mutate({ id: u.id, suspend: !u.suspended })}>
                      {u.suspended ? <CheckCircle className="h-4 w-4 text-emerald-600" /> : <Ban className="h-4 w-4 text-amber-600" />}
                    </Button>
                  )}
                  {u.id !== me?.id && <Button size="sm" variant="ghost" title="Usuń" onClick={() => setRemove(u)}><Trash2 className="h-4 w-4 text-red-600" /></Button>}
                </div>
              </Td>
            </tr>
          ))}
        </Table>
      )}

      <Modal open={create} onClose={() => setCreate(false)} title="Nowy użytkownik" footer={<><Button variant="secondary" onClick={() => setCreate(false)}>Anuluj</Button><Button type="submit" form="create-user" loading={createM.isPending}>Utwórz</Button></>}>
        <form id="create-user" onSubmit={submitCreate} className="space-y-4">
          <Field label="Nazwa użytkownika" hint="3–16 znaków: małe litery i cyfry, zaczyna się literą. Będzie to też nazwa konta systemowego.">
            <Input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} required pattern="[a-z][a-z0-9]{2,15}" autoFocus />
          </Field>
          <Field label="Hasło" hint="minimum 8 znaków">
            <Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required minLength={8} />
          </Field>
          <Field label="E-mail">
            <Input type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          </Field>
          <Field label="Rola">
            <Select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as "admin" | "user" })}>
              <option value="user">Użytkownik (konto hostingowe)</option>
              <option value="admin">Administrator</option>
            </Select>
          </Field>
          {form.role === "user" && (
            <Field label="Pakiet">
              <Select value={form.package_id} onChange={(e) => setForm({ ...form, package_id: Number(e.target.value) })} required>
                <option value={0}>– wybierz –</option>
                {packages.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
              </Select>
            </Field>
          )}
        </form>
      </Modal>

      <Modal open={!!edit} onClose={() => setEdit(null)} title={`Edycja: ${edit?.username ?? ""}`} footer={<><Button variant="secondary" onClick={() => setEdit(null)}>Anuluj</Button><Button onClick={() => edit && editM.mutate({ id: edit.id, body: editForm })} loading={editM.isPending}>Zapisz</Button></>}>
        <Field label="E-mail"><Input type="email" value={editForm.email} onChange={(e) => setEditForm({ ...editForm, email: e.target.value })} /></Field>
        {edit?.role === "user" && (
          <Field label="Pakiet">
            <Select value={editForm.package_id} onChange={(e) => setEditForm({ ...editForm, package_id: Number(e.target.value) })}>
              {packages.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
          </Field>
        )}
        <Field label="Nowe hasło" hint="zostaw puste, aby nie zmieniać"><Input type="password" value={editForm.password} onChange={(e) => setEditForm({ ...editForm, password: e.target.value })} /></Field>
      </Modal>

      <Confirm open={!!remove} onClose={() => setRemove(null)} onConfirm={() => remove && removeM.mutate(remove.id)} loading={removeM.isPending} title="Usuń użytkownika" text={<>Konto <b>{remove?.username}</b> zostanie usunięte razem ze wszystkimi domenami, plikami, bazami danych, kontami FTP i zadaniami cron. Tej operacji nie można cofnąć.</>} />
    </>
  );
}
