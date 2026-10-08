import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { put, errorMessage } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { useToast } from "../../components/toast";
import { Button, Card, Field, Input, PageHeader } from "../../components/ui";

export default function UserAccount() {
  const { user, impersonating } = useAuth();
  const toast = useToast();
  const [form, setForm] = useState({ current: "", next: "", repeat: "" });
  const m = useMutation({
    mutationFn: () => put("/auth/password", { current: form.current, new: form.next }),
    onSuccess: () => {
      toast.success("Hasło zostało zmienione");
      setForm({ current: "", next: "", repeat: "" });
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (form.next !== form.repeat) {
      toast.error("Nowe hasła nie są identyczne");
      return;
    }
    m.mutate();
  };
  return (
    <>
      <PageHeader title="Moje konto" />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="Dane konta">
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
            <dt className="text-slate-500">Użytkownik</dt><dd className="font-medium">{user?.username}</dd>
            <dt className="text-slate-500">E-mail</dt><dd>{user?.email || "–"}</dd>
            <dt className="text-slate-500">Pakiet</dt><dd>{user?.package_name || "–"}</dd>
            <dt className="text-slate-500">Katalog domowy</dt><dd className="font-mono">{user?.home}</dd>
          </dl>
        </Card>
        <Card title="Zmiana hasła">
          {impersonating ? (
            <p className="text-sm text-slate-500">Zmiana hasła jest niedostępna w trybie podglądu konta przez administratora.</p>
          ) : (
            <form onSubmit={submit} className="space-y-4">
              <Field label="Obecne hasło"><Input type="password" value={form.current} onChange={(e) => setForm({ ...form, current: e.target.value })} required autoComplete="current-password" /></Field>
              <Field label="Nowe hasło" hint="minimum 8 znaków"><Input type="password" value={form.next} onChange={(e) => setForm({ ...form, next: e.target.value })} required minLength={8} autoComplete="new-password" /></Field>
              <Field label="Powtórz nowe hasło"><Input type="password" value={form.repeat} onChange={(e) => setForm({ ...form, repeat: e.target.value })} required minLength={8} autoComplete="new-password" /></Field>
              <Button type="submit" loading={m.isPending}>Zmień hasło</Button>
            </form>
          )}
        </Card>
      </div>
    </>
  );
}
