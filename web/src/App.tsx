import { Navigate, Route, Routes } from "react-router-dom";
import { useAuth } from "./lib/auth";
import Layout from "./components/Layout";
import { Spinner } from "./components/ui";
import Login from "./pages/Login";
import AdminDashboard from "./pages/admin/Dashboard";
import AdminUsers from "./pages/admin/Users";
import AdminPackages from "./pages/admin/Packages";
import AdminDomains from "./pages/admin/Domains";
import AdminSettings from "./pages/admin/Settings";
import AdminAudit from "./pages/admin/Audit";
import UserDashboard from "./pages/user/Dashboard";
import UserDomains from "./pages/user/Domains";
import UserDatabases from "./pages/user/Databases";
import UserFiles from "./pages/user/Files";
import UserFtp from "./pages/user/Ftp";
import UserCron from "./pages/user/Cron";
import UserAccount from "./pages/user/Account";

export default function App() {
  const { user, loading } = useAuth();
  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner />
      </div>
    );
  }
  if (!user) {
    return (
      <Routes>
        <Route path="*" element={<Login />} />
      </Routes>
    );
  }
  if (user.role === "admin") {
    return (
      <Routes>
        <Route path="/admin" element={<Layout admin />}>
          <Route index element={<AdminDashboard />} />
          <Route path="users" element={<AdminUsers />} />
          <Route path="packages" element={<AdminPackages />} />
          <Route path="domains" element={<AdminDomains />} />
          <Route path="settings" element={<AdminSettings />} />
          <Route path="audit" element={<AdminAudit />} />
        </Route>
        <Route path="*" element={<Navigate to="/admin" replace />} />
      </Routes>
    );
  }
  return (
    <Routes>
      <Route path="/" element={<Layout admin={false} />}>
        <Route index element={<UserDashboard />} />
        <Route path="domains" element={<UserDomains />} />
        <Route path="databases" element={<UserDatabases />} />
        <Route path="files" element={<UserFiles />} />
        <Route path="ftp" element={<UserFtp />} />
        <Route path="cron" element={<UserCron />} />
        <Route path="account" element={<UserAccount />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
