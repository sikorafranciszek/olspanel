import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, KeyRound, Plus, Trash2, UserPlus } from "lucide-react";
import { del, get, post, put, errorMessage } from "../../lib/api";
import type { Database, DBUser } from "../../lib/types";
import { useAuth } from "../../lib/auth";
import { useToast } from "../../components/toast";
import { Button, Card, Confirm, Field, Input, Modal, PageHeader, Spinner, formatDate } from "../../components/ui";

export default function UserDatabases() {
  const qc = useQueryClient();
  const toast = useToast();
  const { user } = useAuth();
  const list = useQuery({ queryKey: ["databases"], queryFn: () => get<Database[]>("/databases") });
  const [create, setCreate] = useState(false);
  const [suffix, setSuffix] = useState("");
  const [addUser, setAddUser] = useState<Database | null>(null);
  const [userForm, setUserForm] = useState({ suffix: "", password: "" });
  const [pw, setPw] = useState<{ db: Database; u: DBUser } | null>(null);
  const [newPw, setNewPw] = useState("");
  const [removeDb, setRemoveDb] = useState<Database | null>(null);
  const [removeUser, setRemoveUser] = useState<{ db: Database; u: DBUser } | null>(null);
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["databases"] });
    qc.invalidateQueries({ queryKey: ["usage"] });
  };
  const onErr = (e: unknown) => toast.error(errorMessage(e));

  const createM = useMutation({ mutationFn: () => post<Database>("/databases", { suffix }), onSuccess: () => { toast.success("Baza utworzona"); setCreate(false); setSuffix(""); invalidate(); }, onError: onErr });
  const addUserM = useMutation({ mutationFn: (dbId: number) => post<DBUser>(`/databases/${dbId}/users`, userForm), onSuccess: () => { toast.success("Użytkownik bazy utworzony"); setAddUser(null); setUserForm({ suffix: "", password: "" }); invalidate(); }, onError: onErr });
  const pwM = useMutation({ mutationFn: ({ db, u }: { db: Database; u: DBUser }) => put(`/databases/${db.id}/users/${u.id}`, { password: newPw }), onSuccess: () => { toast.success("Hasło zmienione"); setPw(null); setNewPw(""); }, onError: onErr });
  const removeDbM = useMutation({ mutationFn: (id: number) => del(`/databases/${id}`), onSuccess: () => { toast.success("Baza usunięta"); setRemoveDb(null); invalidate(); }, onError: onErr });
  const removeUserM = useMutation({ mutationFn: ({ db, u }: { db: Database; u: DBUser }) => del(`/databases/${db.id}/users/${u.id}`), onSuccess: () => { toast.success("Użytkownik bazy usunięty"); setRemoveUser(null); invalidate(); }, onError: onErr });

  const prefix = `${user?.username}_`;
  const submitCreate = (e: FormEvent) => { e.preventDefault(); createM.mutate(); };
  const submitUser = (e: FormEvent) => { e.preventDefault(); if (addUser) addUserM.mutate(addUser.id); };

  return (
    <>
      <PageHeader title="Bazy danych" subtitle="Bazy MariaDB i ich użytkownicy" actions={<><a href="/phpmyadmin/" target="_blank" rel="noreferrer"><Button variant="secondary"><ExternalLink className="h-4 w-4" /> phpMyAdmin</Button></a><Button onClick={() => setCreate(true)}><Plus className="h-4 w-4" /> Nowa baza</Button></>} />
      {list.isLoading ? (
        <Spinner />
      ) : list.data?.length === 0 ? (
        <Card><p className="text-sm text-slate-500">Nie masz jeszcze żadnej bazy danych.</p></Card>
      ) : (
        <div className="space-y-4">
          {list.data?.map((db) => (
            <Card key={db.id} title={<span className="font-mono">{db.name}</span>} actions={<><Button size="sm" variant="secondary" onClick={() => { setAddUser(db); setUserForm({ suffix: "", password: "" }); }}><UserPlus className="h-3.5 w-3.5" /> Dodaj użytkownika</Button><Button size="sm" variant="ghost" onClick={() => setRemoveDb(db)}><Trash2 className="h-4 w-4 text-red-600" /></Button></>}>
              <div className="mb-3 text-xs text-slate-500">Host: <code>localhost</code> · utworzono {formatDate(db.created_at)}</div>
              {db.users.length === 0 ? (
                <p className="text-sm text-slate-500">Brak użytkowników – dodaj użytkownika, aby połączyć się z bazą.</p>
              ) : (
                <ul className="divide-y divide-slate-100 text-sm">
                  {db.users.map((u) => (
                    <li key={u.id} className="flex items-center justify-between py-2">
                      <span className="font-mono">{u.username}</span>
                      <span className="flex gap-1">
                        <Button size="sm" variant="ghost" title="Zmień hasło" onClick={() => { setPw({ db, u }); setNewPw(""); }}><KeyRound className="h-4 w-4" /></Button>
                        <Button size="sm" variant="ghost" title="Usuń" onClick={() => setRemoveUser({ db, u })}><Trash2 className="h-4 w-4 text-red-600" /></Button>
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          ))}
        </div>
      )}

      <Modal open={create} onClose={() => setCreate(false)} title="Nowa baza danych" footer={<><Button variant="secondary" onClick={() => setCreate(false)}>Anuluj</Button><Button type="submit" form="db-form" loading={createM.isPending}>Utwórz</Button></>}>
        <form id="db-form" onSubmit={submitCreate}>
          <Field label="Nazwa bazy" hint="małe litery, cyfry i _">
            <div className="flex items-center"><span className="rounded-l-lg border border-r-0 border-slate-300 bg-slate-50 px-3 py-2 font-mono text-sm text-slate-500">{prefix}</span><Input value={suffix} onChange={(e) => setSuffix(e.target.value)} className="rounded-l-none" required pattern="[a-z0-9_]{1,24}" autoFocus /></div>
          </Field>
        </form>
      </Modal>
      <Modal open={!!addUser} onClose={() => setAddUser(null)} title={`Nowy użytkownik bazy ${addUser?.name ?? ""}`} footer={<><Button variant="secondary" onClick={() => setAddUser(null)}>Anuluj</Button><Button type="submit" form="dbu-form" loading={addUserM.isPending}>Utwórz</Button></>}>
        <form id="dbu-form" onSubmit={submitUser} className="space-y-4">
          <Field label="Nazwa użytkownika">
            <div className="flex items-center"><span className="rounded-l-lg border border-r-0 border-slate-300 bg-slate-50 px-3 py-2 font-mono text-sm text-slate-500">{prefix}</span><Input value={userForm.suffix} onChange={(e) => setUserForm({ ...userForm, suffix: e.target.value })} className="rounded-l-none" required pattern="[a-z0-9_]{1,24}" autoFocus /></div>
          </Field>
          <Field label="Hasło" hint="minimum 8 znaków"><Input type="password" value={userForm.password} onChange={(e) => setUserForm({ ...userForm, password: e.target.value })} required minLength={8} /></Field>
        </form>
      </Modal>
      <Modal open={!!pw} onClose={() => setPw(null)} title={`Zmień hasło: ${pw?.u.username ?? ""}`} footer={<><Button variant="secondary" onClick={() => setPw(null)}>Anuluj</Button><Button onClick={() => pw && pwM.mutate(pw)} loading={pwM.isPending} disabled={newPw.length < 8}>Zapisz</Button></>}>
        <Field label="Nowe hasło"><Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} minLength={8} autoFocus /></Field>
      </Modal>
      <Confirm open={!!removeDb} onClose={() => setRemoveDb(null)} onConfirm={() => removeDb && removeDbM.mutate(removeDb.id)} loading={removeDbM.isPending} title="Usuń bazę" text={<>Baza <b>{removeDb?.name}</b> i wszystkie jej dane zostaną bezpowrotnie usunięte.</>} />
      <Confirm open={!!removeUser} onClose={() => setRemoveUser(null)} onConfirm={() => removeUser && removeUserM.mutate(removeUser)} loading={removeUserM.isPending} title="Usuń użytkownika bazy" text={<>Użytkownik <b>{removeUser?.u.username}</b> straci dostęp do bazy.</>} />
    </>
  );
}
