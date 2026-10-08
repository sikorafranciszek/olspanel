import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Lock, Plus, Trash2 } from "lucide-react";
import { del, get, post, put, errorMessage } from "../../lib/api";
import type { Domain, DomainType, PHPVersion } from "../../lib/types";
import { useToast } from "../../components/toast";
import { Badge, Button, Confirm, Empty, Field, Input, Modal, PageHeader, Select, Spinner, Table, Td, Th, formatDate, phpLabel } from "../../components/ui";

const typeLabel: Record<DomainType, string> = { domain: "domena", subdomain: "subdomena", alias: "alias" };

export default function UserDomains() {
  const qc = useQueryClient();
  const toast = useToast();
  const domains = useQuery({ queryKey: ["domains"], queryFn: () => get<Domain[]>("/domains") });
  const php = useQuery({ queryKey: ["php"], queryFn: () => get<PHPVersion[]>("/php/versions") });
  const [create, setCreate] = useState(false);
  const [form, setForm] = useState({ name: "", type: "domain" as DomainType, parent_id: 0, php_version: "" });
  const [remove, setRemove] = useState<Domain | null>(null);
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["domains"] });
    qc.invalidateQueries({ queryKey: ["usage"] });
  };
  const createM = useMutation({
    mutationFn: () => post<Domain>("/domains", { name: form.name, type: form.type, parent_id: form.parent_id || undefined, php_version: form.php_version || undefined }),
    onSuccess: () => {
      toast.success("Domena została dodana");
      setCreate(false);
      setForm({ name: "", type: "domain", parent_id: 0, php_version: "" });
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const updateM = useMutation({
    mutationFn: ({ id, body }: { id: number; body: { php_version?: string; force_https?: boolean } }) => put<Domain>(`/domains/${id}`, body),
    onSuccess: () => {
      toast.success("Zapisano");
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const sslM = useMutation({
    mutationFn: (id: number) => post<Domain>(`/domains/${id}/ssl/issue`),
    onSuccess: () => {
      toast.success("Certyfikat SSL został wystawiony");
      invalidate();
    },
    onError: (e) => {
      toast.error(errorMessage(e));
      invalidate();
    },
  });
  const removeM = useMutation({
    mutationFn: (id: number) => del(`/domains/${id}`),
    onSuccess: () => {
      toast.success("Domena została usunięta");
      setRemove(null);
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const parents = (domains.data ?? []).filter((d) => d.type === "domain");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    createM.mutate();
  };

  return (
    <>
      <PageHeader title="Domeny" subtitle="Domeny, subdomeny i aliasy przypisane do konta" actions={<Button onClick={() => setCreate(true)}><Plus className="h-4 w-4" /> Dodaj domenę</Button>} />
      {domains.isLoading ? (
        <Spinner />
      ) : (
        <Table head={<><Th>Domena</Th><Th>Typ</Th><Th>PHP</Th><Th>SSL</Th><Th>HTTPS</Th><Th>Dodano</Th><Th className="text-right">Akcje</Th></>} empty={domains.data?.length === 0 && <Empty>Nie masz jeszcze żadnych domen</Empty>}>
          {domains.data?.map((d) => (
            <tr key={d.id} className="hover:bg-slate-50">
              <Td>
                <div className="font-medium text-slate-900">{d.name}</div>
                {d.type !== "alias" && <div className="text-xs text-slate-500">domains/{d.name}/public_html</div>}
              </Td>
              <Td><Badge>{typeLabel[d.type]}</Badge></Td>
              <Td>
                {d.type === "alias" ? "–" : (
                  <Select value={d.php_version} onChange={(e) => updateM.mutate({ id: d.id, body: { php_version: e.target.value } })} className="w-28 py-1 text-xs">
                    {php.data?.map((v) => <option key={v.id} value={v.id}>{v.label}</option>)}
                    {!php.data?.some((v) => v.id === d.php_version) && <option value={d.php_version}>{phpLabel(d.php_version)}</option>}
                  </Select>
                )}
              </Td>
              <Td>
                {d.type === "alias" ? "–" : d.ssl_status === "active" ? (
                  <span className="flex flex-col"><Badge tone="green">aktywny</Badge><span className="mt-0.5 text-[11px] text-slate-500">do {formatDate(d.ssl_expires_at)}</span></span>
                ) : (
                  <Button size="sm" variant="secondary" onClick={() => sslM.mutate(d.id)} loading={sslM.isPending && sslM.variables === d.id}><Lock className="h-3.5 w-3.5" /> {d.ssl_status === "error" ? "Spróbuj ponownie" : "Włącz SSL"}</Button>
                )}
              </Td>
              <Td>
                {d.type !== "alias" && (
                  <label className="flex items-center gap-1.5 text-xs">
                    <input type="checkbox" checked={d.force_https} disabled={d.ssl_status !== "active"} onChange={(e) => updateM.mutate({ id: d.id, body: { force_https: e.target.checked } })} className="rounded border-slate-300" /> wymuś
                  </label>
                )}
              </Td>
              <Td className="text-slate-500">{formatDate(d.created_at)}</Td>
              <Td><div className="flex justify-end"><Button size="sm" variant="ghost" onClick={() => setRemove(d)} title="Usuń"><Trash2 className="h-4 w-4 text-red-600" /></Button></div></Td>
            </tr>
          ))}
        </Table>
      )}

      <Modal open={create} onClose={() => setCreate(false)} title="Dodaj domenę" footer={<><Button variant="secondary" onClick={() => setCreate(false)}>Anuluj</Button><Button type="submit" form="domain-form" loading={createM.isPending}>Dodaj</Button></>}>
        <form id="domain-form" onSubmit={submit} className="space-y-4">
          <Field label="Typ">
            <Select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value as DomainType })}>
              <option value="domain">Domena</option>
              <option value="subdomain" disabled={parents.length === 0}>Subdomena</option>
              <option value="alias" disabled={parents.length === 0}>Alias (domena wskazująca na inną)</option>
            </Select>
          </Field>
          {form.type !== "domain" && (
            <Field label="Domena nadrzędna">
              <Select value={form.parent_id} onChange={(e) => setForm({ ...form, parent_id: Number(e.target.value) })} required>
                <option value={0}>– wybierz –</option>
                {parents.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
              </Select>
            </Field>
          )}
          <Field label="Nazwa" hint={form.type === "subdomain" ? "pełna nazwa, np. blog.example.com" : "bez www, np. example.com"}>
            <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required autoFocus />
          </Field>
          {form.type !== "alias" && (
            <Field label="Wersja PHP">
              <Select value={form.php_version} onChange={(e) => setForm({ ...form, php_version: e.target.value })}>
                <option value="">– domyślna –</option>
                {php.data?.map((v) => <option key={v.id} value={v.id}>{v.label}</option>)}
              </Select>
            </Field>
          )}
        </form>
      </Modal>
      <Confirm open={!!remove} onClose={() => setRemove(null)} onConfirm={() => remove && removeM.mutate(remove.id)} loading={removeM.isPending} title="Usuń domenę" text={<>Domena <b>{remove?.name}</b>{remove?.type === "domain" ? " wraz z subdomenami i aliasami" : ""} oraz jej katalog z plikami zostaną usunięte. Tej operacji nie można cofnąć.</>} />
    </>
  );
}
