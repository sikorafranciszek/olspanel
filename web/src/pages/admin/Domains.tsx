import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { get } from "../../lib/api";
import type { Domain } from "../../lib/types";
import { Badge, Empty, Input, PageHeader, Spinner, Table, Td, Th, formatDate, phpLabel } from "../../components/ui";

const typeLabel: Record<Domain["type"], string> = { domain: "domena", subdomain: "subdomena", alias: "alias" };

export default function AdminDomains() {
  const q = useQuery({ queryKey: ["admin", "domains"], queryFn: () => get<Domain[]>("/admin/domains") });
  const [filter, setFilter] = useState("");
  const rows = (q.data ?? []).filter((d) => !filter || d.name.includes(filter.toLowerCase()) || d.username.includes(filter.toLowerCase()));
  return (
    <>
      <PageHeader title="Domeny" subtitle="Wszystkie domeny na serwerze" actions={<Input placeholder="Szukaj domeny lub użytkownika…" value={filter} onChange={(e) => setFilter(e.target.value)} className="w-64" />} />
      {q.isLoading ? (
        <Spinner />
      ) : (
        <Table head={<><Th>Domena</Th><Th>Typ</Th><Th>Użytkownik</Th><Th>PHP</Th><Th>SSL</Th><Th>Utworzono</Th></>} empty={rows.length === 0 && <Empty>Brak domen</Empty>}>
          {rows.map((d) => (
            <tr key={d.id} className="hover:bg-slate-50">
              <Td className="font-medium text-slate-900">{d.name}</Td>
              <Td><Badge>{typeLabel[d.type]}</Badge></Td>
              <Td>{d.username}</Td>
              <Td>{d.type === "alias" ? "–" : phpLabel(d.php_version)}</Td>
              <Td>{d.type === "alias" ? "–" : d.ssl_status === "active" ? <Badge tone="green">aktywny</Badge> : d.ssl_status === "error" ? <Badge tone="red">błąd</Badge> : <Badge>brak</Badge>}</Td>
              <Td className="text-slate-500">{formatDate(d.created_at)}</Td>
            </tr>
          ))}
        </Table>
      )}
    </>
  );
}
