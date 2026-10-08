import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { del, get, post, put, errorMessage } from "../../lib/api";
import type { FTPAccount } from "../../lib/types";
import { useAuth } from "../../lib/auth";
import { useToast } from "../../components/toast";
import { Button, Card, Confirm, Empty, Field, Input, Modal, PageHeader, Spinner, Table, Td, Th, formatDate } from "../../components/ui";

export default function UserFtp() {
  const qc = useQueryClient();
  const toast = useToast();
  const { user } = useAuth();
  const list = useQuery({ queryKey: ["ftp"], queryFn: () => get<FTPAccount[]>("/ftp") });
  const [create, setCreate] = useState(false);
  const [form, setForm] = useState({ suffix: "", password: "", home_subdir: "" });
  const [pw, setPw] = useState<FTPAccount | null>(null);
  const [newPw, setNewPw] = useState("");
  const [remove, setRemove] = useState<FTPAccount | null>(null);
  const invalidate = () => { qc.invalidateQueries({ queryKey: ["ftp"] }); qc.invalidateQueries({ queryKey: ["usage"] }); };
  const onErr = (e: unknown) => toast.error(errorMessage(e));
  const createM = useMutation({ mutationFn: () => post<FTPAccount>("/ftp", form), onSuccess: () => { toast.success("Konto FTP utworzone"); setCreate(false); setForm({ suffix: "", password: "", home_subdir: "" }); invalidate(); }, onError: onErr });
  const pwM = useMutation({ mutationFn: (a: FTPAccount) => put(`/ftp/${a.id}`, { password: newPw }), onSuccess: () => { toast.success("Hasło zmienione"); setPw(null); setNewPw(""); }, onError: onErr });
  const removeM = useMutation({ mutationFn: (id: number) => del(`/ftp/${id}`), onSuccess: () => { toast.success("Konto FTP usunięte"); setRemove(null); invalidate(); }, onError: onErr });
  const hasMain = list.data?.some((a) => a.login === user?.username);
  const submit = (e: FormEvent) => { e.preventDefault(); createM.mutate(); };

  return (
    <>
      <PageHeader title="Konta FTP" subtitle="Dostęp do plików przez FTP (z TLS)" actions={<Button onClick={() => setCreate(true)}><Plus className="h-4 w-4" /> Nowe konto FTP</Button>} />
      <Card className="mb-4">
        <dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
          <dt className="text-slate-500">Serwer</dt><dd className="font-mono">{window.location.hostname}</dd>
          <dt className="text-slate-500">Port</dt><dd className="font-mono">21 (tryb pasywny, szyfrowanie TLS – explicit FTPS)</dd>
          <dt className="text-slate-500">Katalog strony</dt><dd className="font-mono">domains/&lt;domena&gt;/public_html</dd>
        </dl>
      </Card>
      {list.isLoading ? <Spinner /> : (
        <Table head={<><Th>Login</Th><Th>Katalog</Th><Th>Utworzono</Th><Th className="text-right">Akcje</Th></>} empty={list.data?.length === 0 && <Empty>Brak kont FTP</Empty>}>
          {list.data?.map((a) => (
            <tr key={a.id} className="hover:bg-slate-50">
              <Td className="font-mono font-medium text-slate-900">{a.login}</Td>
              <Td className="font-mono text-slate-600">/{a.home_subdir || ""}</Td>
              <Td className="text-slate-500">{formatDate(a.created_at)}</Td>
              <Td><div className="flex justify-end gap-1"><Button size="sm" variant="ghost" title="Zmień hasło" onClick={() => { setPw(a); setNewPw(""); }}><KeyRound className="h-4 w-4" /></Button><Button size="sm" variant="ghost" title="Usuń" onClick={() => setRemove(a)}><Trash2 className="h-4 w-4 text-red-600" /></Button></div></Td>
            </tr>
          ))}
        </Table>
      )}
      <Modal open={create} onClose={() => setCreate(false)} title="Nowe konto FTP" footer={<><Button variant="secondary" onClick={() => setCreate(false)}>Anuluj</Button><Button type="submit" form="ftp-form" loading={createM.isPending}>Utwórz</Button></>}>
        <form id="ftp-form" onSubmit={submit} className="space-y-4">
          <Field label="Login" hint={hasMain ? "konto główne już istnieje – podaj przyrostek" : "zostaw puste, aby utworzyć konto główne o nazwie użytkownika"}>
            <div className="flex items-center"><span className="rounded-l-lg border border-r-0 border-slate-300 bg-slate-50 px-3 py-2 font-mono text-sm text-slate-500">{user?.username}{form.suffix ? "_" : ""}</span><Input value={form.suffix} onChange={(e) => setForm({ ...form, suffix: e.target.value })} className="rounded-l-none" pattern="[a-z0-9_]{0,24}" required={hasMain} autoFocus /></div>
          </Field>
          <Field label="Hasło" hint="minimum 8 znaków"><Input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required minLength={8} /></Field>
          <Field label="Katalog startowy" hint="względem katalogu domowego; puste = cały katalog domowy"><Input value={form.home_subdir} onChange={(e) => setForm({ ...form, home_subdir: e.target.value })} placeholder="domains/example.com/public_html" /></Field>
        </form>
      </Modal>
      <Modal open={!!pw} onClose={() => setPw(null)} title={`Zmień hasło: ${pw?.login ?? ""}`} footer={<><Button variant="secondary" onClick={() => setPw(null)}>Anuluj</Button><Button onClick={() => pw && pwM.mutate(pw)} loading={pwM.isPending} disabled={newPw.length < 8}>Zapisz</Button></>}>
        <Field label="Nowe hasło"><Input type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} minLength={8} autoFocus /></Field>
      </Modal>
      <Confirm open={!!remove} onClose={() => setRemove(null)} onConfirm={() => remove && removeM.mutate(remove.id)} loading={removeM.isPending} title="Usuń konto FTP" text={<>Konto <b>{remove?.login}</b> zostanie usunięte. Pliki pozostaną nienaruszone.</>} />
    </>
  );
}
