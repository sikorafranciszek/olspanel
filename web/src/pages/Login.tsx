import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { errorMessage } from "../lib/api";
import { Button, ErrorBox, Field, Input } from "../components/ui";

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const u = await login(username, password);
      navigate(u.role === "admin" ? "/admin" : "/", { replace: true });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-full items-center justify-center bg-gradient-to-br from-slate-100 to-slate-200 p-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-5 rounded-2xl border border-slate-200 bg-white p-8 shadow-lg">
        <div className="flex items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-600 text-lg font-bold text-white">O</span>
          <div>
            <h1 className="text-lg font-semibold text-slate-900">olspanel</h1>
            <p className="text-xs text-slate-500">Panel hostingowy OpenLiteSpeed</p>
          </div>
        </div>
        {error && <ErrorBox>{error}</ErrorBox>}
        <Field label="Nazwa użytkownika">
          <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus required />
        </Field>
        <Field label="Hasło">
          <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
        </Field>
        <Button type="submit" className="w-full" loading={busy}>
          Zaloguj się
        </Button>
      </form>
    </div>
  );
}
