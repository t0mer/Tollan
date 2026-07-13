import { Routes, Route } from "react-router-dom";
import { useAuth } from "@/lib/auth";
import { LoginScreen } from "@/pages/login";
import { UsersPage } from "@/pages/users";
import { TokensPage } from "@/pages/tokens";
import { AppShell } from "@/components/app-shell";
import { OverviewPage } from "@/pages/overview";
import { SearchPage } from "@/pages/search";
import { InputsPage } from "@/pages/inputs";
import { StreamsPage } from "@/pages/streams";
import { PipelinesPage } from "@/pages/pipelines";
import { DashboardsPage } from "@/pages/dashboards";
import { DashboardViewPage } from "@/pages/dashboard-view";
import { AlertsPage } from "@/pages/alerts";
import { FleetPage } from "@/pages/fleet";
import { NotificationsPage } from "@/pages/notifications";
import { OutputsPage } from "@/pages/outputs";
import { SystemPage } from "@/pages/system";
import { PlaceholderPage } from "@/pages/placeholder";

export default function App() {
  const { loading, authEnabled, needsSetup, user } = useAuth();

  if (loading) {
    return <div className="grid min-h-screen place-items-center text-sm text-muted-foreground">Loading…</div>;
  }
  if (authEnabled && (needsSetup || !user)) {
    return <LoginScreen />;
  }

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<OverviewPage />} />
        <Route path="search" element={<SearchPage />} />
        <Route path="streams" element={<StreamsPage />} />
        <Route path="pipelines" element={<PipelinesPage />} />
        <Route path="dashboards" element={<DashboardsPage />} />
        <Route path="dashboards/:id" element={<DashboardViewPage />} />
        <Route path="alerts" element={<AlertsPage />} />
        <Route path="notifications" element={<NotificationsPage />} />
        <Route path="outputs" element={<OutputsPage />} />
        <Route path="inputs" element={<InputsPage />} />
        <Route path="fleet" element={<FleetPage />} />
        <Route path="system" element={<SystemPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="tokens" element={<TokensPage />} />
        <Route path="*" element={<PlaceholderPage title="Not found" blurb="Nothing here." />} />
      </Route>
    </Routes>
  );
}
